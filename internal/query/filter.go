// Package query implements the Live Tail UI filter (spec.md ss27). This is
// purely a display filter: it never influences the Sampling Decision.
package query

import (
	"strconv"
	"strings"
	"time"

	"github.com/ucpr/tailsamplingpreviewer/internal/trace"
)

// Filter is a parsed Live Tail query, e.g. `service.name:payment
// status:error duration:>1s`. All terms are ANDed together.
type Filter struct {
	terms []term
}

type term struct {
	key  string // "" for a free-text term
	op   string // "", ":", ">", ">=", "<", "<="
	text string
}

// Parse builds a Filter from raw query text. Parse never fails: malformed
// terms are treated as free-text substring matches, matching the forgiving
// feel of a Datadog-style search bar (spec.md ss27).
func Parse(query string) Filter {
	fields := strings.Fields(query)
	terms := make([]term, 0, len(fields))
	for _, f := range fields {
		terms = append(terms, parseTerm(f))
	}
	return Filter{terms: terms}
}

func parseTerm(f string) term {
	idx := strings.IndexByte(f, ':')
	if idx < 0 {
		return term{text: strings.ToLower(f)}
	}
	key := strings.ToLower(f[:idx])
	val := f[idx+1:]

	op := ":"
	for _, candidate := range []string{">=", "<=", ">", "<"} {
		if strings.HasPrefix(val, candidate) {
			op = candidate
			val = val[len(candidate):]
			break
		}
	}
	return term{key: key, op: op, text: val}
}

// Empty reports whether the filter has no terms (matches everything).
func (f Filter) Empty() bool { return len(f.terms) == 0 }

// Match reports whether t satisfies every term in the filter.
func (f Filter) Match(t *trace.Trace) bool {
	for _, term := range f.terms {
		if !term.match(t) {
			return false
		}
	}
	return true
}

func (t term) match(tr *trace.Trace) bool {
	if t.key == "" {
		return matchFreeText(tr, t.text)
	}

	switch t.key {
	case "status":
		want := strings.EqualFold(t.text, "error")
		return tr.HasError() == want
	case "duration":
		return matchDuration(tr, t.op, t.text)
	case "service.name", "service":
		return strings.Contains(strings.ToLower(tr.RootServiceName()), strings.ToLower(t.text))
	default:
		return matchAttribute(tr, t.key, t.text)
	}
}

func matchFreeText(tr *trace.Trace, text string) bool {
	if strings.Contains(strings.ToLower(tr.RootServiceName()), text) {
		return true
	}
	for _, s := range tr.Spans {
		if strings.Contains(strings.ToLower(s.Name), text) {
			return true
		}
	}
	return false
}

func matchDuration(tr *trace.Trace, op, text string) bool {
	want, err := time.ParseDuration(text)
	if err != nil {
		return false
	}
	got := tr.Duration()
	switch op {
	case ">":
		return got > want
	case ">=":
		return got >= want
	case "<":
		return got < want
	case "<=":
		return got <= want
	default: // ":"
		return got == want
	}
}

func matchAttribute(tr *trace.Trace, key, text string) bool {
	for _, s := range tr.Spans {
		v, ok := s.Attributes[key]
		if !ok {
			continue
		}
		if str, ok := v.(string); ok && strings.EqualFold(str, text) {
			return true
		}
		if str := toDisplayString(v); strings.EqualFold(str, text) {
			return true
		}
	}
	return false
}

func toDisplayString(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case bool:
		return strconv.FormatBool(x)
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	case int64:
		return strconv.FormatInt(x, 10)
	default:
		return ""
	}
}
