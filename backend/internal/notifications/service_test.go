package notifications

import (
	"testing"
	"time"
)

// TestClassifyPriority pins the action-item heuristic so the agent's
// "what needs attention" half of the rollup stays consistent. If we tweak
// the regex set we want the test to fail loudly.
func TestClassifyPriority(t *testing.T) {
	now := time.Now()
	cases := []struct {
		name     string
		ev       Event
		priority string
		reason   string
	}{
		{
			name:     "missed_call_via_category",
			ev:       Event{Category: "call", Title: "Phone", Content: "+1 415 555 0102", CapturedAt: now},
			priority: "high",
			reason:   "missed_call",
		},
		{
			name:     "missed_call_via_title",
			ev:       Event{Category: "msg", Title: "Missed call from Sarah", Content: "", CapturedAt: now},
			priority: "high",
			reason:   "missed_call",
		},
		{
			name:     "time_sensitive_keyword",
			ev:       Event{Title: "Sarah", Content: "Showing at 3pm tomorrow — confirm please", CapturedAt: now},
			priority: "high",
			reason:   "time_sensitive",
		},
		{
			name:     "question_mark_message",
			ev:       Event{Title: "Mom", Content: "Are you free Sunday?", CapturedAt: now},
			priority: "medium",
			reason:   "question",
		},
		{
			name:     "default_low_priority",
			ev:       Event{Title: "News", Content: "BART weekend service update", CapturedAt: now},
			priority: "low",
			reason:   "follow_up",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := classify(tc.ev)
			if got.Priority != tc.priority {
				t.Errorf("priority: got %q, want %q (event=%+v)", got.Priority, tc.priority, tc.ev)
			}
			if got.Reason != tc.reason {
				t.Errorf("reason: got %q, want %q (event=%+v)", got.Reason, tc.reason, tc.ev)
			}
			if got.Summary == "" {
				t.Errorf("summary should never be empty for %+v", tc.ev)
			}
		})
	}
}

// TestPriorityRankOrder enforces the "high before medium before low"
// ordering used by Service.PendingActions to sort the response.
func TestPriorityRankOrder(t *testing.T) {
	if priorityRank("high") >= priorityRank("medium") {
		t.Errorf("high should rank before medium")
	}
	if priorityRank("medium") >= priorityRank("low") {
		t.Errorf("medium should rank before low")
	}
	if priorityRank("unknown") != priorityRank("low") {
		t.Errorf("unknown priorities should rank as low (got %d, low=%d)",
			priorityRank("unknown"), priorityRank("low"))
	}
}

// TestServiceConfigDefaults guarantees that a zero-value ServiceConfig
// produces sensible runtime knobs. Anything that downgrades these limits
// will overflow agent token budgets fast.
func TestServiceConfigDefaults(t *testing.T) {
	cfg := ServiceConfig{}.WithDefaults()
	if cfg.DefaultPageSize <= 0 {
		t.Errorf("DefaultPageSize must default to >0; got %d", cfg.DefaultPageSize)
	}
	if cfg.MaxPageSize < cfg.DefaultPageSize {
		t.Errorf("MaxPageSize (%d) must be >= DefaultPageSize (%d)",
			cfg.MaxPageSize, cfg.DefaultPageSize)
	}
	if cfg.ReplyWindowHrs <= 0 {
		t.Errorf("ReplyWindowHrs must default to >0; got %d", cfg.ReplyWindowHrs)
	}
}

// TestServiceClampLimit pins the "page caller asked for more than max ⇒
// return max; asked for nothing ⇒ return default" contract used by every
// MCP query path.
func TestServiceClampLimit(t *testing.T) {
	s := &Service{cfg: ServiceConfig{DefaultPageSize: 25, MaxPageSize: 100}}
	cases := []struct {
		in, want int
	}{
		{0, 25},
		{-1, 25},
		{50, 50},
		{100, 100},
		{500, 100},
	}
	for _, tc := range cases {
		got := s.clampLimit(tc.in)
		if got != tc.want {
			t.Errorf("clampLimit(%d) = %d; want %d", tc.in, got, tc.want)
		}
	}
}

// TestSummariseFallback verifies the fallback chain (title+content >
// title-only > content-only > app label) so we never emit an empty action
// summary even on garbage input.
func TestSummariseFallback(t *testing.T) {
	cases := []struct {
		ev   Event
		want string
	}{
		{Event{Title: "T", Content: "C"}, "T: C"},
		{Event{Title: "T"}, "T"},
		{Event{Content: "C"}, "C"},
		{Event{AppLabel: "WhatsApp"}, "WhatsApp"},
	}
	for _, tc := range cases {
		got := summarise(tc.ev)
		if got != tc.want {
			t.Errorf("summarise(%+v) = %q; want %q", tc.ev, got, tc.want)
		}
	}
}
