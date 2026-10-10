package openai

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
)

func patchOpenAIUsageMapForClient(raw map[string]interface{}, adjusted *dto.Usage) {
	if raw == nil || adjusted == nil {
		return
	}
	raw["prompt_tokens"] = adjusted.PromptTokens
	raw["completion_tokens"] = adjusted.CompletionTokens
	raw["total_tokens"] = adjusted.TotalTokens
	if _, ok := raw["input_tokens"]; ok || adjusted.InputTokens != 0 {
		raw["input_tokens"] = adjusted.InputTokens
	}
	if _, ok := raw["output_tokens"]; ok || adjusted.OutputTokens != 0 {
		raw["output_tokens"] = adjusted.OutputTokens
	}
	if _, exists := raw["prompt_cache_hit_tokens"]; exists {
		raw["prompt_cache_hit_tokens"] = adjusted.PromptTokensDetails.CachedTokens
	}
	if _, exists := raw["claude_cache_creation_5_m_tokens"]; exists {
		raw["claude_cache_creation_5_m_tokens"] = adjusted.ClaudeCacheCreation5mTokens
	}
	if _, exists := raw["claude_cache_creation_1_h_tokens"]; exists {
		raw["claude_cache_creation_1_h_tokens"] = adjusted.ClaudeCacheCreation1hTokens
	}
	patchOpenAIInputTokenDetailsForClient(raw["prompt_tokens_details"], adjusted.PromptTokensDetails)
	if adjusted.InputTokensDetails != nil {
		patchOpenAIInputTokenDetailsForClient(raw["input_tokens_details"], *adjusted.InputTokensDetails)
	}
	delete(raw, "billing_usage")
}

func patchOpenAIInputTokenDetailsForClient(raw interface{}, adjusted dto.InputTokenDetails) {
	details, ok := raw.(map[string]interface{})
	if !ok {
		return
	}
	details["cached_tokens"] = adjusted.CachedTokens
	if _, exists := details["cache_write_tokens"]; exists {
		details["cache_write_tokens"] = adjusted.CacheWriteTokens
	}
	if _, exists := details["cached_creation_tokens"]; exists {
		details["cached_creation_tokens"] = adjusted.CachedCreationTokens
	}
	if _, exists := details["text_tokens"]; exists {
		details["text_tokens"] = adjusted.TextTokens
	}
}

func normalizedResponsesUsage(usage *dto.Usage) dto.Usage {
	if usage == nil {
		return dto.Usage{}
	}
	clone := *usage
	if usage.InputTokensDetails != nil {
		details := *usage.InputTokensDetails
		clone.InputTokensDetails = &details
	}
	return *normalizedClientUsage(&clone)
}

func normalizedClientUsage(usage *dto.Usage) *dto.Usage {
	if usage == nil || usage.PromptTokens != 0 || usage.InputTokens == 0 {
		return usage
	}
	clone := *usage
	clone.PromptTokens = usage.InputTokens
	clone.CompletionTokens = usage.OutputTokens
	if clone.TotalTokens == 0 {
		clone.TotalTokens = clone.PromptTokens + clone.CompletionTokens
	}
	if usage.InputTokensDetails != nil {
		clone.PromptTokensDetails = *usage.InputTokensDetails
	}
	return &clone
}

func patchOpenAIResponseUsageForClient(data string, info *relaycommon.RelayInfo, usage *dto.Usage) string {
	normalized := normalizedClientUsage(usage)
	adjusted := service.AdjustUsageForClient(info, normalized)
	if adjusted == normalized {
		return data
	}
	var envelope map[string]interface{}
	if err := common.UnmarshalJsonStr(data, &envelope); err != nil {
		return data
	}
	rawUsage, ok := envelope["usage"].(map[string]interface{})
	if !ok {
		return data
	}
	patchOpenAIUsageMapForClient(rawUsage, adjusted)
	encoded, err := common.Marshal(envelope)
	if err != nil {
		return data
	}
	return string(encoded)
}

func patchResponsesUsageForClient(data string, info *relaycommon.RelayInfo, usage *dto.Usage, nestedResponse bool) string {
	normalized := normalizedClientUsage(usage)
	adjusted := service.AdjustUsageForClient(info, normalized)
	if adjusted == normalized || adjusted == nil {
		return data
	}
	var envelope map[string]interface{}
	if err := common.UnmarshalJsonStr(data, &envelope); err != nil {
		return data
	}
	target := envelope
	if nestedResponse {
		response, ok := envelope["response"].(map[string]interface{})
		if !ok {
			return data
		}
		target = response
	}
	rawUsage, ok := target["usage"].(map[string]interface{})
	if !ok {
		return data
	}
	patchOpenAIUsageMapForClient(rawUsage, adjusted)
	// Responses names its aggregate fields input_tokens/output_tokens. Keep the
	// compatibility aliases coherent when an upstream also emits them.
	rawUsage["input_tokens"] = adjusted.PromptTokens
	rawUsage["output_tokens"] = adjusted.CompletionTokens
	if adjusted.InputTokensDetails == nil {
		adjusted.InputTokensDetails = &adjusted.PromptTokensDetails
	}
	if details, ok := rawUsage["input_tokens_details"].(map[string]interface{}); ok {
		details["cached_tokens"] = adjusted.PromptTokensDetails.CachedTokens
		if _, exists := details["cache_write_tokens"]; exists {
			details["cache_write_tokens"] = adjusted.PromptTokensDetails.CacheWriteTokens
		}
	}
	encoded, err := common.Marshal(envelope)
	if err != nil {
		return data
	}
	return string(encoded)
}

