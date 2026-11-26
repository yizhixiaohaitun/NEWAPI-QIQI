package relay

import (
	"encoding/json"
	"net/http"
	"strings"
	"sync"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
)

// ClaudeSignatureRetryClaimer returns the inbound request's per-channel
// compatibility budget. The controller obtains it before copying the context
// for any race lanes, so those lanes and later key selection share one budget.
func ClaudeSignatureRetryClaimer(c *gin.Context) func(channelID int) bool {
	const key = "claude_signature_retry_claim"
	if existing, ok := c.Get(key); ok {
		if claim, ok := existing.(func(int) bool); ok {
			return claim
		}
	}
	var channels sync.Map
	claim := func(channelID int) bool {
		_, alreadyRetried := channels.LoadOrStore(channelID, struct{}{})
		return !alreadyRetried
	}
	c.Set(key, claim)
	return claim
}

func claimClaudeSignatureRetry(c *gin.Context, info *relaycommon.RelayInfo) bool {
	if info.ClaimClaudeSignatureRetry != nil {
		return info.ClaimClaudeSignatureRetry(info.ChannelId)
	}
	return ClaudeSignatureRetryClaimer(c)(info.ChannelId)
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
		case "assistant", "system":
			// Preserve system messages in place. They do not establish a user
			// boundary and must not block completed-turn history recovery.
		case "user":
			genuine, known := claudeSignatureUserBoundary(msg.Content, true)
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
	// Only extend the historical fallback to mixed or system-interleaved
	// tool continuations after the same pairing checks as active recovery.
	activeThinking := false
	hasSystem, hasContinuation, hasMixed := false, false, false
	for _, raw := range messages[cutoff+1:] {
		var msg struct {
			Role    string          `json:"role"`
			Content json.RawMessage `json:"content"`
		}
		if common.Unmarshal(raw, &msg) != nil {
			return body, false, false
		}
		if msg.Role == "system" {
			hasSystem = true
			continue
		}
		hasContinuation = true
		if msg.Role == "user" {
			_, plain := claudeGenuineUserContent(msg.Content)
			hasMixed = hasMixed || !plain // This tail was already checked with mixed results allowed.
			continue
		}
		var blocks []struct {
			Type string `json:"type"`
		}
		if common.Unmarshal(msg.Content, &blocks) != nil {
			return body, false, false
		}
		for _, block := range blocks {
			if block.Type == "thinking" || block.Type == "redacted_thinking" {
				activeThinking = true
			}
		}
	}
	if (activeThinking || hasMixed || hasSystem && hasContinuation) && !pairedClaudeToolContinuation(messages[cutoff+1:]) {
		return body, false, false
	}
	if !activeThinking {
		// The rejected response permits omission of a completed assistant
		// message only when it consists entirely of known thinking blocks.
		out, changed := stripClaudeThinkingBeforeUser(body, request, messages, cutoff, true)
		return out, changed, false
	}
	changed := false
	cleanedMessages := make([]json.RawMessage, 0, len(messages))
	for i, raw := range messages {
		var msg map[string]json.RawMessage
		if common.Unmarshal(raw, &msg) != nil || msg == nil {
			return body, false, false
		}
		var role string
		if common.Unmarshal(msg["role"], &role) != nil {
			return body, false, false
		}
		if role != "assistant" {
			cleanedMessages = append(cleanedMessages, raw)
			continue
		}
		var blocks []json.RawMessage
		if common.Unmarshal(msg["content"], &blocks) != nil {
			// A completed assistant reply may use the string shorthand. It has
			// no signed blocks and must stay unchanged, not block recovery.
			var text string
			if common.Unmarshal(msg["content"], &text) == nil {
				cleanedMessages = append(cleanedMessages, raw)
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
		if len(kept) == 0 {
			// Only a safely recognized completed historical thinking-only
			// message may disappear. Never drop active-turn content.
			if i >= cutoff || !isClaudeThinkingOnlyMessage(msg, blocks) {
				return body, false, false
			}
			changed = true
			continue
		}
		if len(kept) == len(blocks) {
			cleanedMessages = append(cleanedMessages, raw)
			continue
		}
		var err error
		msg["content"], err = common.Marshal(kept)
		if err != nil {
			return body, false, false
		}
		encoded, err := common.Marshal(msg)
		if err != nil {
			return body, false, false
		}
		cleanedMessages = append(cleanedMessages, encoded)
		changed = true
	}
	if !changed {
		return body, false, false
	}
	var err error
	request["messages"], err = common.Marshal(cleanedMessages)
	if err != nil {
		return body, false, false
	}
	out, err := common.Marshal(request)
	if err != nil {
		return body, false, false
	}
	return out, true, true
}

// Only known, fully paired client tools are supported. Text after the results
// and text-only system messages stay in place and never start a new user turn.
// Reject server tools, duplicate IDs, unpaired results and assistant prefill.
func pairedClaudeToolContinuation(messages []json.RawMessage) bool {
	pending, seen := map[string]bool{}, map[string]bool{}
	lastRole := ""
	for _, raw := range messages {
		var msg struct {
			Role    string          `json:"role"`
			Content json.RawMessage `json:"content"`
		}
		if common.Unmarshal(raw, &msg) != nil {
			return false
		}
		if msg.Role == "system" {
			var text string
			if common.GetJsonType(msg.Content) == "string" && common.Unmarshal(msg.Content, &text) == nil {
				continue
			}
			var blocks []struct {
				Type string  `json:"type"`
				Text *string `json:"text"`
			}
			if common.Unmarshal(msg.Content, &blocks) != nil || len(blocks) == 0 {
				return false
			}
			for _, block := range blocks {
				if block.Type != "text" || block.Text == nil {
					return false
				}
			}
			continue
		}
		var blocks []struct {
			Type      string  `json:"type"`
			ID        string  `json:"id"`
			ToolUseID string  `json:"tool_use_id"`
			Text      *string `json:"text"`
		}
		if common.Unmarshal(msg.Content, &blocks) != nil || len(blocks) == 0 {
			return false
		}
		if msg.Role == "assistant" && lastRole == "user" && len(pending) != 0 {
			return false
		}
		hasResult := false
		for _, block := range blocks {
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
				switch block.Type {
				case "tool_result":
					if !pending[block.ToolUseID] {
						return false
					}
					delete(pending, block.ToolUseID)
					hasResult = true
				case "text":
					// Text can follow a complete result batch, not interrupt it
					// even when results span consecutive user messages.
					if block.Text == nil || !hasResult || len(pending) != 0 {
						return false
					}
				default:
					return false
				}
			default:
				return false
			}
		}
		if msg.Role == "user" && !hasResult {
			return false
		}
		lastRole = msg.Role
	}
	return lastRole == "user" && len(pending) == 0 && len(seen) > 0
}
