package openai

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func deductionUsageTestInfo() *relaycommon.RelayInfo {
	return &relaycommon.RelayInfo{
		UsingGroup:  "default",
		ChannelMeta: &relaycommon.ChannelMeta{ChannelSetting: dto.ChannelSettings{InputTokenDeduction: 490}},
	}
}

func TestPatchOpenAIResponseUsageForClientDeductsCachedInput(t *testing.T) {
	data := `{"id":"chat","provider_extension":"kept","usage":{"prompt_tokens":506,"completion_tokens":0,"total_tokens":506,"prompt_cache_hit_tokens":450,"claude_cache_creation_5_m_tokens":0,"claude_cache_creation_1_h_tokens":0,"cost":1.25,"prompt_tokens_details":{"cached_tokens":450,"text_tokens":506,"extension":"kept"},"billing_usage":{"source":"must-not-leak"}}}`
	usage := &dto.Usage{
		PromptTokens:         506,
		TotalTokens:          506,
		PromptCacheHitTokens: 450,
		PromptTokensDetails:  dto.InputTokenDetails{CachedTokens: 450, TextTokens: 506},
	}

	patched := patchOpenAIResponseUsageForClient(data, deductionUsageTestInfo(), usage)
	var envelope map[string]interface{}
	require.NoError(t, common.UnmarshalJsonStr(patched, &envelope))
	public := envelope["usage"].(map[string]interface{})
	require.EqualValues(t, 16, public["prompt_tokens"])
	require.EqualValues(t, 16, public["total_tokens"])
	details := public["prompt_tokens_details"].(map[string]interface{})
	require.EqualValues(t, 16, details["cached_tokens"])
	require.EqualValues(t, 16, details["text_tokens"])
	require.EqualValues(t, 16, public["prompt_cache_hit_tokens"])
	require.EqualValues(t, 0, public["claude_cache_creation_5_m_tokens"])
	require.EqualValues(t, 0, public["claude_cache_creation_1_h_tokens"])
	require.EqualValues(t, 1.25, public["cost"], "upstream cost is not rewritten as gateway pricing")
	require.Equal(t, "kept", details["extension"])
	require.NotContains(t, public, "billing_usage")
	require.Equal(t, "kept", envelope["provider_extension"])
	require.Equal(t, 506, usage.PromptTokens)
}

func TestPatchResponsesUsageForClientDeductsCachedInput(t *testing.T) {
	data := `{"type":"response.completed","extension":"kept","response":{"id":"resp","usage":{"input_tokens":506,"output_tokens":0,"total_tokens":506,"prompt_cache_hit_tokens":450,"input_tokens_details":{"cached_tokens":450,"cached_creation_tokens":0,"cache_write_tokens":0,"text_tokens":506,"extension":"kept"}}}}`
	usage := &dto.Usage{InputTokens: 506, TotalTokens: 506, PromptCacheHitTokens: 450, InputTokensDetails: &dto.InputTokenDetails{CachedTokens: 450, TextTokens: 506}}

	patched := patchResponsesUsageForClient(data, deductionUsageTestInfo(), usage, true)
	var envelope map[string]interface{}
	require.NoError(t, common.UnmarshalJsonStr(patched, &envelope))
	public := envelope["response"].(map[string]interface{})["usage"].(map[string]interface{})
	require.EqualValues(t, 16, public["input_tokens"])
	require.EqualValues(t, 0, public["output_tokens"])
	require.EqualValues(t, 16, public["total_tokens"])
	details := public["input_tokens_details"].(map[string]interface{})
	require.EqualValues(t, 16, details["cached_tokens"])
	require.EqualValues(t, 16, details["text_tokens"])
	require.EqualValues(t, 0, details["cached_creation_tokens"])
	require.EqualValues(t, 0, details["cache_write_tokens"])
	require.EqualValues(t, 16, public["prompt_cache_hit_tokens"])
	require.Equal(t, "kept", details["extension"])
	require.Equal(t, "kept", envelope["extension"])
	require.Equal(t, 506, usage.InputTokens)
}

