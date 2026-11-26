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
	return stripClaudeThinkingBeforeUser(body, request, messages, cutoff, false)
}

func stripClaudeThinkingBeforeUser(body []byte, request map[string]json.RawMessage, messages []json.RawMessage, cutoff int, allowThinkingOnlyMessageRemoval bool) ([]byte, bool) {
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
		// its prefix could invalidate that retained signature. Only an explicit
		// signature-rejection retry may omit a completed, thinking-only message.
		if removed && len(kept) == 0 {
			if allowThinkingOnlyMessageRemoval && isClaudeThinkingOnlyMessage(msg, blocks) {
				messages[i] = nil
				changed = true
				continue
			}
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
	if allowThinkingOnlyMessageRemoval {
		keptMessages := make([]json.RawMessage, 0, len(messages))
		for _, message := range messages {
			if message != nil {
				keptMessages = append(keptMessages, message)
			}
		}
		messages = keptMessages
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

// A thinking-only assistant message has no visible text or tool semantics to
// preserve. Validate its complete shape before allowing rejection-specific omission.
func isClaudeThinkingOnlyMessage(message map[string]json.RawMessage, blocks []json.RawMessage) bool {
	if len(message) != 2 || len(blocks) == 0 {
		return false
	}
	if _, ok := message["content"]; !ok {
		return false
	}
	var role string
	if common.Unmarshal(message["role"], &role) != nil || role != "assistant" {
		return false
	}
	for _, raw := range blocks {
		var block map[string]json.RawMessage
		if common.Unmarshal(raw, &block) != nil || block == nil {
			return false
		}
		var kind string
		if common.Unmarshal(block["type"], &kind) != nil {
			return false
		}
		switch kind {
		case "thinking":
			if len(block) < 2 || len(block) > 3 {
				return false
			}
			for key := range block {
				if key != "type" && key != "thinking" && key != "signature" {
					return false
				}
			}
			// Upstream can return an empty string when it supplies an opaque
			// signature without a thinking summary. Missing/null is ambiguous.
			var thinking *string
			if common.Unmarshal(block["thinking"], &thinking) != nil || thinking == nil {
				return false
			}
			if signature, ok := block["signature"]; ok {
				var value *string
				if common.Unmarshal(signature, &value) != nil || value == nil {
					return false
				}
			}
		case "redacted_thinking":
			if len(block) != 2 {
				return false
			}
			for key := range block {
				if key != "type" && key != "data" {
					return false
				}
			}
			var data string
			if common.Unmarshal(block["data"], &data) != nil || data == "" {
				return false
			}
		default:
			return false
		}
	}
	return true
}

// A user message that contains only tool results continues the active turn.
// Unknown block kinds are ambiguous and must not shift the boundary.
func claudeGenuineUserContent(content json.RawMessage) (bool, bool) {
	return claudeSignatureUserBoundary(content, false)
}

// Mixed tool-result/text messages remain continuations, never new user turns.
// Only the explicit-rejection recovery opts in; historical cleanup stays strict.
func claudeSignatureUserBoundary(content json.RawMessage, allowMixedToolText bool) (bool, bool) {
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
	hasMedia := false
	for _, block := range blocks {
		var header struct {
			Type string  `json:"type"`
			Text *string `json:"text"`
		}
		if common.Unmarshal(block, &header) != nil {
			return false, false
		}
		switch header.Type {
		case "tool_result":
			hasToolResult = true
		case "text":
			if header.Text == nil {
				return false, false
			}
			genuine = true
		case "image", "document", "video", "audio":
			genuine, hasMedia = true, true
		default:
			return false, false
		}
	}
	// Mixed tool results and text continue the active turn. They are known for
	// the explicit tool-recovery validator, but never become the cutoff/user
	// boundary; the preceding assistant thinking must remain in that turn.
	if genuine && hasToolResult {
		return false, allowMixedToolText && !hasMedia
	}
	return genuine, true
}
