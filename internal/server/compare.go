package server

import (
	"go.uber.org/zap"

	"github.com/ucpr/tailsamplingpreviewer/internal/sampling"
	"github.com/ucpr/tailsamplingpreviewer/internal/trace"
)

// PolicySnapshot is one side (Current or Candidate) of a Policy Compare
// result (spec.md ss30).
type PolicySnapshot struct {
	Observed     int     `json:"observed"`
	Keep         int     `json:"keep"`
	Drop         int     `json:"drop"`
	SamplingRate float64 `json:"sampling_rate"`
}

// CompareResult is the outcome of evaluating every currently-decided trace
// against both the live policy (Current) and a candidate policy, without
// applying the candidate (spec.md ss30 "Policy Compare").
type CompareResult struct {
	Current                 PolicySnapshot `json:"current"`
	Candidate               PolicySnapshot `json:"candidate"`
	NewlyKept               int            `json:"newly_kept"`
	NewlyDropped            int            `json:"newly_dropped"`
	ErrorTracesNewlyDropped int            `json:"error_traces_newly_dropped"`
}

// comparePolicy evaluates every StateDecided trace currently retained
// against candidate, without mutating the server's active policy, store,
// or statistics. Current-side counts come from each trace's
// already-computed Decision (the live policy's ground truth); Candidate
// counts come from re-running candidate against that very same trace set
// in this one pass, so the two sides always diff an identical population
// -- mixing in s.stats.Snapshot() here would not do that, since that
// running total can include traces already evicted from the store by the
// time this runs.
func (s *Server) comparePolicy(candidate sampling.Config) (CompareResult, error) {
	ev, err := sampling.NewEvaluator(candidate)
	if err != nil {
		return CompareResult{}, err
	}

	var result CompareResult
	for _, t := range s.store.All() {
		if t.State != trace.StateDecided {
			continue
		}

		currentKeep := t.Decision == trace.DecisionKeep
		result.Current.Observed++
		if currentKeep {
			result.Current.Keep++
		} else {
			result.Current.Drop++
		}

		// A candidate evaluation error (e.g. an OTTL runtime error under
		// error_mode: propagate) only drops this one trace from the
		// candidate-side tally and the Newly* comparison below, rather
		// than failing the whole request -- consistent with this
		// project's "Preview data may be lossy" principle (see
		// evaluator.go/store.go).
		candResult, err := ev.Evaluate(&t)
		if err != nil {
			s.logger.Error("compare policy: evaluate trace", zap.String("trace_id", t.TraceID.String()), zap.Error(err))
			continue
		}

		candidateKeep := candResult.Decision == trace.DecisionKeep
		result.Candidate.Observed++
		if candidateKeep {
			result.Candidate.Keep++
		} else {
			result.Candidate.Drop++
		}

		switch {
		case !currentKeep && candidateKeep:
			result.NewlyKept++
		case currentKeep && !candidateKeep:
			result.NewlyDropped++
			if t.HasError() {
				result.ErrorTracesNewlyDropped++
			}
		}
	}

	result.Current.SamplingRate = samplingRate(result.Current.Keep, result.Current.Observed)
	result.Candidate.SamplingRate = samplingRate(result.Candidate.Keep, result.Candidate.Observed)

	return result, nil
}

func samplingRate(keep, observed int) float64 {
	if observed == 0 {
		return 0
	}
	return float64(keep) / float64(observed)
}
