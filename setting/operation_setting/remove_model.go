package operation_setting

import (
	"strings"
	"sync/atomic"
)

var automaticRemoveModelEnabled atomic.Bool
var automaticRemoveModelKeywords atomic.Value

func init() {
	automaticRemoveModelEnabled.Store(true)
	AutomaticRemoveModelKeywordsFromString("This Model is marked by provider as Legacy")
}
func AutomaticRemoveModelEnabled() bool     { return automaticRemoveModelEnabled.Load() }
func SetAutomaticRemoveModelEnabled(v bool) { automaticRemoveModelEnabled.Store(v) }
func AutomaticRemoveModelKeywordsToString() string {
	return strings.Join(automaticRemoveModelKeywords.Load().([]string), "\n")
}
func AutomaticRemoveModelKeywordsFromString(s string) {
	automaticRemoveModelKeywords.Store(normalizeRetryKeywords(strings.Split(strings.ReplaceAll(s, "\r\n", "\n"), "\n")))
}
func ShouldRemoveModel(message string) bool {
	if !AutomaticRemoveModelEnabled() {
		return false
	}
	message = strings.ToLower(message)
	for _, keyword := range automaticRemoveModelKeywords.Load().([]string) {
		if strings.Contains(message, keyword) {
			return true
		}
	}
	return false
}
