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

func sanitizeTaskPublicErrorValue(value any, _ bool) bool {
	if items, ok := value.([]any); ok {
		changed := false
		for _, item := range items {
			if sanitizeTaskPublicErrorValue(item, false) {
				changed = true
			}
		}
		return changed
	}
	typed, ok := value.(map[string]any)
	if !ok {
		return false
	}

	changed := false
	explicitErrorMatched := false
	for key, child := range typed {
		normalizedKey := normalizeTaskDetailKey(key)
		if normalizedKey == "error" {
			if publicError, matched := sanitizeStructuredTaskError(child); matched {
				typed[key] = publicError
				changed = true
				explicitErrorMatched = true
				continue
			}
		}
		if sanitizeTaskPublicErrorValue(child, false) {
			changed = true
		}
	}

	publicMessages := publicFlatTaskFailures(typed)
	flatFailureMatched := len(publicMessages) > 0
	if !explicitErrorMatched && !flatFailureMatched {
		return changed
	}

	// Once an explicit error object/array or flat failure field is recognized
	// as a group error, rebuild its parent object rather than replacing strings
	// recursively. This prevents the same group name from surviving in sibling
	// metadata/details/debug fields while retaining ordinary task identity,
	// state, prompt and successful output fields.
	for key, child := range typed {
		normalizedKey := normalizeTaskDetailKey(key)
		if normalizedKey == "error" && explicitErrorMatched {
			continue
		}
		if isPublicTaskResultKey(normalizedKey) {
			if isDiagnosticTaskResultKey(normalizedKey) {
				if text, ok := child.(string); ok {
					if message, matched := types.PublicGroupErrorMessage(text); matched {
						typed[key] = message
					}
				}
			}
			continue
		}
		if message, matched := publicMessages[key]; matched {
			typed[key] = message
			continue
		}
		delete(typed, key)
	}
	return true
}

func sanitizeStructuredTaskError(value any) (any, bool) {
	envelope, err := common.Marshal(map[string]any{"type": "error", "error": value})
	if err != nil {
		return value, false
	}
	sanitized, changed := types.SanitizeGroupErrorStreamData(string(envelope))
	if !changed {
		return value, false
	}
	var publicEnvelope map[string]any
	if err := common.Unmarshal([]byte(sanitized), &publicEnvelope); err != nil {
		return value, false
	}
	publicError, ok := publicEnvelope["error"]
	return publicError, ok
}

func publicFlatTaskFailures(value map[string]any) map[string]string {
	messages := make(map[string]string)
	for key, child := range value {
		normalizedKey := normalizeTaskDetailKey(key)
		if normalizedKey != "message" && normalizedKey != "failreason" {
			continue
		}
		text, ok := child.(string)
		if !ok {
			continue
		}
		if message, matched := types.PublicGroupErrorMessage(text); matched {
			messages[key] = message
		}
	}
	return messages
}

func normalizeTaskDetailKey(key string) string {
	return strings.ToLower(strings.NewReplacer("_", "", "-", "", " ", "").Replace(key))
}

func isPublicTaskResultKey(key string) bool {
	switch key {
	case "id", "taskid", "status", "state", "progress", "prompt", "output", "result", "resulturl", "url", "createdat", "updatedat", "submittime", "starttime", "finishtime":
		return true
	default:
		return false
	}
}

func isDiagnosticTaskResultKey(key string) bool {
	switch key {
	case "result", "resulturl", "url":
		return true
	default:
		return false
	}
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
