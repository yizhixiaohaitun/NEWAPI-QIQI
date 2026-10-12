package common

// Domains/URLs containing a field path are NOT exempt. The historical domain
// regex may match only a substring of an underscore-containing field; inspect
// the whole delimiter-bounded token before allowing that exact field name.
func isProtectedProtocolPath(text string, start, end int) bool {
	isFieldChar := func(c byte) bool {
		return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_' || c == '-' || c == '.' || c == '@' || c == '/' || c == '\\'
	}
	for start > 0 && isFieldChar(text[start-1]) {
		start--
	}
	for end < len(text) && isFieldChar(text[end]) {
		end++
	}
	switch text[start:end] {
	case "thinking.type.enabled", "thinking.type.adaptive", "thinking.type.disabled", "thinking.budget_tokens", "output_config.effort":
		return true
	}
	return false
}
