package trace

import (
	"testing"
	"time"

	"go.opentelemetry.io/collector/pdata/pcommon"
)

func mkID(b byte) pcommon.TraceID {
	var id pcommon.TraceID
	id[0] = b
	return id
}

func TestStore_IngestMergesAndCreates(t *testing.T) {
	s := NewStore(StoreConfig{MaxTraces: 10, MaxAge: time.Hour})
	now := time.Now()
	id := mkID(1)

	touched := s.Ingest(now, map[pcommon.TraceID][]Span{id: {{Name: "a"}}})
	if len(touched) != 1 || len(touched[0].Spans) != 1 {
		t.Fatalf("unexpected first ingest result: %+v", touched)
	}

	later := now.Add(time.Second)
	touched = s.Ingest(later, map[pcommon.TraceID][]Span{id: {{Name: "b"}}})
	if len(touched) != 1 || len(touched[0].Spans) != 2 {
		t.Fatalf("expected merged trace with 2 spans, got %+v", touched)
	}
	if !touched[0].LastSeen.Equal(later) {
		t.Fatalf("LastSeen not updated: %+v", touched[0])
	}

	got, ok := s.Get(id)
	if !ok || len(got.Spans) != 2 {
		t.Fatalf("Get returned unexpected trace: ok=%v %+v", ok, got)
	}
}

func TestStore_EvictsByMaxTraces(t *testing.T) {
	s := NewStore(StoreConfig{MaxTraces: 2, MaxAge: time.Hour})
	now := time.Now()
	for i := byte(1); i <= 3; i++ {
		s.Ingest(now, map[pcommon.TraceID][]Span{mkID(i): {{Name: "x"}}})
	}
	if s.Len() != 2 {
		t.Fatalf("expected ring buffer capped at 2, got %d", s.Len())
	}
	if _, ok := s.Get(mkID(1)); ok {
		t.Fatalf("oldest trace should have been evicted")
	}
	if s.Evicted() != 1 {
		t.Fatalf("expected 1 eviction, got %d", s.Evicted())
	}
}

func TestStore_DueForDecisionOnlyOnce(t *testing.T) {
	s := NewStore(StoreConfig{MaxTraces: 10, MaxAge: time.Hour})
	now := time.Now()
	id := mkID(1)
	s.Ingest(now, map[pcommon.TraceID][]Span{id: {{Name: "a"}}})

	if due := s.DueForDecision(now, 30*time.Second); len(due) != 0 {
		t.Fatalf("expected nothing due before decision_wait elapses, got %+v", due)
	}

	later := now.Add(31 * time.Second)
	due := s.DueForDecision(later, 30*time.Second)
	if len(due) != 1 {
		t.Fatalf("expected exactly 1 due trace, got %+v", due)
	}

	// Should not be returned again until re-opened by late-arriving spans.
	due = s.DueForDecision(later, 30*time.Second)
	if len(due) != 0 {
		t.Fatalf("expected trace not to be due twice, got %+v", due)
	}

	updated, ok := s.ApplyDecision(id, DecisionKeep, []string{"errors"})
	if !ok || updated.Decision != DecisionKeep || updated.State != StateDecided {
		t.Fatalf("ApplyDecision failed: ok=%v %+v", ok, updated)
	}
}

func TestStore_ApplyDecision_UnknownID(t *testing.T) {
	s := NewStore(StoreConfig{MaxTraces: 10, MaxAge: time.Hour})
	_, ok := s.ApplyDecision(mkID(1), DecisionKeep, nil)
	if ok {
		t.Fatal("expected ApplyDecision on an unknown trace id to report ok=false")
	}
}

func TestStore_IngestReopensADecidedTrace(t *testing.T) {
	s := NewStore(StoreConfig{MaxTraces: 10, MaxAge: time.Hour})
	now := time.Now()
	id := mkID(1)

	s.Ingest(now, map[pcommon.TraceID][]Span{id: {{Name: "a"}}})
	if _, ok := s.ApplyDecision(id, DecisionDrop, nil); !ok {
		t.Fatal("setup: ApplyDecision failed")
	}

	// A late-arriving span for an already-decided trace must reopen it
	// for re-evaluation rather than being silently discarded.
	later := now.Add(time.Second)
	touched := s.Ingest(later, map[pcommon.TraceID][]Span{id: {{Name: "b"}}})
	if len(touched) != 1 || touched[0].State != StateReceiving {
		t.Fatalf("expected trace reopened to RECEIVING, got %+v", touched)
	}
}

func TestStore_EvictsByMaxAge(t *testing.T) {
	s := NewStore(StoreConfig{MaxTraces: 10, MaxAge: time.Minute})
	base := time.Now()

	s.Ingest(base, map[pcommon.TraceID][]Span{mkID(1): {{Name: "old"}}})
	s.Ingest(base.Add(30*time.Second), map[pcommon.TraceID][]Span{mkID(2): {{Name: "new"}}})

	// A third ingest, far enough in the future that trace 1's LastSeen
	// (base) is now older than MaxAge, triggers eviction of trace 1 only.
	s.Ingest(base.Add(90*time.Second), map[pcommon.TraceID][]Span{mkID(3): {{Name: "newest"}}})

	if _, ok := s.Get(mkID(1)); ok {
		t.Fatal("expected the stale trace to be evicted by MaxAge")
	}
	if _, ok := s.Get(mkID(2)); !ok {
		t.Fatal("expected the still-fresh trace to survive")
	}
	if s.Len() != 2 {
		t.Fatalf("expected 2 traces remaining, got %d", s.Len())
	}
}

