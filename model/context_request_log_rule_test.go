package model

import "testing"

func intPtr(v int) *int { return &v }

func TestWildcardModelMatchOnlyAsteriskIsSpecial(t *testing.T) {
	cases := []struct {
		pattern string
		value   string
		match   bool
	}{
		{"openrouter/*", "OpenRouter/anthropic/claude", true},
		{"gpt-4?", "gpt-4?", true},
		{"gpt-4?", "gpt-4o", false},
		{"model[1]", "model[1]", true},
		{"model[1]", "model1", false},
	}
	for _, tc := range cases {
		if got := wildcardModelMatch(tc.pattern, tc.value); got != tc.match {
			t.Fatalf("wildcardModelMatch(%q, %q) = %v, want %v", tc.pattern, tc.value, got, tc.match)
		}
	}
}
func TestMatchContextLogRules(t *testing.T) {
	rules := []ContextRequestLogRule{
		{Id: 10, Name: "model", ModelPattern: "GPT-*", Decision: ContextLogDecisionCapture, Enabled: true, Priority: 1},
		{Id: 20, Name: "user", UserId: intPtr(7), Decision: ContextLogDecisionSkip, Enabled: true, Priority: 99},
		{Id: 30, Name: "both-low", UserId: intPtr(7), ModelPattern: "gpt-4o", Decision: ContextLogDecisionSkip, Enabled: true, Priority: 1},
		{Id: 31, Name: "both-high", UserId: intPtr(7), ModelPattern: "GPT-4*", Decision: ContextLogDecisionCapture, Enabled: true, Priority: 2},
		{Id: 40, Name: "disabled", UserId: intPtr(8), Decision: ContextLogDecisionCapture, Enabled: false, Priority: 100},
	}
	cases := []struct {
		name    string
		user    int
		model   string
		def     bool
		capture bool
		id      int
	}{
		{"user model wins and case insensitive wildcard", 7, "GpT-4O", false, true, 31},
		{"user only beats model only", 7, "gpt-3.5", true, false, 20},
		{"model only", 9, "GPT-mini", false, true, 10},
		{"disabled and fallback", 8, "claude", false, false, 0},
		{"global fallback", 9, "claude", true, true, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := MatchContextLogRules(rules, tc.user, tc.model, tc.def)
			if got.Capture != tc.capture || got.RuleId != tc.id {
				t.Fatalf("got %+v", got)
			}
		})
	}
}
