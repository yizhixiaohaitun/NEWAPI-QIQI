package service

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestResolveInputTokenDeductionUsesFinalGroupOverride(t *testing.T) {
	relayInfo := &relaycommon.RelayInfo{
		UsingGroup: "priority",
		ChannelMeta: &relaycommon.ChannelMeta{ChannelSetting: dto.ChannelSettings{
			InputTokenDeduction: 490,
			InputTokenDeductionByGroup: map[string]int{
				"priority": 0,
				"default":  200,
			},
		}},
	}

	require.Zero(t, resolveInputTokenDeduction(relayInfo), "an explicit zero disables the channel default")
	relayInfo.UsingGroup = "default"
	require.Equal(t, 200, resolveInputTokenDeduction(relayInfo))
	relayInfo.UsingGroup = "retry-final"
	require.Equal(t, 490, resolveInputTokenDeduction(relayInfo))
}

func TestApplyInputTokenDeductionOpenAIKeepsOriginalIndependent(t *testing.T) {
	original := &dto.Usage{
		PromptTokens:     500,
		CompletionTokens: 20,
		TotalTokens:      520,
		InputTokens:      500,
		OutputTokens:     20,
		UsageSemantic:    dto.BillingUsageSemanticOpenAI,
		PromptTokensDetails: dto.InputTokenDetails{
			TextTokens: 500,
		},
	}

	adjusted, audit := applyInputTokenDeduction(nil, original, 490)

	require.Equal(t, 500, audit.OriginalTextInputTokens)
	require.Equal(t, 490, audit.AppliedTokens)
	require.Equal(t, 10, adjusted.PromptTokens)
	require.Equal(t, 10, adjusted.InputTokens)
	require.Equal(t, 30, adjusted.TotalTokens)
	require.Equal(t, 10, adjusted.PromptTokensDetails.TextTokens)
	require.Equal(t, 500, original.PromptTokens)
	require.Equal(t, 500, original.PromptTokensDetails.TextTokens)
}

func TestApplyInputTokenDeductionCacheOrderAndClaudeSplitConsistency(t *testing.T) {
	original := &dto.Usage{
		PromptTokens:     100,
		CompletionTokens: 9,
		TotalTokens:      109,
		InputTokens:      180,
		UsageSemantic:    dto.BillingUsageSemanticAnthropic,
		PromptTokensDetails: dto.InputTokenDetails{
			CachedTokens:         30,
			CachedCreationTokens: 50,
		},
		ClaudeCacheCreation5mTokens: 20,
		ClaudeCacheCreation1hTokens: 30,
	}

	adjusted, audit := applyInputTokenDeduction(nil, original, 145)

	require.Equal(t, 180, audit.OriginalTextInputTokens)
	require.Equal(t, 145, audit.AppliedTokens)
	require.Zero(t, adjusted.PromptTokens, "ordinary input is deducted first")
	require.Zero(t, adjusted.PromptTokensDetails.CachedTokens, "cache reads are deducted second")
	require.Equal(t, 35, adjusted.PromptTokensDetails.CachedCreationTokens)
	require.Equal(t, 5, adjusted.ClaudeCacheCreation5mTokens, "5m writes are deducted before 1h writes")
	require.Equal(t, 30, adjusted.ClaudeCacheCreation1hTokens)
	require.Equal(t, 35, adjusted.InputTokens)
	require.Equal(t, 9, adjusted.TotalTokens)
	require.Equal(t, 180, original.InputTokens)
	require.Equal(t, 20, original.ClaudeCacheCreation5mTokens)
}

func TestApplyInputTokenDeductionDoesNotDeductNonTextModalities(t *testing.T) {
	usage := &dto.Usage{
		PromptTokens:     110,
		CompletionTokens: 4,
		TotalTokens:      114,
		UsageSemantic:    dto.BillingUsageSemanticGemini,
		PromptTokensDetails: dto.InputTokenDetails{
			TextTokens:  100,
			AudioTokens: 7,
			ImageTokens: 3,
		},
	}

	adjusted, audit := applyInputTokenDeduction(nil, usage, 500)

	require.Equal(t, 100, audit.OriginalTextInputTokens)
	require.Equal(t, 100, audit.AppliedTokens)
	require.Equal(t, 10, adjusted.PromptTokens)
	require.Equal(t, 7, adjusted.PromptTokensDetails.AudioTokens)
	require.Equal(t, 3, adjusted.PromptTokensDetails.ImageTokens)
	require.Equal(t, 4, adjusted.CompletionTokens)
}

func TestApplyInputTokenDeductionClonesBillingUsageAndInputDetailsPointers(t *testing.T) {
	inputDetails := &dto.InputTokenDetails{TextTokens: 50}
	usage := &dto.Usage{
		PromptTokens:       50,
		TotalTokens:        50,
		InputTokensDetails: inputDetails,
		BillingUsage: dto.NewOpenAIChatBillingUsage(&dto.Usage{
			PromptTokens: 50,
			TotalTokens:  50,
		}),
	}

	adjusted, _ := applyInputTokenDeduction(nil, usage, 10)

	require.NotSame(t, usage, adjusted)
	require.NotSame(t, usage.InputTokensDetails, adjusted.InputTokensDetails)
	require.NotSame(t, usage.BillingUsage, adjusted.BillingUsage)
	require.Equal(t, 50, usage.BillingUsage.OpenAIUsage.PromptTokens)
}

