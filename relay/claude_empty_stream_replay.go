package relay

import (
	"bytes"
	"encoding/json"

	"github.com/QuantumNous/new-api/common"
)

// canReplayEmptyClaudeStream only admits stateless Messages requests. Tool
// definitions without a type (or with type custom) run on the client; a hosted
// tool or provider-bound context might do work even if no output was returned.
func canReplayEmptyClaudeStream(body []byte) bool {
	var req struct {
		Stream bool `json:"stream"`
		Tools  []struct {
			Type        string          `json:"type"`
			Name        string          `json:"name"`
			InputSchema json.RawMessage `json:"input_schema"`
		} `json:"tools"`
		MCPServers        []json.RawMessage `json:"mcp_servers"`
		Container         json.RawMessage   `json:"container"`
		ContextManagement json.RawMessage   `json:"context_management"`
		ExtraBody         json.RawMessage   `json:"extra_body"`
	}
	if common.GetJsonType(body) != "object" || common.Unmarshal(body, &req) != nil || !req.Stream || len(req.MCPServers) != 0 {
		return false
	}
	for _, state := range []json.RawMessage{req.Container, req.ContextManagement, req.ExtraBody} {
		if len(state) != 0 && !bytes.Equal(bytes.TrimSpace(state), []byte("null")) {
			return false
		}
	}
	for _, tool := range req.Tools {
		if (tool.Type != "" && tool.Type != "custom") || tool.Name == "" || common.GetJsonType(tool.InputSchema) != "object" {
			return false
		}
	}
	return true
}
