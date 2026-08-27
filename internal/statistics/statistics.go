// Package statistics tracks aggregate KEEP/DROP counts and per-policy match
// counts across the traces the Preview Server has decided (spec.md ss29).
package statistics

import (
	"sort"
	"sync"

	"github.com/ucpr/tailsamplingpreviewer/internal/sampling"
)

// Snapshot is a point-in-time read of the aggregate statistics.
type Snapshot struct {
	Observed      uint64            `json:"observed"`
	Keep          uint64            `json:"keep"`
	Drop          uint64            `json:"drop"`
	SamplingRate  float64           `json:"sampling_rate"`
	PolicyMatches map[string]uint64 `json:"policy_matches"`
}

// PolicyMatchList returns PolicyMatches sorted by name, convenient for
// stable JSON/UI rendering (spec.md ss29 "Policy / Matches" table).
func (s Snapshot) PolicyMatchList() []PolicyMatch {
	out := make([]PolicyMatch, 0, len(s.PolicyMatches))
	for name, n := range s.PolicyMatches {
		out = append(out, PolicyMatch{Name: name, Matches: n})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

type PolicyMatch struct {
	Name    string `json:"name"`
	Matches uint64 `json:"matches"`
}

// Engine accumulates decisions. It is safe for concurrent use.
type Engine struct {
	mu            sync.Mutex
	observed      uint64
	keep          uint64
	drop          uint64
	policyMatches map[string]uint64
}

func NewEngine() *Engine {
	return &Engine{policyMatches: make(map[string]uint64)}
}

// Record folds one trace's evaluation result into the running totals.
func (e *Engine) Record(result sampling.EvalResult) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.observed++
	if result.Decision.String() == "KEEP" {
		e.keep++
	} else {
		e.drop++
	}
	for _, p := range result.Policies {
		if p.Matched {
			e.policyMatches[p.Name]++
		}
	}
}

// Reset zeroes every counter. Policy Update re-evaluation calls Reset then
// Record for every retained trace so statistics reflect only the current
// policy set (spec.md ss24).
func (e *Engine) Reset() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.observed, e.keep, e.drop = 0, 0, 0
	e.policyMatches = make(map[string]uint64)
}

// Snapshot returns a copy of the current totals.
func (e *Engine) Snapshot() Snapshot {
	e.mu.Lock()
	defer e.mu.Unlock()
	rate := 0.0
	if e.observed > 0 {
		rate = float64(e.keep) / float64(e.observed)
	}
	matches := make(map[string]uint64, len(e.policyMatches))
	for k, v := range e.policyMatches {
		matches[k] = v
	}
	return Snapshot{
		Observed:      e.observed,
		Keep:          e.keep,
		Drop:          e.drop,
		SamplingRate:  rate,
		PolicyMatches: matches,
	}
}
