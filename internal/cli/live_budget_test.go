package cli_test

import (
	"fmt"
	"testing"
)

func TestLiveBudgetAllow(t *testing.T) {
	b := liveBudget{limit: 30, start: 80}
	cases := []struct {
		name          string
		current, cost float64
		want          bool
		reason        string
	}{
		{"fresh run, cheap", 80, 4, true, ""},
		{"would exceed", 60, 12, false, "budget"},
		{"exactly at limit", 52, 2, true, ""},
		{"unknown cost", 80, 0, false, "unknown cost"},
		{"negative cost", 80, -1, false, "unknown cost"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ok, reason := b.allow(c.current, c.cost)
			if ok != c.want {
				t.Fatalf("allow(%v,%v) = %v (%s), want %v", c.current, c.cost, ok, reason, c.want)
			}
			if !ok && reason == "" {
				t.Fatal("denied without a reason")
			}
			if c.reason == "unknown cost" && reason != c.reason {
				t.Fatalf("reason = %q", reason)
			}
		})
	}
}

// liveBudget caps how many credits one live run may spend. start is the
// balance measured at the beginning of the run.
type liveBudget struct{ limit, start float64 }

// allow reports whether a generation costing cost fits in the remaining
// budget given the current balance. An unknown cost (<= 0) is never allowed.
func (b liveBudget) allow(current, cost float64) (bool, string) {
	if cost <= 0 {
		return false, "unknown cost"
	}
	spent := b.start - current
	if spent+cost > b.limit {
		return false, fmt.Sprintf("budget: spent %.2f + cost %.2f > limit %.2f", spent, cost, b.limit)
	}
	return true, ""
}
