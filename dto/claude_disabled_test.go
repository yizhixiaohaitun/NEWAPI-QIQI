package dto

import (
	"encoding/json"
	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestOpus55DisabledCapabilityAndEffort(t *testing.T) {
	for _, model := range []string{"claude-opus-5-5", "claude-opus-5-5-20261001", "anthropic.claude-opus-5-5", "us.anthropic.claude-opus-5-5-v1:0", "eu.anthropic.claude-opus-5-5-v2:0", "apac.anthropic.claude-opus-5-5-v1:0", "global.anthropic.claude-opus-5-5-v1:0", "claude-opus-5-5@20261001", "claude-opus-5-5-high", "claude-opus-5-5-thinking"} {
		for _, effort := range []string{"", "low", "medium", "high"} {
			t.Run(model+"/"+effort, func(t *testing.T) {
				output := json.RawMessage(`{"extension":9007199254740993}`)
				if effort != "" {
					output = json.RawMessage(`{"effort":"` + effort + `","extension":9007199254740993}`)
				}
				req := &ClaudeRequest{Model: model, MaxTokens: common.GetPointer[uint](64000), Thinking: &Thinking{Type: "disabled", BudgetTokens: common.GetPointer(4096)}, OutputConfig: output}
				require.NoError(t, NormalizeClaudeAdaptiveThinking(req, ""))
				assert.Equal(t, "adaptive", req.Thinking.Type)
				assert.Nil(t, req.Thinking.BudgetTokens)
				expected := effort
				if expected == "" {
					expected = "low"
				}
				assert.JSONEq(t, `{"effort":"`+expected+`","extension":9007199254740993}`, string(req.OutputConfig))
				assert.Equal(t, uint(64000), *req.MaxTokens)
				before, err := common.Marshal(req)
				require.NoError(t, err)
				require.NoError(t, NormalizeClaudeAdaptiveThinking(req, ""))
				after, err := common.Marshal(req)
				require.NoError(t, err)
				assert.Equal(t, string(before), string(after), "normalization is idempotent")
			})
		}
	}
	for _, model := range []string{"claude-opus-4-5", "claude-opus-4-6", "claude-opus-4-7", "claude-opus-4-7-high", "claude-opus-4-8", "claude-opus-5-50", "claude-opus-6-0", "other.anthropic.claude-opus-5-5"} {
		req := &ClaudeRequest{Model: model, Thinking: &Thinking{Type: "disabled"}}
		require.NoError(t, NormalizeClaudeAdaptiveThinking(req, ""))
		assert.Equal(t, "disabled", req.Thinking.Type, model)
	}
}

func TestOpus55DisabledFinalJSONOverride(t *testing.T) {
	for _, model := range []string{"claude-opus-5-5", "us.anthropic.claude-opus-5-5-v1:0", "claude-opus-5-5@20261001"} {
		for _, effort := range []string{"", "medium", "high"} {
			input := `{"model":"` + model + `","thinking":{"type":"disabled","budget_tokens":4096,"extension":9007199254740993},"max_tokens":64000,"messages":[{"role":"user","content":"unchanged"}],"output_config":{"extension":1.1234567890123456789`
			if effort != "" {
				input += `,"effort":"` + effort + `"`
			}
			input += `},"provider_extension":9007199254740993}`
			output, err := NormalizeClaudeAdaptiveJSON([]byte(input), "")
			require.NoError(t, err)
			var fields map[string]json.RawMessage
			require.NoError(t, common.Unmarshal(output, &fields))
			assert.JSONEq(t, `{"type":"adaptive","extension":9007199254740993}`, string(fields["thinking"]))
			expected := effort
			if expected == "" {
				expected = "low"
			}
			assert.JSONEq(t, `{"effort":"`+expected+`","extension":1.1234567890123456789}`, string(fields["output_config"]))
			assert.Equal(t, "64000", string(fields["max_tokens"]))
			assert.Equal(t, "9007199254740993", string(fields["provider_extension"]))
			again, err := NormalizeClaudeAdaptiveJSON(output, "")
			require.NoError(t, err)
			assert.JSONEq(t, string(output), string(again))
		}
	}
	input := []byte(`{"thinking":{"type":"disabled"},"max_tokens":64000}`)
	output, err := NormalizeClaudeAdaptiveJSON(input, "claude-opus-5-5")
	require.NoError(t, err)
	assert.Contains(t, string(output), `"effort":"low"`)
	for _, model := range []string{"claude-opus-4-7", "claude-opus-4-8", "claude-opus-4-5"} {
		output, err := NormalizeClaudeAdaptiveJSON(input, model)
		require.NoError(t, err)
		assert.Equal(t, string(input), string(output))
	}
}

func TestOpus55DisabledExplicitConversionEffort(t *testing.T) {
	req := &ClaudeRequest{Model: "claude-opus-5-5", Thinking: &Thinking{Type: "disabled"}}
	require.NoError(t, NormalizeClaudeAdaptiveThinking(req, "medium"))
	assert.JSONEq(t, `{"effort":"medium"}`, string(req.OutputConfig))
}