func TestApplyInputTokenDeductionUsesFinalClaudeSemanticForLegacyUsage(t *testing.T) {
	relayInfo := &relaycommon.RelayInfo{
		FinalRequestRelayFormat: types.RelayFormatOpenAI,
		OriginModelName:         "claude-legacy",
		PriceData: types.PriceData{
			ModelRatio:         1,
			CacheRatio:         0.1,
			CacheCreationRatio: 1.25,
			GroupRatioInfo:     types.GroupRatioInfo{GroupRatio: 1},
		},
	}
	usage := &dto.Usage{
		PromptTokens: 100,
		PromptTokensDetails: dto.InputTokenDetails{
			CachedTokens:         30,
			CachedCreationTokens: 20,
		},
		ClaudeCacheCreation5mTokens: 20,
	}

	adjusted, audit := applyInputTokenDeduction(relayInfo, usage, 110)
	summary := calculateTextQuotaSummary(testGinContext(), relayInfo, adjusted)

	require.Equal(t, 150, audit.OriginalTextInputTokens)
	require.Equal(t, 110, audit.AppliedTokens)
	require.Zero(t, adjusted.PromptTokens)
	require.Equal(t, 20, adjusted.PromptTokensDetails.CachedTokens)
	require.Equal(t, 20, adjusted.PromptTokensDetails.CachedCreationTokens)
	require.GreaterOrEqual(t, summary.Quota, 0)
}

func TestApplyInputTokenDeductionOpenRouterClaudeTreatsPromptAsInclusive(t *testing.T) {
	relayInfo := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{ChannelType: constant.ChannelTypeOpenRouter},
		PriceData: types.PriceData{
			ModelRatio:         1,
			CacheRatio:         0.1,
			CacheCreationRatio: 1.25,
			GroupRatioInfo:     types.GroupRatioInfo{GroupRatio: 1},
		},
	}
	usage := &dto.Usage{
		PromptTokens:  100,
		UsageSemantic: dto.BillingUsageSemanticAnthropic,
		PromptTokensDetails: dto.InputTokenDetails{
			CachedTokens:         30,
			CachedCreationTokens: 20,
		},
	}

	adjusted, audit := applyInputTokenDeduction(relayInfo, usage, 70)
	summary := calculateTextQuotaSummary(testGinContext(), relayInfo, adjusted)

	require.Equal(t, 100, audit.OriginalTextInputTokens)
	require.Equal(t, 70, audit.AppliedTokens)
	require.Equal(t, 30, adjusted.PromptTokens)
	require.Equal(t, 10, adjusted.PromptTokensDetails.CachedTokens)
	require.Equal(t, 20, adjusted.PromptTokensDetails.CachedCreationTokens)
	require.GreaterOrEqual(t, summary.PromptTokens, 0)
	require.GreaterOrEqual(t, summary.Quota, 0)
}

func testGinContext() *gin.Context {
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	return ctx
}

func TestInputTokenDeductionFeedsTieredMinimumWithAdjustedInput(t *testing.T) {
	adjusted, audit := applyInputTokenDeduction(nil, &dto.Usage{PromptTokens: 500, TotalTokens: 500}, 490)
	require.Equal(t, 490, audit.AppliedTokens)
	relayInfo := &relaycommon.RelayInfo{TieredBillingSnapshot: &billingexpr.BillingSnapshot{
		BillingMode:  "tiered_expr",
		ExprString:   `tier("base", max(p, 20))`,
		ExprHash:     billingexpr.ExprHashString(`tier("base", max(p, 20))`),
		GroupRatio:   1,
		QuotaPerUnit: 500_000,
	}}

	ok, quota, _ := TryTieredSettle(relayInfo, BuildTieredTokenParams(adjusted, false, nil))
	require.True(t, ok)
	require.Equal(t, 10, quota, "the tiered minimum of 20 tokens is applied after 500-490")
}

