package channel_purity

import (
	"crypto/rand"
	"encoding/binary"
	"encoding/json"
	"strings"
)

const DefaultMaxSnapshotBytes = 256 << 10

type CaptureDecision struct {
	Eligible bool
	Reason   string
	PairID   string
	Sampled  bool
}

type CapturePolicy struct {
	SampleRate       float64
	MaxSnapshotBytes int
	Store            *PairStore
}

func IsDetectionRequest(headers map[string][]string) bool {
	for name, values := range headers {
		if strings.EqualFold(name, DetectionHeader) {
			for _, value := range values {
				if strings.TrimSpace(value) != "" {
					return true
				}
			}
		}
	}
	return false
}

func (p CapturePolicy) Capture(group, requestType string, body []byte, detection bool) CaptureDecision {
	if detection {
		return CaptureDecision{Reason: "detection_request"}
	}
	max := p.MaxSnapshotBytes
	if max <= 0 {
		max = DefaultMaxSnapshotBytes
	}
	if len(body) == 0 || len(body) > max {
		return CaptureDecision{Reason: "size"}
	}
	model, reason := inspectSafeRequest(body)
	if reason != "" {
		return CaptureDecision{Reason: reason}
	}
	if p.Store == nil || p.SampleRate <= 0 || !randomSample(p.SampleRate) {
		return CaptureDecision{Eligible: true, Reason: "not_sampled"}
	}
	id, err := p.Store.Put(group, ModelFamily(model), requestType, body)
	if err != nil {
		return CaptureDecision{Eligible: true, Reason: "store_failed"}
	}
	return CaptureDecision{Eligible: true, Sampled: true, PairID: id}
}

func inspectSafeRequest(body []byte) (string, string) {
	var root map[string]any
	if json.Unmarshal(body, &root) != nil {
		return "", "invalid_json"
	}
	if _, ok := root["file"]; ok {
		return "", "file"
	}
	if containsSensitive(root) {
		return "", "sensitive_or_multimedia"
	}
	if tools, ok := root["tools"].([]any); ok {
		for _, raw := range tools {
			tool, _ := raw.(map[string]any)
			typeName, _ := tool["type"].(string)
			if typeName != "function" {
				return "", "side_effect_tool"
			}
			fn, _ := tool["function"].(map[string]any)
			strict, _ := fn["strict"].(bool)
			if !strict {
				return "", "side_effect_tool"
			}
		}
	}
	model, _ := root["model"].(string)
	return model, ""
}

func containsSensitive(v any) bool {
	switch x := v.(type) {
	case map[string]any:
		for key, value := range x {
			lower := strings.ToLower(key)
			if lower == "image_url" || lower == "input_audio" || lower == "audio" || lower == "video" || lower == "file_id" || lower == "file_data" || lower == "attachments" {
				return true
			}
			if lower == "type" {
				if s, ok := value.(string); ok && (strings.Contains(s, "image") || strings.Contains(s, "audio") || strings.Contains(s, "video") || strings.Contains(s, "file")) {
					return true
				}
			}
			if containsSensitive(value) {
				return true
			}
		}
	case []any:
		for _, item := range x {
			if containsSensitive(item) {
				return true
			}
		}
	}
	return false
}

func randomSample(rate float64) bool {
	if rate >= 1 {
		return true
	}
	var raw [8]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return false
	}
	return float64(binary.BigEndian.Uint64(raw[:]))/float64(^uint64(0)) < rate
}
