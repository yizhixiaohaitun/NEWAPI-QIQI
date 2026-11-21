package relay

import (
	"encoding/json"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/model_setting"
	"github.com/gin-gonic/gin"
)

// NativeChatStreamForHedge excludes body overrides and the same automatic
// bridge that TextHelper would select for this channel.
func NativeChatStreamForHedge(c *gin.Context, info *relaycommon.RelayInfo, setting dto.ChannelSettings, overrides map[string]interface{}) bool {
	if !headerOnlyOverrides(overrides) {
		return false
	}
	global := model_setting.GetGlobalSettings()
	if global.PassThroughRequestEnabled || setting.PassThroughBodyEnabled {
		return false
	}
	if !global.AutoChatToResponsesEnabled {
		return true
	}
	candidate := *info
	candidate.ChannelMeta = &relaycommon.ChannelMeta{ChannelSetting: setting, ParamOverride: overrides}
	return !safeChatResponsesBridge(c, &candidate)
}

// StreamHedgeHeaderOnlyOverrides ensures channels cannot add persistent state
// or disable streaming after the incoming request passed the eligibility gate.
func StreamHedgeHeaderOnlyOverrides(overrides map[string]interface{}) bool {
	return headerOnlyOverrides(overrides)
}

// Only automatically bridge requests whose complete wire representation is known
// to survive the conversion. Unknown fields and channel overrides keep the chat path.
func safeChatResponsesBridge(c *gin.Context, info *relaycommon.RelayInfo) bool {
	if !headerOnlyOverrides(info.ParamOverride) || info.ChannelSetting.SystemPrompt != "" {
		return false
	}
	storage, err := common.GetBodyStorage(c)
	if err != nil {
		return false
	}
	body, err := storage.Bytes()
	if err != nil {
		return false
	}
	var req map[string]json.RawMessage
	if common.Unmarshal(body, &req) != nil {
		return false
	}
	allowed := map[string]bool{"prompt_cache_key": true, "model": true, "messages": true, "stream": true, "stream_options": true, "temperature": true, "top_p": true, "max_tokens": true, "max_completion_tokens": true}
	for k := range req {
		if !allowed[k] {
			return false
		}
	}
	if _, a := req["max_tokens"]; a {
		if _, b := req["max_completion_tokens"]; b {
			return false
		}
	}
	if raw, ok := req["stream_options"]; ok && string(raw) != "null" {
		var opts map[string]json.RawMessage
		if common.Unmarshal(raw, &opts) != nil {
			return false
		}
		for k := range opts {
			if k != "include_usage" {
				return false
			}
		}
	}
	var messages []map[string]json.RawMessage
	if common.Unmarshal(req["messages"], &messages) != nil || len(messages) == 0 {
		return false
	}
	for i, msg := range messages {
		for k := range msg {
			if k != "role" && k != "content" {
				return false
			}
		}
		var role, content string
		if common.Unmarshal(msg["role"], &role) != nil || (role != "user" && role != "assistant" && !(role == "system" && i == 0)) || common.Unmarshal(msg["content"], &content) != nil || string(msg["content"]) == "null" {
			return false
		}
		if role == "system" && (content == "" || strings.TrimSpace(content) != content) {
			return false
		}
	}
	return true
}

// Header-only passthrough templates cannot change the request body.
func headerOnlyOverrides(overrides map[string]interface{}) bool {
	if len(overrides) == 0 {
		return true
	}
	if len(overrides) != 1 {
		return false
	}
	raw, ok := overrides["operations"]
	if !ok {
		return false
	}
	b, err := json.Marshal(raw)
	if err != nil {
		return false
	}
	var ops []map[string]json.RawMessage
	if json.Unmarshal(b, &ops) != nil || len(ops) == 0 {
		return false
	}
	for _, op := range ops {
		var mode string
		if json.Unmarshal(op["mode"], &mode) != nil || mode != "pass_headers" {
			return false
		}
	}
	return true
}
