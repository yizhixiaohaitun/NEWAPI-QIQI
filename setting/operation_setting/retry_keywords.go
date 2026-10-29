package operation_setting

import (
	"strings"
	"sync/atomic"
)

// DefaultAutomaticRetryKeywords are upstream error fragments that should be
// retried on another channel even when their status code is normally final
// (for example the 400 "prompt flagged" response some upstreams return
// intermittently for prompts other channels accept).
var DefaultAutomaticRetryKeywords = []string{
	"your prompt was flagged as potentially violating our usage policy",
}

var automaticRetryKeywords atomic.Value // []string, stored lower-cased

func init() {
	automaticRetryKeywords.Store(normalizeRetryKeywords(DefaultAutomaticRetryKeywords))
}

func normalizeRetryKeywords(items []string) []string {
	out := make([]string, 0, len(items))
	seen := make(map[string]struct{}, len(items))
	for _, item := range items {
		item = strings.ToLower(strings.TrimSpace(item))
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		out = append(out, item)
	}
	return out
}

func getAutomaticRetryKeywords() []string {
	v, _ := automaticRetryKeywords.Load().([]string)
	return v
}

// AutomaticRetryKeywordsToString returns one keyword per line.
func AutomaticRetryKeywordsToString() string {
	return strings.Join(getAutomaticRetryKeywords(), "\n")
}

// AutomaticRetryKeywordsFromString replaces the keyword list. Each non-empty
// line is one keyword; matching is case-insensitive substring matching.
func AutomaticRetryKeywordsFromString(s string) {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	automaticRetryKeywords.Store(normalizeRetryKeywords(strings.Split(s, "\n")))
}

// ShouldRetryByErrorMessage reports whether any configured keyword appears in
// one of the given error messages.
func ShouldRetryByErrorMessage(messages ...string) bool {
	keywords := getAutomaticRetryKeywords()
	if len(keywords) == 0 {
		return false
	}
	for _, message := range messages {
		if message == "" {
			continue
		}
		lower := strings.ToLower(message)
		for _, keyword := range keywords {
			if strings.Contains(lower, keyword) {
				return true
			}
		}
	}
	return false
}
