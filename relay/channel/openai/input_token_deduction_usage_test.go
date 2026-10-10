package openai

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/stretchr/testify/require"
)

func deductionUsageTestInfo() *relaycommon.RelayInfo {
	return &relaycommon.RelayInfo{
		UsingGroup:  "default",
		ChannelMeta: &relaycommon.ChannelMeta{ChannelSetting: dto.ChannelSettings{InputTokenDeduction: 490}},
	}
}

func TestPatchOpenAIResponseUsageForClientDeductsCachedInput(t *testing.T) {
	data := `{"id":"chat","provider_extension":"kept","usage":{"prompt_tokens":506,"completion_tokens":0,"total_tokens":506,"prompt_tokens_details":{"cached_tokens":450,"extension":"kept"},"billing_usage":{"source":"must-not-leak"}}}`
	usage := &dto.Usage{PromptTokens: 506, TotalTokens: 506, PromptTokensDetails: dto.InputTokenDetails{CachedTokens: 450}}

	patched := patchOpenAIResponseUsageForClient(data, deductionUsageTestInfo(), usage)
	var envelope map[string]interface{}
	require.NoError(t, common.UnmarshalJsonStr(patched, &envelope))
	public := envelope["usage"].(map[string]interface{})
	require.EqualValues(t, 16, public["prompt_tokens"])
	require.EqualValues(t, 16, public["total_tokens"])
	require.EqualValues(t, 16, public["prompt_tokens_details"].(map[string]interface{})["cached_tokens"])
	require.Equal(t, "kept", public["prompt_tokens_details"].(map[string]interface{})["extension"])
	require.NotContains(t, public, "billing_usage")
	require.Equal(t, "kept", envelope["provider_extension"])
	require.Equal(t, 506, usage.PromptTokens)
}

func TestPatchResponsesUsageForClientDeductsCachedInput(t *testing.T) {
	data := `{"type":"response.completed","extension":"kept","response":{"id":"resp","usage":{"input_tokens":506,"output_tokens":0,"total_tokens":506,"input_tokens_details":{"cached_tokens":450,"extension":"kept"}}}}`
	usage := &dto.Usage{InputTokens: 506, TotalTokens: 506, InputTokensDetails: &dto.InputTokenDetails{CachedTokens: 450}}

	patched := patchResponsesUsageForClient(data, deductionUsageTestInfo(), usage, true)
	var envelope map[string]interface{}
	require.NoError(t, common.UnmarshalJsonStr(patched, &envelope))
	public := envelope["response"].(map[string]interface{})["usage"].(map[string]interface{})
	require.EqualValues(t, 16, public["input_tokens"])
	require.EqualValues(t, 0, public["output_tokens"])
	require.EqualValues(t, 16, public["total_tokens"])
	details := public["input_tokens_details"].(map[string]interface{})
	require.EqualValues(t, 16, details["cached_tokens"])
	require.Equal(t, "kept", details["extension"])
	require.Equal(t, "kept", envelope["extension"])
	require.Equal(t, 506, usage.InputTokens)
}

func TestPublicUsagePatchNoConfigurationKeepsBytes(t *testing.T) {
	data := `{"usage":{"prompt_tokens":506,"completion_tokens":0,"total_tokens":506,"prompt_tokens_details":{"cached_tokens":450}}}`
	usage := &dto.Usage{PromptTokens: 506, TotalTokens: 506, PromptTokensDetails: dto.InputTokenDetails{CachedTokens: 450}}
	require.Equal(t, data, patchOpenAIResponseUsageForClient(data, &relaycommon.RelayInfo{}, usage))
}
