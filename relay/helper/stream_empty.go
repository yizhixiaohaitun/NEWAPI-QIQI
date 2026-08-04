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
//   - 正常收尾（done / eof / handler_stop）→ 返回 nil，即使 0 数据也不改变现有行为
//     （eof 0 数据由各渠道既有的 usage 兜底逻辑处理，保持保守不扩大判定范围）。
//
// 状态码约定与 Responses 流的 responsesStreamPrematureEndError 保持一致：
//   - client_gone → 499 + SkipRetry（客户端已断开，重试无意义）；
//   - timeout → 504（是否重试由「自动重试状态码」配置决定，默认 504 不重试）；
//   - 其他异常（scanner_error / ping_fail / panic）→ 502，走默认重试配置。
func StreamAbnormalEmptyError(c *gin.Context, info *relaycommon.RelayInfo) *types.NewAPIError {
	if info == nil || info.StreamStatus == nil {
		return nil
	}
	if info.ReceivedResponseCount > 0 {
		return nil
	}
	if info.StreamStatus.IsNormalEnd() {
		return nil
	}
	reason := info.StreamStatus.EndReason
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
		)
	default:
		return types.NewOpenAIError(
			fmt.Errorf("upstream stream ended before producing any data (reason=%s)", reason),
			types.ErrorCodeBadResponse,
			http.StatusBadGateway,
		)
	}
}
