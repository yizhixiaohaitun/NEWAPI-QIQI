package service

import (
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
)

type inputTokenDeductionAudit struct {
	OriginalTextInputTokens int
	AppliedTokens           int
}

func resolveInputTokenDeduction(relayInfo *relaycommon.RelayInfo) int {
	if relayInfo == nil || relayInfo.ChannelMeta == nil {
		return 0
	}
	setting := relayInfo.ChannelSetting
	if deduction, ok := setting.InputTokenDeductionByGroup[relayInfo.UsingGroup]; ok {
		return deduction
	}
	return setting.InputTokenDeduction
}

func applyInputTokenDeduction(relayInfo *relaycommon.RelayInfo, usage *dto.Usage, requested int) (*dto.Usage, inputTokenDeductionAudit) {
	if usage == nil || requested <= 0 {
		return usage, inputTokenDeductionAudit{}
	}
	adjusted := cloneUsageForBilling(usage)

	audioTokens := nonNegative(adjusted.PromptTokensDetails.AudioTokens)
	imageTokens := nonNegative(adjusted.PromptTokensDetails.ImageTokens)
	cacheReadTokens := nonNegative(adjusted.PromptTokensDetails.CachedTokens)
	cacheWriteTokens := adjusted.PromptTokensDetails.CacheCreationTokensTotal()
	usageSemantic := usageSemanticFromUsage(relayInfo, adjusted)
	isAnthropic := usageSemantic == dto.BillingUsageSemanticAnthropic
	isOpenRouterClaudeInclusive := isAnthropic && relayInfo != nil && relayInfo.ChannelMeta != nil &&
		relayInfo.ChannelType == constant.ChannelTypeOpenRouter
	cachesOutsidePrompt := (isAnthropic && !isOpenRouterClaudeInclusive) || isLegacyClaudeDerivedOpenAIUsage(relayInfo, adjusted)

	promptTextTokens := nonNegative(adjusted.PromptTokens - audioTokens - imageTokens)
	ordinaryTokens := promptTextTokens
	originalTextInputTokens := promptTextTokens
	if cachesOutsidePrompt {
		originalTextInputTokens += cacheReadTokens + cacheWriteTokens
	} else {
		ordinaryTokens = nonNegative(promptTextTokens - cacheReadTokens - cacheWriteTokens)
	}
	if adjusted.PromptTokensDetails.TextTokens > 0 && !cachesOutsidePrompt {
		originalTextInputTokens = deductionMinInt(originalTextInputTokens, adjusted.PromptTokensDetails.TextTokens)
		ordinaryTokens = nonNegative(originalTextInputTokens - cacheReadTokens - cacheWriteTokens)
	}

	audit := inputTokenDeductionAudit{OriginalTextInputTokens: originalTextInputTokens}
	remaining := deductionMinInt(nonNegative(requested), originalTextInputTokens)
	if remaining == 0 {
		return adjusted, audit
	}

	ordinaryDeduction := deductionMinInt(remaining, ordinaryTokens)
	remaining -= ordinaryDeduction
	adjusted.PromptTokens = nonNegative(adjusted.PromptTokens - ordinaryDeduction)

	cacheReadDeduction := deductionMinInt(remaining, cacheReadTokens)
	remaining -= cacheReadDeduction
	adjusted.PromptTokensDetails.CachedTokens = nonNegative(cacheReadTokens - cacheReadDeduction)
	if adjusted.PromptCacheHitTokens != 0 {
		adjusted.PromptCacheHitTokens = adjusted.PromptTokensDetails.CachedTokens
	}
	if !cachesOutsidePrompt {
		adjusted.PromptTokens = nonNegative(adjusted.PromptTokens - cacheReadDeduction)
	}

	cacheWriteDeduction := deductionMinInt(remaining, cacheWriteTokens)
	remaining -= cacheWriteDeduction
	applyCacheWriteDeduction(adjusted, cacheWriteTokens, cacheWriteDeduction)
	if !cachesOutsidePrompt {
		adjusted.PromptTokens = nonNegative(adjusted.PromptTokens - cacheWriteDeduction)
	}

	audit.AppliedTokens = ordinaryDeduction + cacheReadDeduction + cacheWriteDeduction
	if adjusted.InputTokens > 0 {
		adjusted.InputTokens = nonNegative(adjusted.InputTokens - audit.AppliedTokens)
	}
	if adjusted.PromptTokensDetails.TextTokens > 0 {
		adjusted.PromptTokensDetails.TextTokens = nonNegative(adjusted.PromptTokensDetails.TextTokens - audit.AppliedTokens)
	}
	if adjusted.InputTokensDetails != nil {
		adjusted.InputTokensDetails.CachedTokens = adjusted.PromptTokensDetails.CachedTokens
		adjusted.InputTokensDetails.CachedCreationTokens = adjusted.PromptTokensDetails.CachedCreationTokens
		adjusted.InputTokensDetails.CacheWriteTokens = adjusted.PromptTokensDetails.CacheWriteTokens
		if adjusted.InputTokensDetails.TextTokens > 0 {
			adjusted.InputTokensDetails.TextTokens = nonNegative(adjusted.InputTokensDetails.TextTokens - audit.AppliedTokens)
		}
	}
	adjusted.TotalTokens = adjusted.PromptTokens + adjusted.CompletionTokens
	return adjusted, audit
}

