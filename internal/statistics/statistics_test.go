package statistics

import (
	"reflect"
	"testing"

	"github.com/ucpr/tailsamplingpreviewer/internal/sampling"
	itrace "github.com/ucpr/tailsamplingpreviewer/internal/trace"
)

func result(decision itrace.Decision, matched ...string) sampling.EvalResult {
	policies := make([]sampling.PolicyResult, 0, len(matched))
	for _, name := range matched {
		policies = append(policies, sampling.PolicyResult{Name: name, Matched: true})
	}
	return sampling.EvalResult{Decision: decision, Policies: policies}
}

func TestEngine_Record(t *testing.T) {
	tests := []struct {
		name    string
		records []sampling.EvalResult
		want    Snapshot
	}{
		{
			name:    "single keep",
			records: []sampling.EvalResult{result(itrace.DecisionKeep, "errors")},
			want: Snapshot{
				Observed: 1, Keep: 1, Drop: 0, SamplingRate: 1,
				PolicyMatches: map[string]uint64{"errors": 1},
			},
		},
		{
			name:    "single drop, no policy matched",
			records: []sampling.EvalResult{result(itrace.DecisionDrop)},
			want: Snapshot{
				Observed: 1, Keep: 0, Drop: 1, SamplingRate: 0,
				PolicyMatches: map[string]uint64{},
			},
		},
		{
			name: "mixed keep/drop computes sampling rate",
			records: []sampling.EvalResult{
				result(itrace.DecisionKeep, "errors"),
				result(itrace.DecisionKeep, "errors"),
				result(itrace.DecisionDrop),
				result(itrace.DecisionDrop),
			},
			want: Snapshot{
				Observed: 4, Keep: 2, Drop: 2, SamplingRate: 0.5,
				PolicyMatches: map[string]uint64{"errors": 2},
			},
		},
		{
			name: "a trace can match multiple policies, each is counted",
			records: []sampling.EvalResult{
				result(itrace.DecisionKeep, "errors", "slow"),
			},
			want: Snapshot{
				Observed: 1, Keep: 1, Drop: 0, SamplingRate: 1,
				PolicyMatches: map[string]uint64{"errors": 1, "slow": 1},
			},
		},
		{
			name:    "no records yields a zero-value snapshot with no divide-by-zero",
			records: nil,
			want: Snapshot{
				Observed: 0, Keep: 0, Drop: 0, SamplingRate: 0,
				PolicyMatches: map[string]uint64{},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := NewEngine()
			for _, r := range tt.records {
				e.Record(r)
			}
			got := e.Snapshot()
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("Snapshot() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestEngine_Reset(t *testing.T) {
	e := NewEngine()
	e.Record(result(itrace.DecisionKeep, "errors"))
	e.Record(result(itrace.DecisionDrop))

	e.Reset()
	got := e.Snapshot()
	want := Snapshot{Observed: 0, Keep: 0, Drop: 0, SamplingRate: 0, PolicyMatches: map[string]uint64{}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Snapshot() after Reset = %+v, want %+v", got, want)
	}
}

func TestSnapshot_PolicyMatchList_SortedByName(t *testing.T) {
	e := NewEngine()
	e.Record(result(itrace.DecisionKeep, "zeta"))
	e.Record(result(itrace.DecisionKeep, "alpha"))
	e.Record(result(itrace.DecisionKeep, "alpha"))

	got := e.Snapshot().PolicyMatchList()
	want := []PolicyMatch{{Name: "alpha", Matches: 2}, {Name: "zeta", Matches: 1}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("PolicyMatchList() = %+v, want %+v", got, want)
	}
}