func TestStore_EvictsByMaxMemory(t *testing.T) {
	// Each empty single-span trace costs exactly the fixed overhead from
	// Span.approxSizeBytes (128 bytes), so the budget below fits 2 traces.
	s := NewStore(StoreConfig{MaxTraces: 100, MaxAge: time.Hour, MaxMemory: 300})
	now := time.Now()

	s.Ingest(now, map[pcommon.TraceID][]Span{mkID(1): {{}}})
	s.Ingest(now.Add(time.Millisecond), map[pcommon.TraceID][]Span{mkID(2): {{}}})
	s.Ingest(now.Add(2*time.Millisecond), map[pcommon.TraceID][]Span{mkID(3): {{}}})

	if got := s.TotalBytes(); got > 300 {
		t.Fatalf("TotalBytes() = %d, want <= 300 (MaxMemory)", got)
	}
	if _, ok := s.Get(mkID(1)); ok {
		t.Fatal("expected the oldest trace to be evicted once MaxMemory was exceeded")
	}
	if _, ok := s.Get(mkID(3)); !ok {
		t.Fatal("expected the newest trace to survive")
	}
	if s.Evicted() != 1 {
		t.Fatalf("expected 1 eviction, got %d", s.Evicted())
	}
}

func TestStore_ForEachMutate(t *testing.T) {
	s := NewStore(StoreConfig{MaxTraces: 10, MaxAge: time.Hour})
	now := time.Now()
	s.Ingest(now, map[pcommon.TraceID][]Span{
		mkID(1): {{Name: "a"}},
		mkID(2): {{Name: "b"}},
	})

	s.ForEachMutate(func(t *Trace) { t.Decision = DecisionKeep })

	for _, id := range []pcommon.TraceID{mkID(1), mkID(2)} {
		got, ok := s.Get(id)
		if !ok || got.Decision != DecisionKeep {
			t.Fatalf("trace %v not mutated: ok=%v %+v", id, ok, got)
		}
	}
}

func TestStore_All(t *testing.T) {
	s := NewStore(StoreConfig{MaxTraces: 10, MaxAge: time.Hour})
	now := time.Now()
	s.Ingest(now, map[pcommon.TraceID][]Span{mkID(1): {{Name: "a"}}})
	s.Ingest(now.Add(time.Millisecond), map[pcommon.TraceID][]Span{mkID(2): {{Name: "b"}}})

	all := s.All()
	if len(all) != 2 {
		t.Fatalf("expected 2 traces, got %d", len(all))
	}
	// Oldest first.
	if all[0].TraceID != mkID(1) || all[1].TraceID != mkID(2) {
		t.Fatalf("unexpected order: %+v", all)
	}
}

func TestNewStore_DefaultsWhenUnset(t *testing.T) {
	tests := []struct {
		name string
		cfg  StoreConfig
	}{
		{"zero value config", StoreConfig{}},
		{"zero MaxTraces only", StoreConfig{MaxTraces: 0, MaxAge: time.Hour, MaxMemory: 1024}},
		{"zero MaxAge only", StoreConfig{MaxTraces: 100, MaxAge: 0, MaxMemory: 1024}},
		{"zero MaxMemory only", StoreConfig{MaxTraces: 100, MaxAge: time.Hour, MaxMemory: 0}},
		{"negative values", StoreConfig{MaxTraces: -1, MaxAge: -1}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := NewStore(tt.cfg)
			def := DefaultStoreConfig()
			if s.cfg.MaxTraces <= 0 {
				t.Fatalf("MaxTraces not defaulted: %d", s.cfg.MaxTraces)
			}
			if s.cfg.MaxAge <= 0 {
				t.Fatalf("MaxAge not defaulted: %v", s.cfg.MaxAge)
			}
			if s.cfg.MaxMemory <= 0 {
				t.Fatalf("MaxMemory not defaulted: %d", s.cfg.MaxMemory)
			}
			if tt.cfg.MaxTraces <= 0 && s.cfg.MaxTraces != def.MaxTraces {
				t.Fatalf("MaxTraces default = %d, want %d", s.cfg.MaxTraces, def.MaxTraces)
			}
			if tt.cfg.MaxAge <= 0 && s.cfg.MaxAge != def.MaxAge {
				t.Fatalf("MaxAge default = %v, want %v", s.cfg.MaxAge, def.MaxAge)
			}
			if tt.cfg.MaxMemory <= 0 && s.cfg.MaxMemory != def.MaxMemory {
				t.Fatalf("MaxMemory default = %d, want %d", s.cfg.MaxMemory, def.MaxMemory)
			}
		})
	}
}
