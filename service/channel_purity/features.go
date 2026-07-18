package channel_purity

import (
	"encoding/json"
	"net/http"
	"sort"
	"strings"
)

// AnonymousFeatures contains protocol-only evidence. It never contains response text,
// reasoning, tool arguments, identifiers, or header values.
type AnonymousFeatures struct {
	Protocol          string          `json:"protocol"`
	StatusCode        int             `json:"status_code"`
	ModelFamily       string          `json:"model_family,omitempty"`
	FieldPaths        []string        `json:"field_paths,omitempty"`
	EventSequence     []string        `json:"event_sequence,omitempty"`
	FinishReasons     []string        `json:"finish_reasons,omitempty"`
	ProviderUsage     TokenUsage      `json:"provider_usage"`
	UnifiedTokenCount int             `json:"unified_token_count"`
	HeaderPresence    map[string]bool `json:"header_presence,omitempty"`
	HasSignatureID    bool            `json:"has_signature_id,omitempty"`
	Truncated         bool            `json:"truncated,omitempty"`
}

type TokenUsage struct {
	Input  int `json:"input"`
	Output int `json:"output"`
	Total  int `json:"total"`
}

var trustedEvidenceHeaders = []string{
	"content-type", "openai-processing-ms", "x-request-id",
	"x-reasoning-included", "x-codex-turn-state",
}

func ExtractAnonymousFeatures(status int, header http.Header, body []byte, truncated bool) AnonymousFeatures {
	f := AnonymousFeatures{Protocol: "json", StatusCode: status, HeaderPresence: map[string]bool{}, Truncated: truncated}
	for _, name := range trustedEvidenceHeaders {
		f.HeaderPresence[name] = strings.TrimSpace(header.Get(name)) != ""
	}
	trimmed := strings.TrimSpace(string(body))
	if strings.Contains(strings.ToLower(header.Get("Content-Type")), "text/event-stream") || strings.HasPrefix(trimmed, "data:") || strings.HasPrefix(trimmed, "event:") {
		f.Protocol = "sse"
		extractSSEFeatures([]byte(trimmed), &f)
	} else {
		extractJSONFeatures(body, &f)
	}
	f.FieldPaths = uniqueSorted(f.FieldPaths)
	f.FinishReasons = uniqueSorted(f.FinishReasons)
	return f
}

func extractSSEFeatures(body []byte, f *AnonymousFeatures) {
	var eventName string
	for _, raw := range strings.Split(string(body), "\n") {
		line := strings.TrimSpace(strings.TrimSuffix(raw, "\r"))
		switch {
		case strings.HasPrefix(line, "event:"):
			eventName = safeEnum(strings.TrimSpace(strings.TrimPrefix(line, "event:")))
		case strings.HasPrefix(line, "data:"):
			data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			if data == "" || data == "[DONE]" {
				if data == "[DONE]" {
					f.EventSequence = appendBounded(f.EventSequence, "done", 128)
				}
				continue
			}
			var value any
			if json.Unmarshal([]byte(data), &value) != nil {
				continue
			}
			if eventName == "" {
				eventName = eventType(value)
			}
			f.EventSequence = appendBounded(f.EventSequence, eventName, 128)
			collectJSON(value, "", f)
			eventName = ""
		}
	}
}

func extractJSONFeatures(body []byte, f *AnonymousFeatures) {
	var value any
	if json.Unmarshal(body, &value) == nil {
		collectJSON(value, "", f)
	}
}

var sensitiveLeaf = map[string]bool{
	"content": true, "text": true, "delta": true, "arguments": true,
	"reasoning": true, "reasoning_content": true, "encrypted_content": true,
	"input": true, "instructions": true, "output_text": true,
}

func collectJSON(value any, path string, f *AnonymousFeatures) {
	switch current := value.(type) {
	case map[string]any:
		keys := make([]string, 0, len(current))
		for key := range current {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			lower := strings.ToLower(key)
			childPath := lower
			if path != "" {
				childPath = path + "." + lower
			}
			f.FieldPaths = appendBounded(f.FieldPaths, childPath, 512)
			if lower == "signature_id" {
				f.HasSignatureID = true
			}
			if sensitiveLeaf[lower] {
				continue
			}
			switch lower {
			case "model":
				f.ModelFamily = ModelFamily(stringValue(current[key]))
			case "finish_reason", "stop_reason", "status":
				if v := safeEnum(stringValue(current[key])); v != "" {
					f.FinishReasons = append(f.FinishReasons, v)
				}
			case "usage":
				collectUsage(current[key], &f.ProviderUsage)
			}
			collectJSON(current[key], childPath, f)
		}
	case []any:
		for i, item := range current {
			if i >= 16 {
				break
			}
			collectJSON(item, path+"[]", f)
		}
	}
	f.UnifiedTokenCount = f.ProviderUsage.Total
	if f.UnifiedTokenCount == 0 {
		f.UnifiedTokenCount = f.ProviderUsage.Input + f.ProviderUsage.Output
	}
}

func collectUsage(value any, usage *TokenUsage) {
	m, ok := value.(map[string]any)
	if !ok {
		return
	}
	usage.Input = firstInt(m, "prompt_tokens", "input_tokens")
	usage.Output = firstInt(m, "completion_tokens", "output_tokens")
	usage.Total = firstInt(m, "total_tokens")
}
func firstInt(m map[string]any, keys ...string) int {
	for _, k := range keys {
		if n, ok := m[k].(float64); ok && n >= 0 {
			return int(n)
		}
	}
	return 0
}
func stringValue(v any) string { s, _ := v.(string); return s }
func eventType(v any) string {
	if m, ok := v.(map[string]any); ok {
		return safeEnum(stringValue(m["type"]))
	}
	return "message"
}
func safeEnum(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	if len(s) > 80 {
		return ""
	}
	for _, r := range s {
		if !(r == '.' || r == '_' || r == '-' || r >= 'a' && r <= 'z' || r >= '0' && r <= '9') {
			return ""
		}
	}
	return s
}
func appendBounded(values []string, value string, max int) []string {
	if value != "" && len(values) < max {
		return append(values, value)
	}
	return values
}
func uniqueSorted(values []string) []string {
	set := map[string]struct{}{}
	for _, v := range values {
		if v != "" {
			set[v] = struct{}{}
		}
	}
	out := make([]string, 0, len(set))
	for v := range set {
		out = append(out, v)
	}
	sort.Strings(out)
	return out
}

func ModelFamily(model string) string {
	model = strings.ToLower(strings.TrimSpace(strings.TrimPrefix(model, "models/")))
	parts := strings.FieldsFunc(model, func(r rune) bool { return r == '-' || r == ':' || r == '/' })
	if len(parts) == 0 {
		return ""
	}
	if len(parts) > 1 && (parts[0] == "gpt" || parts[0] == "claude" || parts[0] == "gemini") {
		return parts[0] + "-" + parts[1]
	}
	return parts[0]
}
