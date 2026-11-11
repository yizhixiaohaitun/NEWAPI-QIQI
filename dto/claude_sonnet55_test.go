package dto

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSonnet55AdaptiveThinkingBoundary(t *testing.T) {
	for _, model := range []string{"claude-sonnet-5-5", "claude-sonnet-5-5-20261101", "us.anthropic.claude-sonnet-5-5-v1:0", "claude-sonnet-5-5@20261101", "claude-sonnet-5-5-high"} {
		t.Run(model, func(t *testing.T) {
			req := &ClaudeRequest{Model: model, Thinking: &Thinking{Type: "enabled", BudgetTokens: common.GetPointer(4096)}, Temperature: common.GetPointer(0.3)}
			require.NoError(t, NormalizeClaudeAdaptiveThinking(req, ""))
			assert.Equal(t, "adaptive", req.Thinking.Type)
			assert.Nil(t, req.Thinking.BudgetTokens)
			assert.JSONEq(t, `{"effort":"high"}`, string(req.OutputConfig))
			assert.Equal(t, 0.3, *req.Temperature)
		})
	}
	for _, model := range []string{"claude-sonnet-5-4", "claude-sonnet-5-6", "claude-sonnet-5-50", "claude-sonnet-5-5-extra", "jp.anthropic.claude-sonnet-5-5-v1:0", "au.anthropic.claude-sonnet-5-5-v1:0"} {
		assert.False(t, ClaudeRequiresAdaptiveThinking(model), model)
	}
	req := &ClaudeRequest{Model: "claude-sonnet-5-5", Thinking: &Thinking{Type: "disabled"}}
	require.NoError(t, NormalizeClaudeAdaptiveThinking(req, ""))
	assert.Equal(t, "disabled", req.Thinking.Type)
}
