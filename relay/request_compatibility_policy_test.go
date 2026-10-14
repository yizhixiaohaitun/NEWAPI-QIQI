package relay

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service/relayconvert"
	"github.com/QuantumNous/new-api/setting/model_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestRequestCompatibilityThinkingPolicy(t *testing.T) {
	for _, model := range []string{"claude-opus-4-7", "claude-opus-4-8", "claude-opus-5-5"} {
		for _, enabled := range []bool{true, false} {
			for _, effort := range []string{"high", "medium", "none"} {
				for _, protocol := range []string{"chat", "responses"} {
					t.Run(model+"/"+effort+"/"+protocol+"/"+common.Interface2String(enabled), func(t *testing.T) {
						c, _ := gin.CreateTestContext(httptest.NewRecorder())
						common.SetContextKey(c, constant.ContextKeyTokenRequestCompatibilityEnabled, &enabled)
						info := &relaycommon.RelayInfo{RequestCompatibilityEnabled: &enabled}
						var req any
						if protocol == "chat" {
							req = &dto.GeneralOpenAIRequest{Model: model, MaxTokens: common.GetPointer[uint](64000), ReasoningEffort: effort, Messages: []dto.Message{{Role: "user", Content: "Hi"}}}
						} else {
							req = &dto.OpenAIResponsesRequest{Model: model, MaxOutputTokens: common.GetPointer[uint](64000), Input: json.RawMessage(`"Hi"`), Reasoning: &dto.Reasoning{Effort: effort}}
						}
						result, err := relayconvert.ConvertRequest(c, info, types.RelayFormatClaude, req)
						require.NoError(t, err)
						r := result.Value.(*dto.ClaudeRequest)
						if !enabled {
							if effort == "none" {
								require.Equal(t, "disabled", r.Thinking.Type)
							} else {
								require.Equal(t, "enabled", r.Thinking.Type)
								require.NotNil(t, r.Thinking.BudgetTokens)
							}
							require.Empty(t, r.OutputConfig)
						} else if effort == "none" && model != "claude-opus-5-5" {
							require.Equal(t, "disabled", r.Thinking.Type)
						} else {
							require.Equal(t, "adaptive", r.Thinking.Type)
							require.Nil(t, r.Thinking.BudgetTokens)
							if effort == "none" {
								require.JSONEq(t, `{"effort":"low"}`, string(r.OutputConfig))
							} else {
								require.JSONEq(t, `{"effort":"`+effort+`"}`, string(r.OutputConfig))
							}
						}
						require.Equal(t, uint(64000), *r.MaxTokens)
					})
				}
			}
		}
	}
}

func TestRequestCompatibilityLegacyThinkingSuffixToggle(t *testing.T) {
	old := model_setting.GetClaudeSettings().ThinkingAdapterEnabled
	model_setting.GetClaudeSettings().ThinkingAdapterEnabled = true
	defer func() { model_setting.GetClaudeSettings().ThinkingAdapterEnabled = old }()
	for _, enabled := range []bool{false, true} {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		common.SetContextKey(c, constant.ContextKeyTokenRequestCompatibilityEnabled, &enabled)
		result, err := relayconvert.ConvertRequest(c, &relaycommon.RelayInfo{RequestCompatibilityEnabled: &enabled}, types.RelayFormatClaude, &dto.GeneralOpenAIRequest{Model: "claude-opus-4-6-thinking", MaxTokens: common.GetPointer[uint](30000), Messages: []dto.Message{{Role: "user", Content: "Hi"}}})
		require.NoError(t, err)
		r := result.Value.(*dto.ClaudeRequest)
		if enabled {
			require.NotNil(t, r.Thinking)
			require.Equal(t, "enabled", r.Thinking.Type)
			require.Equal(t, "claude-opus-4-6", r.Model)
		} else {
			require.Nil(t, r.Thinking)
			require.Equal(t, "claude-opus-4-6-thinking", r.Model)
		}
	}
}
