package sampling

import "github.com/ucpr/tailsamplingpreviewer/internal/trace"

// PolicyResult records whether a single named policy matched a trace
// (spec.md ss28 "Matched Policies").
type PolicyResult struct {
	Name    string `json:"name"`
	Matched bool   `json:"matched"`
}

// EvalResult is the outcome of evaluating every policy in a Config against
// one trace (spec.md ss23).
type EvalResult struct {
	Decision trace.Decision `json:"decision"`
	Policies []PolicyResult `json:"policies"`
}

// MatchedNames returns the names of policies that matched, in Config order.
func (r EvalResult) MatchedNames() []string {
	names := make([]string, 0, len(r.Policies))
	for _, p := range r.Policies {
		if p.Matched {
			names = append(names, p.Name)
		}
	}
	return names
}