func TestOaiStreamHandlerPublishesDeductedFinalUsageAliasesOnce(t *testing.T) {
	originalTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() { constant.StreamingTimeout = originalTimeout })
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	info := deductionUsageTestInfo()
	info.RelayFormat = types.RelayFormatOpenAI
	info.RelayMode = relayconstant.RelayModeChatCompletions
	info.IsStream = true
	info.ShouldIncludeUsage = true
	info.StartTime = time.Now()
	info.FirstResponseTime = info.StartTime
	info.ChannelMeta.UpstreamModelName = "gpt-test"
	stream := strings.Join([]string{
		`data: {"id":"chat_stream","object":"chat.completion.chunk","created":1,"model":"gpt-test","choices":[{"index":0,"delta":{"content":""}}]}`,
		``,
		`data: {"id":"chat_stream","object":"chat.completion.chunk","created":1,"model":"gpt-test","choices":[],"usage":{"prompt_tokens":506,"completion_tokens":0,"total_tokens":506,"prompt_cache_hit_tokens":450,"prompt_tokens_details":{"cached_tokens":450,"text_tokens":506}}}`,
		``,
		`data: [DONE]`,
		``,
	}, "\n")
	response := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       io.NopCloser(strings.NewReader(stream)),
	}

	settlement, apiErr := OaiStreamHandler(ctx, info, response)

	require.Nil(t, apiErr)
	require.Equal(t, 506, settlement.PromptTokens)
	require.Equal(t, 450, settlement.PromptCacheHitTokens)
	output := recorder.Body.String()
	require.Contains(t, output, `"prompt_tokens":16`)
	require.Contains(t, output, `"prompt_cache_hit_tokens":16`)
	require.Contains(t, output, `"cached_tokens":16`)
	require.Contains(t, output, `"text_tokens":16`)
	require.NotContains(t, output, `"prompt_tokens":506`)
}

func TestOaiResponsesHandlerKeepsCacheCreationAliasesNormalized(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	info := deductionUsageTestInfo()
	info.ChannelMeta.ChannelSetting.InputTokenDeduction = 521
	body := `{"id":"resp_cache_write","object":"response","usage":{"input_tokens":536,"output_tokens":0,"total_tokens":536,"usage_semantic":"openai","prompt_cache_hit_tokens":450,"claude_cache_creation_5_m_tokens":10,"claude_cache_creation_1_h_tokens":20,"input_tokens_details":{"cached_tokens":450,"cached_creation_tokens":30,"cache_write_tokens":30,"text_tokens":536}}}`
	response := &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
	}

	settlement, apiErr := OaiResponsesHandler(ctx, info, response)

	require.Nil(t, apiErr)
	require.Equal(t, 536, settlement.PromptTokens)
	require.Equal(t, 450, settlement.PromptTokensDetails.CachedTokens)
	require.Equal(t, 30, settlement.PromptTokensDetails.CachedCreationTokens)
	var envelope map[string]interface{}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &envelope))
	public := envelope["usage"].(map[string]interface{})
	require.EqualValues(t, 15, public["input_tokens"])
	require.EqualValues(t, 0, public["prompt_cache_hit_tokens"])
	require.EqualValues(t, 0, public["claude_cache_creation_5_m_tokens"])
	require.EqualValues(t, 15, public["claude_cache_creation_1_h_tokens"])
	details := public["input_tokens_details"].(map[string]interface{})
	require.EqualValues(t, 0, details["cached_tokens"])
	require.EqualValues(t, 15, details["cached_creation_tokens"])
	require.EqualValues(t, 15, details["cache_write_tokens"])
	require.EqualValues(t, 15, details["text_tokens"])
}