func applyUsagePostProcessing(info *relaycommon.RelayInfo, usage *dto.Usage, responseBody []byte) {
	if info == nil || usage == nil {
		return
	}

	switch info.ChannelType {
	case constant.ChannelTypeDeepSeek:
		if usage.PromptTokensDetails.CachedTokens == 0 && usage.PromptCacheHitTokens != 0 {
			usage.PromptTokensDetails.CachedTokens = usage.PromptCacheHitTokens
		}
	case constant.ChannelTypeZhipu_v4:
		// 智普的cached_tokens在标准位置: usage.prompt_tokens_details.cached_tokens
		if usage.PromptTokensDetails.CachedTokens == 0 {
			if usage.InputTokensDetails != nil && usage.InputTokensDetails.CachedTokens > 0 {
				usage.PromptTokensDetails.CachedTokens = usage.InputTokensDetails.CachedTokens
			} else if cachedTokens, ok := extractCachedTokensFromBody(responseBody); ok {
				usage.PromptTokensDetails.CachedTokens = cachedTokens
			} else if usage.PromptCacheHitTokens > 0 {
				usage.PromptTokensDetails.CachedTokens = usage.PromptCacheHitTokens
			}
		}
	case constant.ChannelTypeMoonshot:
		// Moonshot的cached_tokens在非标准位置: choices[].usage.cached_tokens
		if usage.PromptTokensDetails.CachedTokens == 0 {
			if usage.InputTokensDetails != nil && usage.InputTokensDetails.CachedTokens > 0 {
				usage.PromptTokensDetails.CachedTokens = usage.InputTokensDetails.CachedTokens
			} else if cachedTokens, ok := extractMoonshotCachedTokensFromBody(responseBody); ok {
				usage.PromptTokensDetails.CachedTokens = cachedTokens
			} else if cachedTokens, ok := extractCachedTokensFromBody(responseBody); ok {
				usage.PromptTokensDetails.CachedTokens = cachedTokens
			} else if usage.PromptCacheHitTokens > 0 {
				usage.PromptTokensDetails.CachedTokens = usage.PromptCacheHitTokens
			}
		}
	case constant.ChannelTypeOpenAI:
		if usage.PromptTokensDetails.CachedTokens == 0 {
			if cachedTokens, ok := extractLlamaCachedTokensFromBody(responseBody); ok {
				usage.PromptTokensDetails.CachedTokens = cachedTokens
			}
		}
	}
}

func extractCachedTokensFromBody(body []byte) (int, bool) {
	if len(body) == 0 {
		return 0, false
	}

	var payload struct {
		Usage struct {
			PromptTokensDetails struct {
				CachedTokens *int `json:"cached_tokens"`
			} `json:"prompt_tokens_details"`
			CachedTokens         *int `json:"cached_tokens"`
			PromptCacheHitTokens *int `json:"prompt_cache_hit_tokens"`
		} `json:"usage"`
	}

	if err := common.Unmarshal(body, &payload); err != nil {
		return 0, false
	}

	if payload.Usage.PromptTokensDetails.CachedTokens != nil {
		return *payload.Usage.PromptTokensDetails.CachedTokens, true
	}
	if payload.Usage.CachedTokens != nil {
		return *payload.Usage.CachedTokens, true
	}
	if payload.Usage.PromptCacheHitTokens != nil {
		return *payload.Usage.PromptCacheHitTokens, true
	}
	return 0, false
}

// extractMoonshotCachedTokensFromBody 从Moonshot的非标准位置提取cached_tokens
// Moonshot的流式响应格式: {"choices":[{"usage":{"cached_tokens":111}}]}
func extractMoonshotCachedTokensFromBody(body []byte) (int, bool) {
	if len(body) == 0 {
		return 0, false
	}

	var payload struct {
		Choices []struct {
			Usage struct {
				CachedTokens *int `json:"cached_tokens"`
			} `json:"usage"`
		} `json:"choices"`
	}

	if err := common.Unmarshal(body, &payload); err != nil {
		return 0, false
	}

	// 遍历choices查找cached_tokens
	for _, choice := range payload.Choices {
		if choice.Usage.CachedTokens != nil && *choice.Usage.CachedTokens > 0 {
			return *choice.Usage.CachedTokens, true
		}
	}

	return 0, false
}

// extractLlamaCachedTokensFromBody 从llama.cpp的非标准位置提取cache_n
func extractLlamaCachedTokensFromBody(body []byte) (int, bool) {
	if len(body) == 0 {
		return 0, false
	}

	var payload struct {
		Timings struct {
			CachedTokens *int `json:"cache_n"`
		} `json:"timings"`
	}

	if err := common.Unmarshal(body, &payload); err != nil {
		return 0, false
	}

	if payload.Timings.CachedTokens == nil {
		return 0, false
	}
	return *payload.Timings.CachedTokens, true
}
