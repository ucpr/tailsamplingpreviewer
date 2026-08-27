package trace

import (
	"container/list"
	"sync"
	"time"

	"go.opentelemetry.io/collector/pdata/pcommon"
)

// StoreConfig bounds the in-memory ring buffer (spec.md ss31). Traces are
// never persisted to disk.
type StoreConfig struct {
	MaxTraces int
	MaxAge    time.Duration
}

func DefaultStoreConfig() StoreConfig {
	return StoreConfig{MaxTraces: 10000, MaxAge: 5 * time.Minute}
}

// Store is an in-memory, eviction-bounded collection of Trace aggregates.
// It is the single source of truth Live Tail, the Policy Engine and
// Statistics read from. All methods are safe for concurrent use.
type Store struct {
	cfg StoreConfig

	mu      sync.Mutex
	byID    map[pcommon.TraceID]*list.Element // element.Value is *Trace
	order   *list.List                        // front = oldest FirstSeen, back = newest
	evicted uint64
}

func NewStore(cfg StoreConfig) *Store {
	if cfg.MaxTraces <= 0 {
		cfg.MaxTraces = DefaultStoreConfig().MaxTraces
	}
	if cfg.MaxAge <= 0 {
		cfg.MaxAge = DefaultStoreConfig().MaxAge
	}
	return &Store{
		cfg:   cfg,
		byID:  make(map[pcommon.TraceID]*list.Element),
		order: list.New(),
	}
}

// Ingest merges newly-arrived spans, grouped by trace ID, into the store.
// It returns copies of the (possibly newly created) Trace aggregates that
// were touched, for callers that want to push Live Tail updates
// (spec.md ss22). Copies are returned rather than live pointers so callers
// never race with concurrent internal mutation (see ForEachMutate /
// ApplyDecision).
func (s *Store) Ingest(now time.Time, spansByTrace map[pcommon.TraceID][]Span) []Trace {
	s.mu.Lock()
	defer s.mu.Unlock()

	touched := make([]Trace, 0, len(spansByTrace))
	for id, spans := range spansByTrace {
		el, ok := s.byID[id]
		var t *Trace
		if ok {
			t = el.Value.(*Trace)
			t.Spans = append(t.Spans, spans...)
			t.LastSeen = now
			if t.State == StateDecided || t.State == StateExpired {
				// Late-arriving spans re-open the trace for a fresh
				// decision rather than silently discarding them.
				t.State = StateReceiving
			}
			s.order.MoveToBack(el)
		} else {
			t = &Trace{
				TraceID:   id,
				FirstSeen: now,
				LastSeen:  now,
				Spans:     append([]Span(nil), spans...),
				State:     StateReceiving,
			}
			s.byID[id] = s.order.PushBack(t)
		}
		touched = append(touched, cloneTrace(t))
	}

	s.evictLocked(now)
	return touched
}

// DueForDecision returns copies of traces whose decision_wait has elapsed
// and that have not yet been decided (spec.md ss21). It flips their
// internal State to StateReady so they are not returned again; call
// ApplyDecision with the evaluation result to finish the transition to
// StateDecided.
func (s *Store) DueForDecision(now time.Time, decisionWait time.Duration) []Trace {
	s.mu.Lock()
	defer s.mu.Unlock()

	var due []Trace
	for el := s.order.Front(); el != nil; el = el.Next() {
		t := el.Value.(*Trace)
		if t.State != StateReceiving {
			continue
		}
		if now.Sub(t.FirstSeen) >= decisionWait {
			t.State = StateReady
			due = append(due, cloneTrace(t))
		}
	}
	return due
}

// ApplyDecision records a policy evaluation result against the live trace,
// transitioning it to StateDecided. It returns the updated copy and false
// if the trace is no longer retained (e.g. evicted).
func (s *Store) ApplyDecision(id pcommon.TraceID, decision Decision, matchedPolicies []string) (Trace, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	el, ok := s.byID[id]
	if !ok {
		return Trace{}, false
	}
	t := el.Value.(*Trace)
	t.Decision = decision
	t.MatchedPolicies = matchedPolicies
	t.State = StateDecided
	return cloneTrace(t), true
}

// ForEachMutate applies fn to every retained trace under a single lock,
// e.g. to re-evaluate the whole ring buffer after a Policy Update
// (spec.md ss24). fn receives the live pointer; it must not retain it
// past the call.
func (s *Store) ForEachMutate(fn func(*Trace)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for el := s.order.Front(); el != nil; el = el.Next() {
		fn(el.Value.(*Trace))
	}
}

// All returns a snapshot copy of every Trace currently retained, oldest
// first.
func (s *Store) All() []Trace {
	s.mu.Lock()
	defer s.mu.Unlock()

	out := make([]Trace, 0, s.order.Len())
	for el := s.order.Front(); el != nil; el = el.Next() {
		out = append(out, cloneTrace(el.Value.(*Trace)))
	}
	return out
}

// Get returns a copy of the Trace for the given ID, if present.
func (s *Store) Get(id pcommon.TraceID) (Trace, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	el, ok := s.byID[id]
	if !ok {
		return Trace{}, false
	}
	return cloneTrace(el.Value.(*Trace)), true
}

func cloneTrace(t *Trace) Trace {
	clone := *t
	clone.Spans = append([]Span(nil), t.Spans...)
	clone.MatchedPolicies = append([]string(nil), t.MatchedPolicies...)
	return clone
}

// Len reports the number of traces currently retained.
func (s *Store) Len() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.order.Len()
}

// Evicted reports the cumulative number of traces dropped from the ring
// buffer due to MaxTraces/MaxAge pressure (feeds
// tailpreview_trace_evicted_total, spec.md ss36).
func (s *Store) Evicted() uint64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.evicted
}

// evictLocked drops the oldest traces until the store satisfies
// MaxTraces/MaxAge. Callers must hold s.mu.
func (s *Store) evictLocked(now time.Time) {
	for s.order.Len() > s.cfg.MaxTraces {
		s.popOldestLocked()
	}
	for el := s.order.Front(); el != nil; {
		t := el.Value.(*Trace)
		if now.Sub(t.LastSeen) <= s.cfg.MaxAge {
			break
		}
		next := el.Next()
		s.removeLocked(el, t.TraceID)
		el = next
	}
}

func (s *Store) popOldestLocked() {
	el := s.order.Front()
	if el == nil {
		return
	}
	t := el.Value.(*Trace)
	s.removeLocked(el, t.TraceID)
}

func (s *Store) removeLocked(el *list.Element, id pcommon.TraceID) {
	s.order.Remove(el)
	delete(s.byID, id)
	s.evicted++
}
