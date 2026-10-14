package dto

import (
	"encoding/json"
	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestRequestCompatibilitySamplingWhitelist(t *testing.T) {
	for _, model := range []string{"claude-opus-4-7", "claude-opus-4-7-high", "claude-opus-4-7-20260416", "us.anthropic.claude-opus-4-7-v1:0", "jp.anthropic.claude-opus-4-7", "au.anthropic.claude-opus-4-7", "claude-opus-4-7@20260416"} {
		for _, thinking := range []*Thinking{nil, {Type: "disabled"}, {Type: "enabled", BudgetTokens: common.GetPointer(4096)}, {Type: "adaptive"}} {
			req := &ClaudeRequest{Model: model, Thinking: thinking, Temperature: common.GetPointer(.96), TopP: common.GetPointer(0.0), TopK: common.GetPointer(4), MaxTokens: common.GetPointer[uint](30000)}
			require.NoError(t, NormalizeClaudeAdaptiveThinking(req, ""))
			require.Nil(t, req.Temperature, model)
			require.Nil(t, req.TopP, model)
			require.Nil(t, req.TopK, model)
			require.Equal(t, uint(30000), *req.MaxTokens)
			body := []byte(`{"model":"` + model + `","temperature":0.96,"top_p":0,"top_k":4,"max_tokens":30000,"unknown":9007199254740993,"output_config":{"effort":"medium"},"messages":[{"role":"user","content":"Hi"}]}`)
			output, err := NormalizeClaudeAdaptiveJSON(body, "")
			require.NoError(t, err)
			var fields map[string]json.RawMessage
			require.NoError(t, json.Unmarshal(output, &fields))
			for _, key := range []string{"temperature", "top_p", "top_k"} {
				require.NotContains(t, fields, key)
			}
			require.Equal(t, "9007199254740993", string(fields["unknown"]))
			require.JSONEq(t, `{"effort":"medium"}`, string(fields["output_config"]))
			again, err := NormalizeClaudeAdaptiveJSON(output, "")
			require.NoError(t, err)
			require.Equal(t, string(output), string(again))
		}
	}
	for _, model := range []string{"claude-opus-4-6", "claude-opus-4-8", "claude-opus-5-5", "claude-opus-6-0", "other-claude-opus-4-7", "gpt-4o"} {
		body := []byte(`{"model":"` + model + `","temperature":0.96,"top_p":0,"top_k":4}`)
		output, err := NormalizeClaudeAdaptiveJSON(body, "")
		require.NoError(t, err)
		require.Equal(t, string(body), string(output), model)
	}
}
