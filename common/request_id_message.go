package common

import (
	"regexp"
	"strings"
)

var trailingRequestIDs = regexp.MustCompile(`(?i)(?:\s*\(request id:\s*[^()]*\))+\s*$`)

// StripTrailingRequestIDs removes only the gateway's known suffix syntax.
// Embedded IDs or ordinary message/response text are not globally replaced.
func StripTrailingRequestIDs(message string) string {
	return strings.TrimRight(trailingRequestIDs.ReplaceAllString(message, ""), " \t\r\n")
}