func TestPostTextConsumeQuotaDeductsSQLiteBalanceAndConsumeLog(t *testing.T) {
	truncate(t)
	seedUser(t, 31, 1000)
	seedToken(t, 32, 31, "deduction-key", 1000)
	seedChannel(t, 33)

	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)
	relayInfo := &relaycommon.RelayInfo{
		UserId:          31,
		TokenId:         32,
		TokenKey:        "deduction-key",
		OriginModelName: "gpt-test",
		UsingGroup:      "retry-final",
		StartTime:       time.Now(),
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelId: 33,
			ChannelSetting: dto.ChannelSettings{
				InputTokenDeduction: 490,
			},
		},
		PriceData: types.PriceData{
			ModelRatio:      1,
			CompletionRatio: 1,
			GroupRatioInfo:  types.GroupRatioInfo{GroupRatio: 1},
		},
	}
	original := &dto.Usage{PromptTokens: 500, TotalTokens: 500}

	PostTextConsumeQuota(ctx, relayInfo, original, nil)

	require.Equal(t, 990, getQuota(t, 31))
	require.Equal(t, 990, getTokenRemain(t, 32))
	var log model.Log
	require.NoError(t, model.LOG_DB.Where("type = ?", model.LogTypeConsume).First(&log).Error)
	require.Equal(t, 10, log.PromptTokens)
	require.Zero(t, log.CompletionTokens)
	require.Equal(t, 10, log.Quota)
	require.Contains(t, log.Other, `"input_token_deduction_original_input_tokens":500`)
	require.Contains(t, log.Other, `"input_token_deduction_applied":490`)
	require.Equal(t, 500, original.PromptTokens, "the client-facing usage remains unchanged")
}

func TestInputTokenDeductionDoesNotRemoveFixedPerRequestPrice(t *testing.T) {
	ctx := testGinContext()
	relayInfo := &relaycommon.RelayInfo{
		OriginModelName: "fixed-price-model",
		StartTime:       time.Now(),
		PriceData: types.PriceData{
			UsePrice:       true,
			ModelPrice:     0.002,
			GroupRatioInfo: types.GroupRatioInfo{GroupRatio: 1},
		},
	}
	adjusted, audit := applyInputTokenDeduction(relayInfo, &dto.Usage{PromptTokens: 100, TotalTokens: 100}, 500)
	summary := calculateTextQuotaSummary(ctx, relayInfo, adjusted)
	preserveFixedPriceAfterInputTokenDeduction(ctx, relayInfo, &dto.Usage{PromptTokens: 100, TotalTokens: 100}, audit, &summary)

	require.Equal(t, 100, audit.AppliedTokens)
	require.Zero(t, summary.PromptTokens)
	require.Equal(t, 1000, summary.Quota, "fixed per-request pricing is independent of token deduction")
}

func TestZeroUsageFixedPriceWithoutDeductionKeepsLegacyZeroQuota(t *testing.T) {
	relayInfo := &relaycommon.RelayInfo{
		OriginModelName: "fixed-price-model",
		StartTime:       time.Now(),
		PriceData: types.PriceData{
			UsePrice:       true,
			ModelPrice:     0.002,
			GroupRatioInfo: types.GroupRatioInfo{GroupRatio: 1},
		},
	}
	summary := calculateTextQuotaSummary(testGinContext(), relayInfo, &dto.Usage{})
	require.Zero(t, summary.Quota)
}

func TestPostTextConsumeQuotaFullyDeductedStillCountsSuccessfulRequest(t *testing.T) {
	truncate(t)
	seedUser(t, 41, 1000)
	seedToken(t, 42, 41, "fully-deducted-key", 1000)
	seedChannel(t, 43)

	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)
	relayInfo := &relaycommon.RelayInfo{
		UserId: 41, TokenId: 42, TokenKey: "fully-deducted-key", OriginModelName: "gpt-test",
		UsingGroup: "default", StartTime: time.Now(),
		ChannelMeta: &relaycommon.ChannelMeta{ChannelId: 43, ChannelSetting: dto.ChannelSettings{InputTokenDeduction: 500}},
		PriceData:   types.PriceData{ModelRatio: 1, CompletionRatio: 1, GroupRatioInfo: types.GroupRatioInfo{GroupRatio: 1}},
	}

	PostTextConsumeQuota(ctx, relayInfo, &dto.Usage{PromptTokens: 100, TotalTokens: 100}, nil)

	var user model.User
	require.NoError(t, model.DB.First(&user, "id = ?", 41).Error)
	require.Equal(t, 1, user.RequestCount)
	require.Zero(t, user.UsedQuota)
	var log model.Log
	require.NoError(t, model.LOG_DB.Where("type = ?", model.LogTypeConsume).First(&log).Error)
	require.Zero(t, log.PromptTokens)
	require.Zero(t, log.Quota)
	require.NotContains(t, log.Content, "上游没有返回计费信息")
}

func TestApplyInputTokenDeductionNoConfigIsNoOpWithoutClone(t *testing.T) {
	usage := &dto.Usage{PromptTokens: 12, TotalTokens: 12}
	adjusted, audit := applyInputTokenDeduction(nil, usage, 0)
	require.Same(t, usage, adjusted)
	require.Zero(t, audit.AppliedTokens)
}

func BenchmarkApplyInputTokenDeduction(b *testing.B) {
	usage := &dto.Usage{
		PromptTokens:     500,
		CompletionTokens: 20,
		TotalTokens:      520,
		InputTokens:      500,
		UsageSemantic:    dto.BillingUsageSemanticOpenAI,
		PromptTokensDetails: dto.InputTokenDetails{
			TextTokens: 500,
		},
	}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = applyInputTokenDeduction(nil, usage, 490)
	}
}