func TestOpenaiHandlerPublishesAllDeductedAliasesAndKeepsSettlementUsage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	info := deductionUsageTestInfo()
	info.RelayFormat = types.RelayFormatOpenAI
	info.ChannelMeta.UpstreamModelName = "claude-test"
	body := `{"id":"chat","object":"chat.completion","choices":[],"usage":{"prompt_tokens":506,"completion_tokens":0,"total_tokens":506,"prompt_cache_hit_tokens":450,"prompt_tokens_details":{"cached_tokens":450,"text_tokens":506},"cost":2.5}}`
	response := &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
	}

	settlement, apiErr := OpenaiHandler(ctx, info, response)

	require.Nil(t, apiErr)
	require.Equal(t, 506, settlement.PromptTokens)
	require.Equal(t, 450, settlement.PromptCacheHitTokens)
	var envelope map[string]interface{}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &envelope))
	public := envelope["usage"].(map[string]interface{})
	require.EqualValues(t, 16, public["prompt_tokens"])
	require.EqualValues(t, 16, public["prompt_cache_hit_tokens"])
	require.EqualValues(t, 16, public["prompt_tokens_details"].(map[string]interface{})["text_tokens"])
	require.EqualValues(t, 2.5, public["cost"])
}

func TestPatchOpenAIUsageMapForClientSynchronizesCacheWriteAndTextAliases(t *testing.T) {
	raw := map[string]interface{}{
		"prompt_cache_hit_tokens":          float64(450),
		"claude_cache_creation_5_m_tokens": float64(10),
		"claude_cache_creation_1_h_tokens": float64(20),
		"cost":                             float64(9.5),
		"prompt_tokens_details": map[string]interface{}{
			"cached_tokens":          float64(450),
			"cached_creation_tokens": float64(30),
			"cache_write_tokens":     float64(30),
			"text_tokens":            float64(536),
		},
		"input_tokens_details": map[string]interface{}{
			"cached_tokens":          float64(450),
			"cached_creation_tokens": float64(30),
			"cache_write_tokens":     float64(30),
			"text_tokens":            float64(536),
		},
	}
	adjusted := &dto.Usage{
		PromptCacheHitTokens: 0,
		PromptTokensDetails: dto.InputTokenDetails{
			CachedCreationTokens: 15,
			CacheWriteTokens:     15,
			TextTokens:           15,
		},
		InputTokensDetails: &dto.InputTokenDetails{
			CachedCreationTokens: 15,
			CacheWriteTokens:     15,
			TextTokens:           15,
		},
		ClaudeCacheCreation1hTokens: 15,
	}

	patchOpenAIUsageMapForClient(raw, adjusted)

	require.EqualValues(t, 0, raw["prompt_cache_hit_tokens"])
	require.EqualValues(t, 0, raw["claude_cache_creation_5_m_tokens"])
	require.EqualValues(t, 15, raw["claude_cache_creation_1_h_tokens"])
	require.EqualValues(t, 9.5, raw["cost"])
	for _, key := range []string{"prompt_tokens_details", "input_tokens_details"} {
		details := raw[key].(map[string]interface{})
		require.EqualValues(t, 0, details["cached_tokens"])
		require.EqualValues(t, 15, details["cached_creation_tokens"])
		require.EqualValues(t, 15, details["cache_write_tokens"])
		require.EqualValues(t, 15, details["text_tokens"])
	}
}

func TestPublicUsagePatchNoConfigurationKeepsBytes(t *testing.T) {
	data := `{"usage":{"prompt_tokens":506,"completion_tokens":0,"total_tokens":506,"prompt_tokens_details":{"cached_tokens":450}}}`
	usage := &dto.Usage{PromptTokens: 506, TotalTokens: 506, PromptTokensDetails: dto.InputTokenDetails{CachedTokens: 450}}
	require.Equal(t, data, patchOpenAIResponseUsageForClient(data, &relaycommon.RelayInfo{}, usage))
}
