package dto

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestNormalizeClaudeAdaptiveThinking(t *testing.T) {
	for _, model := range []string{"claude-opus-5-5", "anthropic.claude-opus-5-5", "us.anthropic.claude-opus-5-5-v1:0", "claude-opus-4-7"} {
		req := &ClaudeRequest{Model: model, MaxTokens: common.GetPointer[uint](16384), Thinking: &Thinking{Type: "enabled", BudgetTokens: common.GetPointer(4096)}, OutputConfig: []byte(`{"custom":9007199254740993}`), Temperature: common.GetPointer(0.3)}
		require.NoError(t, NormalizeClaudeAdaptiveThinking(req, ""))
		assert.Equal(t, "adaptive", req.Thinking.Type)
		assert.Nil(t, req.Thinking.BudgetTokens)
		assert.JSONEq(t, `{"effort":"high","custom":9007199254740993}`, string(req.OutputConfig))
		assert.Equal(t, uint(16384), *req.MaxTokens)
		if model == "claude-opus-5-5" {
			assert.Equal(t, 0.3, *req.Temperature)
		}
	}
	for _, kind := range []string{"adaptive", "disabled"} {
		req := &ClaudeRequest{Model: "claude-opus-5-5", Thinking: &Thinking{Type: kind}, OutputConfig: []byte(`{"effort":"medium","unknown":true}`)}
		require.NoError(t, NormalizeClaudeAdaptiveThinking(req, "high"))
		assert.Equal(t, "adaptive", req.Thinking.Type)
		assert.JSONEq(t, `{"effort":"medium","unknown":true}`, string(req.OutputConfig))
	}
	for _, model := range []string{"claude-opus-4-5", "claude-opus-4-6", "claude-opus-6-0", "not-claude-opus-5-5", "claude-opus-5-50", "claude-opus-5-5-fable", "other.anthropic.claude-opus-5-5"} {
		req := &ClaudeRequest{Model: model, Thinking: &Thinking{Type: "enabled", BudgetTokens: common.GetPointer(4096)}}
		require.NoError(t, NormalizeClaudeAdaptiveThinking(req, ""))
		assert.Equal(t, "enabled", req.Thinking.Type)
		assert.Equal(t, 4096, *req.Thinking.BudgetTokens)
	}
	req := &ClaudeRequest{Model: "claude-opus-5-5"}
	require.NoError(t, NormalizeClaudeAdaptiveThinking(req, ""))
	assert.Nil(t, req.Thinking)
}
