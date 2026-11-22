package dto

import (
	"encoding/json"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFable52ThinkingCompatibility(t *testing.T) {
	for _, tc := range []struct {
		name, model, thinking, config, wantThinking, wantConfig string
	}{
		{"disabled", "claude-fable-5-2", `{"type":"disabled","extension":9007199254740993}`, `{"extension":9007199254740993}`, `{"type":"adaptive","extension":9007199254740993}`, `{"effort":"low","extension":9007199254740993}`},
		{"missing budget", "claude-fable-5-2", `{"type":"enabled"}`, `{"effort":"low","extension":9007199254740993}`, `{"type":"adaptive"}`, `{"effort":"low","extension":9007199254740993}`},
		{"null budget", "claude-fable-5-2", `{"type":"enabled","budget_tokens":null}`, `null`, `{"type":"adaptive"}`, `{"effort":"high"}`},
		{"explicit effort and display", "claude-fable-5-2", `{"type":"disabled","display":"summarized"}`, `{"effort":"high"}`, `{"type":"adaptive","display":"summarized"}`, `{"effort":"high"}`},
		{"explicit budget unchanged", "claude-fable-5-2", `{"type":"enabled","budget_tokens":2048}`, `{"extension":1}`, `{"type":"enabled","budget_tokens":2048}`, `{"extension":1}`},
		{"explicit zero unchanged", "claude-fable-5-2", `{"type":"enabled","budget_tokens":0}`, `{"extension":1}`, `{"type":"enabled","budget_tokens":0}`, `{"extension":1}`},
		{"adaptive already valid", "claude-fable-5-2", `{"type":"adaptive","display":"omitted"}`, `{"effort":"xhigh"}`, `{"type":"adaptive","display":"omitted"}`, `{"effort":"xhigh"}`},
		{"no thinking specified", "claude-fable-5-2", `null`, `{"effort":"low"}`, `null`, `{"effort":"low"}`},
		{"other thinking mode", "claude-fable-5-2", `{"type":"between_tools"}`, `{"effort":"low"}`, `{"type":"between_tools"}`, `{"effort":"low"}`},
		{"legacy model", "claude-sonnet-4-5", `{"type":"enabled","budget_tokens":2048}`, `{"extension":1}`, `{"type":"enabled","budget_tokens":2048}`, `{"extension":1}`},
		{"near name", "claude-fable-5-20", `{"type":"disabled"}`, `{"extension":1}`, `{"type":"disabled"}`, `{"extension":1}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := []byte(`{"model":"` + tc.model + `","max_tokens":1000000,"thinking":` + tc.thinking + `,"output_config":` + tc.config + `,"messages":[{"role":"user","content":"keep this"}],"tools":[{"name":"local","input_schema":{"type":"object"}}],"temperature":0.3,"extension":9007199254740993}`)
			out, err := NormalizeClaudeAdaptiveJSON(body, "")
			require.NoError(t, err)
			if tc.thinking == tc.wantThinking && tc.config == tc.wantConfig {
				assert.Equal(t, body, out, "untouched requests retain their original bytes")
			}
			var before, after map[string]json.RawMessage
			require.NoError(t, common.Unmarshal(body, &before))
			require.NoError(t, common.Unmarshal(out, &after))
			assert.JSONEq(t, tc.wantThinking, string(after["thinking"]))
			assert.JSONEq(t, tc.wantConfig, string(after["output_config"]))
			if tc.name == "disabled" {
				assert.Contains(t, string(after["thinking"]), "9007199254740993")
			}
			if tc.name == "missing budget" || tc.name == "disabled" {
				assert.Contains(t, string(after["output_config"]), "9007199254740993")
			}
			delete(before, "thinking")
			delete(before, "output_config")
			delete(after, "thinking")
			delete(after, "output_config")
			assert.Equal(t, before, after)
			again, err := NormalizeClaudeAdaptiveJSON(out, "")
			require.NoError(t, err)
			assert.Equal(t, out, again)
		})
	}
}
