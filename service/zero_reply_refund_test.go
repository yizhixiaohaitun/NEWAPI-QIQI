package service

import (
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 复用 task_billing_test.go 中的 TestMain / seedUser / seedToken / seedChannel /
// truncate 测试基建（同包共享）。

func setupZeroReplyRefundTest(t *testing.T, enabled bool) (*gin.Context, *relaycommon.RelayInfo) {
	t.Helper()
	truncate(t)
	seedUser(t, 1, 1000)
	seedToken(t, 10, 1, "zero-reply-key", 1000)
	seedChannel(t, 100)

	setting := operation_setting.GetQiqiSetting()
	original := setting.ZeroReplyAutoRefundEnabled
	setting.ZeroReplyAutoRefundEnabled = enabled
	t.Cleanup(func() {
		setting.ZeroReplyAutoRefundEnabled = original
	})

	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)

	relayInfo := &relaycommon.RelayInfo{
		UserId:      1,
		TokenId:     10,
		TokenKey:    "zero-reply-key",
		ChannelMeta: &relaycommon.ChannelMeta{ChannelId: 100},
	}
	return ctx, relayInfo
}

func zeroReplyConsumeParams(promptTokens, completionTokens, quota int) *model.RecordConsumeLogParams {
	return &model.RecordConsumeLogParams{
		ChannelId:        100,
		PromptTokens:     promptTokens,
		CompletionTokens: completionTokens,
		ModelName:        "gpt-test",
		Quota:            quota,
		Content:          "模型倍率 1.00",
		TokenId:          10,
		Group:            "default",
	}
}

func getQuota(t *testing.T, userId int) int {
	t.Helper()
	var user model.User
	require.NoError(t, model.DB.First(&user, "id = ?", userId).Error)
	return user.Quota
}

func getTokenRemain(t *testing.T, tokenId int) int {
	t.Helper()
	var token model.Token
	require.NoError(t, model.DB.First(&token, "id = ?", tokenId).Error)
	return token.RemainQuota
}

func countRefundLogs(t *testing.T) int64 {
	t.Helper()
	var count int64
	require.NoError(t, model.LOG_DB.Model(&model.Log{}).Where("type = ?", model.LogTypeRefund).Count(&count).Error)
	return count
}

func TestZeroReplyAutoRefundEnabledHit(t *testing.T) {
	ctx, relayInfo := setupZeroReplyRefundTest(t, true)

	// 模拟已扣费 50：先真实扣掉，让退款能把余额恢复
	require.NoError(t, model.DecreaseUserQuota(1, 50, false))
	require.NoError(t, model.DecreaseTokenQuota(10, "zero-reply-key", 50))
	require.Equal(t, 950, getQuota(t, 1))
	require.Equal(t, 950, getTokenRemain(t, 10))

	params := zeroReplyConsumeParams(120, 0, 50)
	refunded := MaybeAutoRefundZeroReplyQuota(ctx, relayInfo, params)
	assert.True(t, refunded)

	// 用户与令牌余额恢复
	assert.Equal(t, 1000, getQuota(t, 1))
	assert.Equal(t, 1000, getTokenRemain(t, 10))

	// 原消耗日志参数保留不动（保证 0回复统计/筛选口径不变）
	assert.Equal(t, 50, params.Quota)
	assert.Equal(t, "模型倍率 1.00", params.Content)
	assert.Nil(t, params.Other)
	assert.Equal(t, 120, params.PromptTokens)
	assert.Equal(t, 0, params.CompletionTokens)

	// 写入了一条退款日志，内容注明 zero-reply auto refund
	var logs []model.Log
	require.NoError(t, model.LOG_DB.Where("type = ?", model.LogTypeRefund).Find(&logs).Error)
	require.Len(t, logs, 1)
	assert.Equal(t, 50, logs[0].Quota)
	assert.Equal(t, "gpt-test", logs[0].ModelName)
	assert.Contains(t, logs[0].Content, "zero-reply auto refund")
	assert.Contains(t, logs[0].Other, "zero_reply_auto_refund")
}

func TestZeroReplyAutoRefundDisabledNoChange(t *testing.T) {
	ctx, relayInfo := setupZeroReplyRefundTest(t, false)

	params := zeroReplyConsumeParams(120, 0, 50)
	refunded := MaybeAutoRefundZeroReplyQuota(ctx, relayInfo, params)
	assert.False(t, refunded)

	// 余额不变、日志参数不变、无退款日志
	assert.Equal(t, 1000, getQuota(t, 1))
	assert.Equal(t, 1000, getTokenRemain(t, 10))
	assert.Equal(t, 50, params.Quota)
	assert.Equal(t, "模型倍率 1.00", params.Content)
	assert.Equal(t, int64(0), countRefundLogs(t))
}

func TestZeroReplyAutoRefundConditionNotMet(t *testing.T) {
	ctx, relayInfo := setupZeroReplyRefundTest(t, true)

	// completion > 0 → 不退
	assert.False(t, MaybeAutoRefundZeroReplyQuota(ctx, relayInfo, zeroReplyConsumeParams(120, 5, 50)))
	// prompt = 0 → 不退
	assert.False(t, MaybeAutoRefundZeroReplyQuota(ctx, relayInfo, zeroReplyConsumeParams(0, 0, 50)))
	// quota = 0（本次没扣费）→ 不退
	assert.False(t, MaybeAutoRefundZeroReplyQuota(ctx, relayInfo, zeroReplyConsumeParams(120, 0, 0)))

	assert.Equal(t, 1000, getQuota(t, 1))
	assert.Equal(t, 1000, getTokenRemain(t, 10))
	assert.Equal(t, int64(0), countRefundLogs(t))
}

func TestZeroReplyAutoRefundNoDoubleRefund(t *testing.T) {
	ctx, relayInfo := setupZeroReplyRefundTest(t, true)

	require.NoError(t, model.DecreaseUserQuota(1, 50, false))
	require.NoError(t, model.DecreaseTokenQuota(10, "zero-reply-key", 50))

	// 第一次结算出口：退款成功
	assert.True(t, MaybeAutoRefundZeroReplyQuota(ctx, relayInfo, zeroReplyConsumeParams(120, 0, 50)))
	// 同一请求（同一 gin.Context）第二次结算出口：不再退款
	assert.False(t, MaybeAutoRefundZeroReplyQuota(ctx, relayInfo, zeroReplyConsumeParams(120, 0, 50)))

	// 余额只恢复一次，不会多加
	assert.Equal(t, 1000, getQuota(t, 1))
	assert.Equal(t, 1000, getTokenRemain(t, 10))
	assert.Equal(t, int64(1), countRefundLogs(t))
}

func TestZeroReplyAutoRefundSkipsChannelTest(t *testing.T) {
	ctx, relayInfo := setupZeroReplyRefundTest(t, true)
	relayInfo.IsChannelTest = true

	assert.False(t, MaybeAutoRefundZeroReplyQuota(ctx, relayInfo, zeroReplyConsumeParams(120, 0, 50)))
	assert.Equal(t, 1000, getQuota(t, 1))
	assert.Equal(t, int64(0), countRefundLogs(t))
}
