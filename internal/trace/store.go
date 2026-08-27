package trace

import (
	"container/list"
	"sync"
	"time"

	"go.opentelemetry.io/collector/pdata/pcommon"
)

// StoreConfig bounds the in-memory ring buffer (spec.md ss31). Traces are
// never persisted to disk. Note that MaxAge (and decision_wait, which is a
// separate Policy Engine setting) only govern *when* a trace is decided;
// eviction from memory is governed solely by MaxTraces/MaxAge/MaxMemory
// here, so a decided trace stays visible in Live Tail until ring-buffer
// pressure pushes it out.
type StoreConfig struct {
	MaxTraces int
	MaxAge    time.Duration
	MaxMemory uint64 // approximate retained bytes across all traces; 0 = use default
}

func DefaultStoreConfig() StoreConfig {
	return StoreConfig{MaxTraces: 10000, MaxAge: 5 * time.Minute, MaxMemory: 512 * 1024 * 1024}
}

// Store is an in-memory, eviction-bounded collection of Trace aggregates.
// It is the single source of truth Live Tail, the Policy Engine and
// Statistics read from. All methods are safe for concurrent use.
type Store struct {
	cfg StoreConfig

	mu         sync.Mutex
	byID       map[pcommon.TraceID]*list.Element // element.Value is *Trace
	order      *list.List                        // front = oldest FirstSeen, back = newest
	totalBytes uint64
	evicted    uint64
}

func NewStore(cfg StoreConfig) *Store {
	if cfg.MaxTraces <= 0 {
		cfg.MaxTraces = DefaultStoreConfig().MaxTraces
	}
	if cfg.MaxAge <= 0 {
		cfg.MaxAge = DefaultStoreConfig().MaxAge
	}
	if cfg.MaxMemory <= 0 {
		cfg.MaxMemory = DefaultStoreConfig().MaxMemory
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
		var addedBytes uint64
		for _, sp := range spans {
			addedBytes += sp.approxSizeBytes()
		}

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
		t.SizeBytes += addedBytes
		s.totalBytes += addedBytes
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

// TotalBytes reports the approximate retained memory across every trace
// currently in the store (feeds tailpreview_trace_buffer_bytes,
// spec.md ss36), used to enforce MaxMemory.
func (s *Store) TotalBytes() uint64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.totalBytes
}

// Evicted reports the cumulative number of traces dropped from the ring
// buffer due to MaxTraces/MaxAge/MaxMemory pressure (feeds
// tailpreview_trace_evicted_total, spec.md ss36).
func (s *Store) Evicted() uint64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.evicted
}

// evictLocked drops the oldest traces (front of the list) until the store
// satisfies MaxTraces, MaxAge and MaxMemory. Traces are always sacrificed
// oldest-first, regardless of which constraint is currently violated: the
// list is kept ordered by recency (Ingest calls MoveToBack), so the front
// element is always the best eviction candidate. Callers must hold s.mu.
func (s *Store) evictLocked(now time.Time) {
	for {
		el := s.order.Front()
		if el == nil {
			return
		}
		t := el.Value.(*Trace)

		overCount := s.order.Len() > s.cfg.MaxTraces
		overAge := s.cfg.MaxAge > 0 && now.Sub(t.LastSeen) > s.cfg.MaxAge
		overMemory := s.cfg.MaxMemory > 0 && s.totalBytes > s.cfg.MaxMemory
		if !overCount && !overAge && !overMemory {
			return
		}
		s.removeLocked(el, t.TraceID)
	}
}

func (s *Store) removeLocked(el *list.Element, id pcommon.TraceID) {
	t := el.Value.(*Trace)
	s.totalBytes -= t.SizeBytes
	s.order.Remove(el)
	delete(s.byID, id)
	s.evicted++
}
