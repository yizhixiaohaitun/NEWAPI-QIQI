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
