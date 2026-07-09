package common

import (
	"strings"

	rootcommon "github.com/QuantumNous/new-api/common"
)

func RemoveMissingResponsesReasoningItem(data []byte, itemID string) ([]byte, int, error) {
	itemID = strings.TrimSpace(itemID)
	if len(data) == 0 || itemID == "" {
		return data, 0, nil
	}

	var payload map[string]any
	if err := rootcommon.Unmarshal(data, &payload); err != nil {
		return data, 0, err
	}
	input, ok := payload["input"].([]any)
	if !ok || len(input) == 0 {
		return data, 0, nil
	}

	filtered := make([]any, 0, len(input))
	removed := 0
	for _, rawItem := range input {
		item, ok := rawItem.(map[string]any)
		if ok && isEmptyMissingResponsesReasoningItem(item, itemID) {
			removed++
			continue
		}
		filtered = append(filtered, rawItem)
	}
	if removed == 0 {
		return data, 0, nil
	}

	payload["input"] = filtered
	normalized, err := rootcommon.Marshal(payload)
	if err != nil {
		return data, 0, err
	}
	return normalized, removed, nil
}

func isEmptyMissingResponsesReasoningItem(item map[string]any, itemID string) bool {
	if !strings.EqualFold(strings.TrimSpace(rootcommon.Interface2String(item["type"])), "reasoning") {
		return false
	}
	if strings.TrimSpace(rootcommon.Interface2String(item["id"])) != itemID {
		return false
	}
	if _, exists := item["encrypted_content"]; exists {
		return false
	}
	if !isEmptyResponsesReasoningValue(item["summary"]) || !isEmptyResponsesReasoningValue(item["content"]) {
		return false
	}

	for key, value := range item {
		switch strings.ToLower(strings.TrimSpace(key)) {
		case "type", "id", "status", "summary", "content":
			continue
		default:
			if !isEmptyResponsesReasoningValue(value) {
				return false
			}
		}
	}
	return true
}

func isEmptyResponsesReasoningValue(value any) bool {
	switch typed := value.(type) {
	case nil:
		return true
	case string:
		return strings.TrimSpace(typed) == ""
	case []any:
		return len(typed) == 0
	case map[string]any:
		return len(typed) == 0
	default:
		return false
	}
}
