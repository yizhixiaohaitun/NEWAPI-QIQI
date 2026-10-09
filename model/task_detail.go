package model

import (
	"encoding/json"
	"net/url"
	"regexp"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/types"
)

var taskDetailSecretTextPattern = regexp.MustCompile(`(?i)(authorization|api[-_ ]?key|access[-_ ]?token|refresh[-_ ]?token|secret|password|credential|cookie)(\s*[:=]\s*)([^\s,;]+)`)
var taskDetailBearerPattern = regexp.MustCompile(`(?i)bearer\s+[A-Za-z0-9._~+\-/=]+`)

func isTaskDetailSecretKey(key string) bool {
	normalized := strings.ToLower(strings.NewReplacer("_", "", "-", "", " ", "").Replace(key))
	switch normalized {
	case "authorization", "apikey", "accesskey", "secretkey", "token", "accesstoken", "refreshtoken", "secret", "password", "credential", "credentials", "cookie", "setcookie", "xapikey", "key":
		return true
	default:
		return false
	}
}

func sanitizeTaskDetailURL(value string) string {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return value
	}
	query := parsed.Query()
	changed := false
	for key := range query {
		if isTaskDetailSecretKey(key) {
			query.Set(key, "[REDACTED]")
			changed = true
		}
	}
	if changed {
		parsed.RawQuery = query.Encode()
	}
	return parsed.String()
}

// SanitizeTaskDetailText redacts credentials from free-form task fields and
// query parameters without hiding ordinary signed media URLs.
func SanitizeTaskDetailText(value string) string {
	trimmed := strings.TrimSpace(value)
	if strings.HasPrefix(strings.ToLower(trimmed), "data:") {
		return "[OMITTED DATA URL]"
	}
	value = taskDetailBearerPattern.ReplaceAllString(value, "Bearer [REDACTED]")
	value = taskDetailSecretTextPattern.ReplaceAllString(value, "$1$2[REDACTED]")
	return sanitizeTaskDetailURL(value)
}

func sanitizeTaskDetailValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		result := make(map[string]any, len(typed))
		for key, item := range typed {
			if isTaskDetailSecretKey(key) {
				result[key] = "[REDACTED]"
				continue
			}
			if strings.Contains(strings.ToLower(key), "base64") {
				result[key] = "[OMITTED BASE64]"
				continue
			}
			result[key] = sanitizeTaskDetailValue(item)
		}
		return result
	case []any:
		result := make([]any, len(typed))
		for i, item := range typed {
			result[i] = sanitizeTaskDetailValue(item)
		}
		return result
	case string:
		return SanitizeTaskDetailText(typed)
	default:
		return value
	}
}

// SanitizeTaskPublicErrorJSON rewrites group names only in explicit task
// failure fields. Normal prompts, generated output, and successful data remain
// unchanged.
func SanitizeTaskPublicErrorJSON(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 || string(raw) == "null" {
		return raw
	}
	var value any
	if err := common.Unmarshal(raw, &value); err != nil {
		return raw
	}
	if !sanitizeTaskPublicErrorValue(value, false) {
		return raw
	}
	sanitized, err := common.Marshal(value)
	if err != nil {
		return raw
	}
	return json.RawMessage(sanitized)
}

func sanitizeTaskPublicErrorValue(value any, inError bool) bool {
	changed := false
	switch typed := value.(type) {
	case map[string]any:
		status, _ := typed["status"].(string)
		failed := strings.EqualFold(status, "failed") || strings.EqualFold(status, "failure")
		for key, child := range typed {
			normalizedKey := strings.ToLower(strings.ReplaceAll(key, "_", ""))
			diagnosticField := normalizedKey == "error" || normalizedKey == "failreason" || normalizedKey == "message" || normalizedKey == "details" || normalizedKey == "metadata"
			childInError := inError || normalizedKey == "error" || (failed && diagnosticField)
			if text, ok := child.(string); ok && (childInError || normalizedKey == "failreason") {
				if message, matched := types.PublicGroupErrorMessage(text); matched {
					typed[key] = message
					changed = true
				}
				continue
			}
			if sanitizeTaskPublicErrorValue(child, childInError) {
				changed = true
			}
		}
	case []any:
		for i, child := range typed {
			if text, ok := child.(string); ok && inError {
				if message, matched := types.PublicGroupErrorMessage(text); matched {
					typed[i] = message
					changed = true
				}
				continue
			}
			if sanitizeTaskPublicErrorValue(child, inError) {
				changed = true
			}
		}
	}
	return changed
}

// SanitizeTaskDetailJSON returns a safe JSON copy suitable for persistence or
// task-detail responses. Invalid JSON is intentionally omitted rather than
// returned verbatim, because its contents cannot be inspected for secrets.
func SanitizeTaskDetailJSON(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	var value any
	if err := common.Unmarshal(raw, &value); err != nil {
		return nil
	}
	sanitized, err := common.Marshal(sanitizeTaskDetailValue(value))
	if err != nil {
		return nil
	}
	return json.RawMessage(sanitized)
}