func applyCacheWriteDeduction(usage *dto.Usage, originalTotal, deduction int) {
	if deduction <= 0 {
		return
	}
	remainingDeduction := deduction
	fiveMinuteDeduction := deductionMinInt(remainingDeduction, nonNegative(usage.ClaudeCacheCreation5mTokens))
	usage.ClaudeCacheCreation5mTokens -= fiveMinuteDeduction
	remainingDeduction -= fiveMinuteDeduction

	oneHourDeduction := deductionMinInt(remainingDeduction, nonNegative(usage.ClaudeCacheCreation1hTokens))
	usage.ClaudeCacheCreation1hTokens -= oneHourDeduction
	remainingDeduction -= oneHourDeduction

	remainingTotal := nonNegative(originalTotal - deduction)
	if usage.PromptTokensDetails.CachedCreationTokens != 0 {
		usage.PromptTokensDetails.CachedCreationTokens = remainingTotal
	}
	if usage.PromptTokensDetails.CacheWriteTokens != 0 {
		usage.PromptTokensDetails.CacheWriteTokens = remainingTotal
	}

	// Unsplit cache writes have no 5m/1h detail to update. If split details do
	// exist, keep their sum no greater than the normalized cache-write total.
	splitTotal := usage.ClaudeCacheCreation5mTokens + usage.ClaudeCacheCreation1hTokens
	if splitTotal > remainingTotal {
		overflow := splitTotal - remainingTotal
		oneHourTrim := deductionMinInt(overflow, usage.ClaudeCacheCreation1hTokens)
		usage.ClaudeCacheCreation1hTokens -= oneHourTrim
		overflow -= oneHourTrim
		usage.ClaudeCacheCreation5mTokens = nonNegative(usage.ClaudeCacheCreation5mTokens - overflow)
	}
}

// AdjustUsageForClient applies the same channel deduction used by settlement to
// an isolated, normalized usage value for client responses. The returned usage
// never contains the internal BillingUsage audit snapshot. Callers must keep
// returning the original usage to PostTextConsumeQuota so settlement applies
// the deduction exactly once.
func AdjustUsageForClient(relayInfo *relaycommon.RelayInfo, usage *dto.Usage) *dto.Usage {
	requested := resolveInputTokenDeduction(relayInfo)
	if usage == nil || requested <= 0 {
		return usage
	}
	adjusted, _ := applyInputTokenDeduction(relayInfo, effectiveBillingUsage(usage), requested)
	if adjusted != nil {
		if usage.PromptCacheHitTokens != 0 {
			adjusted.PromptCacheHitTokens = adjusted.PromptTokensDetails.CachedTokens
		}
		adjusted.BillingUsage = nil
	}
	return adjusted
}

// AdjustClaudeUsageForClient preserves Claude's native cache buckets while
// applying the shared ordinary -> cache-read -> cache-write deduction order.
// Internal billing_usage is deliberately omitted from the public result.
func AdjustClaudeUsageForClient(relayInfo *relaycommon.RelayInfo, usage *dto.ClaudeUsage) *dto.ClaudeUsage {
	if usage == nil || resolveInputTokenDeduction(relayInfo) <= 0 {
		return usage
	}
	billing := dto.NewClaudeMessagesBillingUsage(usage)
	adjusted := AdjustUsageForClient(relayInfo, &dto.Usage{BillingUsage: billing})
	if adjusted == nil {
		return usage
	}

	result := *usage
	result.InputTokens = adjusted.PromptTokens
	result.OutputTokens = adjusted.CompletionTokens
	result.CacheReadInputTokens = adjusted.PromptTokensDetails.CachedTokens
	result.CacheCreationInputTokens = adjusted.PromptTokensDetails.CacheCreationTokensTotal()
	result.ClaudeCacheCreation5mTokens = adjusted.ClaudeCacheCreation5mTokens
	result.ClaudeCacheCreation1hTokens = adjusted.ClaudeCacheCreation1hTokens
	if usage.CacheCreation != nil {
		cacheCreation := *usage.CacheCreation
		cacheCreation.Ephemeral5mInputTokens = adjusted.ClaudeCacheCreation5mTokens
		cacheCreation.Ephemeral1hInputTokens = adjusted.ClaudeCacheCreation1hTokens
		result.CacheCreation = &cacheCreation
	}
	result.BillingUsage = nil
	return &result
}

func hasReportedBillableUsage(usage *dto.Usage) bool {
	if usage == nil {
		return false
	}
	return usage.PromptTokens > 0 || usage.CompletionTokens > 0 || usage.TotalTokens > 0 ||
		usage.InputTokens > 0 || usage.OutputTokens > 0 ||
		usage.PromptTokensDetails.CachedTokens > 0 || usage.PromptTokensDetails.CacheCreationTokensTotal() > 0 ||
		usage.PromptTokensDetails.TextTokens > 0 || usage.PromptTokensDetails.AudioTokens > 0 ||
		usage.PromptTokensDetails.ImageTokens > 0
}

func cloneUsageForBilling(usage *dto.Usage) *dto.Usage {
	if usage == nil {
		return nil
	}
	cloned := *usage
	if usage.InputTokensDetails != nil {
		inputDetails := *usage.InputTokensDetails
		cloned.InputTokensDetails = &inputDetails
	}
	cloned.BillingUsage = dto.CloneBillingUsage(usage.BillingUsage)
	return &cloned
}

func nonNegative(value int) int {
	if value < 0 {
		return 0
	}
	return value
}

func deductionMinInt(left, right int) int {
	if left < right {
		return left
	}
	return right
}
