package relay

import (
	"encoding/json"

	"github.com/QuantumNous/new-api/common"
)

// stripHistoricalClaudeThinking removes only completed-turn thinking blocks. If
// the last genuine user input cannot be identified, it leaves the request alone.
// RawMessage preserves unknown values and numeric precision.
func stripHistoricalClaudeThinking(body []byte) ([]byte, bool) {
	var request map[string]json.RawMessage
	if common.Unmarshal(body, &request) != nil || request == nil {
		return body, false
	}
	var messages []json.RawMessage
	if common.Unmarshal(request["messages"], &messages) != nil {
		return body, false
	}
	cutoff := -1
	for i := len(messages) - 1; i >= 0; i-- {
		var msg map[string]json.RawMessage
		if common.Unmarshal(messages[i], &msg) != nil || msg == nil {
			return body, false
		}
		var role string
		if common.Unmarshal(msg["role"], &role) != nil {
			return body, false
		}
		if role != "user" {
			continue
		}
		genuine, known := claudeGenuineUserContent(msg["content"])
		if !known {
			return body, false
		}
		if genuine {
			cutoff = i
			break
		}
	}
	if cutoff < 0 {
		return body, false
	}
	changed := false
	for i := 0; i < cutoff; i++ {
		var msg map[string]json.RawMessage
		if common.Unmarshal(messages[i], &msg) != nil || msg == nil {
			return body, false
		}
		var role string
		if common.Unmarshal(msg["role"], &role) != nil {
			return body, false
		}
		if role != "assistant" {
			continue
		}
		var blocks []json.RawMessage
		if common.Unmarshal(msg["content"], &blocks) != nil || blocks == nil {
			continue
		}
		kept := make([]json.RawMessage, 0, len(blocks))
		removed := false
		for _, block := range blocks {
			var header struct {
				Type string `json:"type"`
			}
			if common.Unmarshal(block, &header) == nil && (header.Type == "thinking" || header.Type == "redacted_thinking") {
				removed = true
				continue
			}
			kept = append(kept, block)
		}
		// Recovery is atomic: keeping a signed-only message after changing
		// its prefix could invalidate that retained signature.
		if removed && len(kept) == 0 {
			return body, false
		}
		if !removed {
			continue
		}
		content, err := common.Marshal(kept)
		if err != nil {
			return body, false
		}
		msg["content"] = content
		encoded, err := common.Marshal(msg)
		if err != nil {
			return body, false
		}
		messages[i] = encoded
		changed = true
	}
	if !changed {
		return body, false
	}
	encoded, err := common.Marshal(messages)
	if err != nil {
		return body, false
	}
	request["messages"] = encoded
	result, err := common.Marshal(request)
	if err != nil {
		return body, false
	}
	return result, true
}

// A user message that contains only tool results continues the active turn.
// Unknown block kinds are ambiguous and must not shift the boundary.
func claudeGenuineUserContent(content json.RawMessage) (bool, bool) {
	var text string
	if len(content) > 0 && content[0] == '"' && common.Unmarshal(content, &text) == nil {
		return true, true
	}
	var blocks []json.RawMessage
	if common.Unmarshal(content, &blocks) != nil || len(blocks) == 0 {
		return false, false
	}
	genuine := false
	hasToolResult := false
	for _, block := range blocks {
		var header struct {
			Type string `json:"type"`
		}
		if common.Unmarshal(block, &header) != nil {
			return false, false
		}
		switch header.Type {
		case "tool_result":
			hasToolResult = true
		case "text", "image", "document", "video", "audio":
			genuine = true
		default:
			return false, false
		}
	}
	// Mixed tool results and user input are ambiguous: retain the whole turn.
	if genuine && hasToolResult {
		return false, false
	}
	return genuine, true
}
