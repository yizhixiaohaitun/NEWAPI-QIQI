package service

import (
	"fmt"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"

	"github.com/gin-gonic/gin"
)

// MaybeAutoRefundZeroReplyQuota 实现 qiqi「0回复自动回退额度」：
// 当开关开启，且本次请求最终结算为消耗类、prompt_tokens>0、completion_tokens=0
// 且实际扣了 quota>0 时，在结算收口点（PostTextConsumeQuota /
// PostAudioConsumeQuota / PostWssConsumeQuota，消耗日志落库之前）实时把已扣
// 的 quota 原路退回（钱包或订阅 + 令牌额度 + 用户额度缓存，复用
// PostConsumeQuota 的负数路径），并另写一条 LogTypeRefund 日志用于对账。
//
// 原消耗日志保留不动（quota/prompt/completion/content 均不修改），保证既有的
// 0回复统计徽章（zero_reply_count/zero_reply_quota）与伪类型筛选口径
// （type=消耗 AND prompt>0 AND completion=0）完全不受影响；回退金额与原因
// 只体现在新增的 LogTypeRefund 退款日志里（内容注明 zero-reply auto refund，
// 带 request id 便于对账）。
//
// 通过 gin.Context 上的 ContextKeyZeroReplyAutoRefunded 标记保证同一请求
// 至多退款一次（结算路径存在多个出口时也不会重复加钱）。
//
// 返回值表示本次是否实际执行了退款。
func MaybeAutoRefundZeroReplyQuota(ctx *gin.Context, relayInfo *relaycommon.RelayInfo, params *model.RecordConsumeLogParams) bool {
	if ctx == nil || relayInfo == nil || params == nil {
		return false
	}
	if !operation_setting.IsZeroReplyAutoRefundEnabled() {
		return false
	}
	// 与 0回复统计口径一致：有输入、无输出、且本次实际扣了费
	if params.PromptTokens <= 0 || params.CompletionTokens != 0 || params.Quota <= 0 {
		return false
	}
	// 渠道测试请求不参与用户额度退款
	if relayInfo.IsChannelTest {
		return false
	}
	// 防重复：同一请求（同一 gin.Context）只退一次。先置标记再退款，
	// 宁可在退款失败时少退（有错误日志可查），也不允许重复加钱。
	if common.GetContextKeyBool(ctx, constant.ContextKeyZeroReplyAutoRefunded) {
		return false
	}
	common.SetContextKey(ctx, constant.ContextKeyZeroReplyAutoRefunded, true)

	refundQuota := params.Quota

	// 复用现有结算负数路径：钱包/订阅资金来源退回 + 令牌额度退回 + 缓存同步
	if err := PostConsumeQuota(relayInfo, -refundQuota, 0, false); err != nil {
		logger.LogError(ctx, fmt.Sprintf("zero-reply auto refund failed, userId %d, tokenId %d, quota %d: %s",
			relayInfo.UserId, relayInfo.TokenId, refundQuota, err.Error()))
		return false
	}

	requestId := ctx.GetString(common.RequestIdKey)
	logger.LogInfo(ctx, fmt.Sprintf("zero-reply auto refund: refunded %s to user %d (model %s, request id %s)",
		logger.FormatQuota(refundQuota), relayInfo.UserId, params.ModelName, requestId))

	// 另写一条退款日志用于对账（复用任务退款的 LogTypeRefund 惯例）；
	// 原消耗日志参数不做任何修改，保证 0回复统计/筛选口径不变。
	model.RecordTaskBillingLog(model.RecordTaskBillingLogParams{
		UserId:    relayInfo.UserId,
		LogType:   model.LogTypeRefund,
		Content:   fmt.Sprintf("zero-reply auto refund（0回复自动回退额度），request id: %s", requestId),
		ChannelId: params.ChannelId,
		ModelName: params.ModelName,
		Quota:     refundQuota,
		TokenId:   relayInfo.TokenId,
		Group:     relayInfo.UsingGroup,
		Other: map[string]interface{}{
			"zero_reply_auto_refund": true,
			"reason":                 "zero-reply auto refund",
			"request_id":             requestId,
			"refund_quota":           refundQuota,
			"prompt_tokens":          params.PromptTokens,
			"completion_tokens":      params.CompletionTokens,
		},
		RequestId: requestId,
	})
	return true
}
