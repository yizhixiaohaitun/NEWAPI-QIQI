package helper

import (
	"fmt"
	"net/http"

	"github.com/QuantumNous/new-api/logger"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
)

// StreamAbnormalEmptyError 判定「流异常结束且未收到任何上游数据」的确定性失败场景：
// StreamScannerHandler 结束后，若 EndReason 属于非正常收尾
// （timeout / client_gone / scanner_error / ping_fail / panic）且
// ReceivedResponseCount == 0（上游没有投递过任何一条 SSE data，既无
// message_start 也无 usage、无任何输出），说明本次请求确定没有成功，
// 应按失败返回错误、不进入计费，让上层 shouldRetry/换渠道逻辑接管，
// 预扣费会经由 controller 的 Billing.Refund 正常退回。
//
// 边界（避免误伤正常流）：
//   - 上游已投递过任何数据（ReceivedResponseCount > 0）→ 返回 nil，维持现有计费行为；
//   - done / handler_stop 收尾 → 返回 nil（这两种只能由上游 [DONE] 或 dataHandler
//     主动触发，不会出现在 0 数据场景之外的误判）；
//   - eof + 0 数据 → 视为「SSE 未正常收尾」按失败处理：上游 200 后未投递任何
//     数据就关闭连接，各协议（OpenAI [DONE] / Claude message_stop / Dify
//     message_end）正常完成时必然先有数据，因此该组合不可能是正常完成。
//
// 状态码约定与 Responses 流的 responsesStreamPrematureEndError 保持一致：
//   - client_gone → 499 + SkipRetry（客户端已断开，重试无意义）；
//   - timeout → 504 + SkipRetry；
//   - 其他异常（eof / scanner_error / ping_fail / panic）→ 502 + SkipRetry。
//
// 重试策略（用户决策）：0 数据异常断流（上游切断连接）一律不做网关侧重试，
// 三类分支全部 SkipRetry、仅保留各自状态码原样透传给下游——由下游 agent
// 拿到真实错误码后自行决定是否重试，避免网关+下游双层重试叠加放大消耗。
func StreamAbnormalEmptyError(c *gin.Context, info *relaycommon.RelayInfo) *types.NewAPIError {
	if info == nil || info.StreamStatus == nil {
		return nil
	}
	if info.ReceivedResponseCount > 0 {
		return nil
	}
	reason := info.StreamStatus.EndReason
	// done / handler_stop 由上游 [DONE] 或 dataHandler 显式触发，维持现状；
	// eof 属于「SSE 未正常收尾」——上游 200 后一条数据都没投递就关闭了连接，
	// 与 timeout/scanner_error 同样按失败处理。
	if reason == relaycommon.StreamEndReasonDone || reason == relaycommon.StreamEndReasonHandlerStop {
		return nil
	}
	logger.LogError(c, fmt.Sprintf("stream ended abnormally with zero upstream data (reason=%s), treat as failure without billing", reason))
	switch reason {
	case relaycommon.StreamEndReasonClientGone:
		return types.NewOpenAIError(
			fmt.Errorf("client disconnected before upstream produced any data"),
			types.ErrorCodeBadResponse,
			499,
			types.ErrOptionWithSkipRetry(),
		)
	case relaycommon.StreamEndReasonTimeout:
		return types.NewOpenAIError(
			fmt.Errorf("upstream stream timed out before producing any data"),
			types.ErrorCodeBadResponse,
			http.StatusGatewayTimeout,
			types.ErrOptionWithSkipRetry(),
		)
	default:
		return types.NewOpenAIError(
			fmt.Errorf("upstream stream ended before producing any data (reason=%s)", reason),
			types.ErrorCodeBadResponse,
			http.StatusBadGateway,
			types.ErrOptionWithSkipRetry(),
		)
	}
}
