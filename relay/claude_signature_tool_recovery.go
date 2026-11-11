package relay

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
)

// The controller can re-enter ClaudeHelper for another key on the same channel.
// Keep the compatibility retry budget on the whole inbound request, not the
// helper invocation. Different channels retain their own single attempt.
func claimClaudeSignatureRetry(c *gin.Context, channelID int) bool {
	const key = "claude_signature_retry_channels"
	var channels map[int]bool
	if value, ok := c.Get(key); ok {
		channels, _ = value.(map[int]bool)
	}
	if channels == nil {
		channels = make(map[int]bool)
		c.Set(key, channels)
	}
	if channels[channelID] {
		return false
	}
	channels[channelID] = true
	return true
}

// Status is checked before channel mappings. A normal 429 is never a signal to
// discard state. This is recovery from an explicit rejection, not validation of
// a signature (which cannot be performed locally).
func isClaudeThinkingSignatureRejection(err *types.NewAPIError) bool {
	return err != nil && (err.StatusCode == http.StatusBadRequest || err.StatusCode == http.StatusTooManyRequests) &&
		strings.Contains(err.Error(), "Invalid `signature` in `thinking` block")
}

// prepareClaudeSignatureRecovery keeps normal requests untouched. After an
// explicit rejection it prefers continuity over retaining unusable reasoning:
// for a fully paired client-tool continuation, remove ALL assistant thinking,
// not just early steps that could invalidate the retained prefix-bound blocks.
// This is a lossy best-effort fallback, NOT an upstream-supported equivalence;
// thinking configuration is retained and the upstream may still reject it.
func prepareClaudeSignatureRecovery(body []byte) ([]byte, bool, bool) {
	var request map[string]json.RawMessage
	if common.Unmarshal(body, &request) != nil || request == nil {
		return body, false, false
	}
	var messages []json.RawMessage
	if common.Unmarshal(request["messages"], &messages) != nil {
		return body, false, false
	}
	cutoff := -1
	for i := len(messages) - 1; i >= 0; i-- {
		var msg struct {
			Role    string          `json:"role"`
			Content json.RawMessage `json:"content"`
		}
		if common.Unmarshal(messages[i], &msg) != nil {
			return body, false, false
		}
		switch msg.Role {
		case "assistant":
		case "user":
			genuine, known := claudeGenuineUserContent(msg.Content)
			if !known {
				return body, false, false
			}
			if genuine {
				cutoff = i
			}
		default:
			return body, false, false
		}
		if cutoff >= 0 {
			break
		}
	}
	if cutoff < 0 {
		return body, false, false
	}
	// With no active thinking, preserve the existing completed-turn policy.
	activeThinking := false
	for _, raw := range messages[cutoff+1:] {
		var msg struct {
			Role    string `json:"role"`
			Content []struct {
				Type string `json:"type"`
			} `json:"content"`
		}
		if common.Unmarshal(raw, &msg) != nil {
			return body, false, false
		}
		if msg.Role == "assistant" {
			for _, block := range msg.Content {
				if block.Type == "thinking" || block.Type == "redacted_thinking" {
					activeThinking = true
				}
			}
		}
	}
	if !activeThinking {
		out, changed := stripHistoricalClaudeThinking(body)
		return out, changed, false
	}
	if !pairedClaudeToolContinuation(messages[cutoff+1:]) {
		return body, false, false
	}
	changed := false
	for i, raw := range messages {
		var msg map[string]json.RawMessage
		if common.Unmarshal(raw, &msg) != nil {
			return body, false, false
		}
		var role string
		if common.Unmarshal(msg["role"], &role) != nil {
			return body, false, false
		}
		if role != "assistant" {
			continue
		}
		var blocks []json.RawMessage
		if common.Unmarshal(msg["content"], &blocks) != nil {
			// A completed assistant reply may use the string shorthand. It has
			// no signed blocks and must stay unchanged, not block recovery.
			var text string
			if common.Unmarshal(msg["content"], &text) == nil {
				continue
			}
			return body, false, false
		}
		kept := make([]json.RawMessage, 0, len(blocks))
		for _, rawBlock := range blocks {
			var block struct {
				Type string `json:"type"`
			}
			if common.Unmarshal(rawBlock, &block) != nil {
				return body, false, false
			}
			if block.Type != "thinking" && block.Type != "redacted_thinking" {
				kept = append(kept, rawBlock)
			}
		}
		// Never invent text, drop a message, or leave a signed block behind after
		// changing its prefix. Abort the entire recovery if any message empties.
		if len(kept) == 0 {
			return body, false, false
		}
		if len(kept) == len(blocks) {
			continue
		}
		var err error
		msg["content"], err = common.Marshal(kept)
		if err != nil {
			return body, false, false
		}
		messages[i], err = common.Marshal(msg)
		if err != nil {
			return body, false, false
		}
		changed = true
	}
	if !changed {
		return body, false, false
	}
	var err error
	request["messages"], err = common.Marshal(messages)
	if err != nil {
		return body, false, false
	}
	out, err := common.Marshal(request)
	if err != nil {
		return body, false, false
	}
	return out, true, true
}

// Only known client tool calls/results are supported. No mixed user input,
// server tools, duplicate IDs, unpaired results or assistant prefill recovery.
func pairedClaudeToolContinuation(messages []json.RawMessage) bool {
	pending, seen := map[string]bool{}, map[string]bool{}
	lastRole := ""
	for _, raw := range messages {
		var msg struct {
			Role    string `json:"role"`
			Content []struct {
				Type      string `json:"type"`
				ID        string `json:"id"`
				ToolUseID string `json:"tool_use_id"`
			} `json:"content"`
		}
		if common.Unmarshal(raw, &msg) != nil || len(msg.Content) == 0 {
			return false
		}
		if msg.Role == "assistant" && lastRole == "user" && len(pending) != 0 {
			return false
		}
		for _, block := range msg.Content {
			switch msg.Role {
			case "assistant":
				switch block.Type {
				case "text", "thinking", "redacted_thinking":
				case "tool_use":
					if block.ID == "" || seen[block.ID] {
						return false
					}
					pending[block.ID], seen[block.ID] = true, true
				default:
					return false
				}
			case "user":
				if block.Type != "tool_result" || !pending[block.ToolUseID] {
					return false
				}
				delete(pending, block.ToolUseID)
			default:
				return false
			}
		}
		lastRole = msg.Role
	}
	return lastRole == "user" && len(pending) == 0 && len(seen) > 0
}
