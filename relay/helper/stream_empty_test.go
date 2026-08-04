package helper

import (
	"net/http"
	"net/http/httptest"
	"testing"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newStreamEmptyTestCtx() *gin.Context {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)
	return c
}

func newRelayInfoWithStream(reason relaycommon.StreamEndReason, received int) *relaycommon.RelayInfo {
	ss := relaycommon.NewStreamStatus()
	if reason != relaycommon.StreamEndReasonNone {
		ss.SetEndReason(reason, nil)
	}
	return &relaycommon.RelayInfo{
		StreamStatus:          ss,
		ReceivedResponseCount: received,
	}
}

// 0数据 + 异常断流 → 返回错误（不进入计费）
func TestStreamAbnormalEmptyError_ZeroDataAbnormalEnd(t *testing.T) {
	// timeout → 504
	err := StreamAbnormalEmptyError(newStreamEmptyTestCtx(), newRelayInfoWithStream(relaycommon.StreamEndReasonTimeout, 0))
	require.NotNil(t, err)
	assert.Equal(t, http.StatusGatewayTimeout, err.StatusCode)

	// client_gone → 499 + skip retry
	err = StreamAbnormalEmptyError(newStreamEmptyTestCtx(), newRelayInfoWithStream(relaycommon.StreamEndReasonClientGone, 0))
	require.NotNil(t, err)
	assert.Equal(t, 499, err.StatusCode)
	assert.True(t, types.IsSkipRetryError(err))

	// scanner_error → 502（可按配置重试换渠道）
	err = StreamAbnormalEmptyError(newStreamEmptyTestCtx(), newRelayInfoWithStream(relaycommon.StreamEndReasonScannerErr, 0))
	require.NotNil(t, err)
	assert.Equal(t, http.StatusBadGateway, err.StatusCode)
	assert.False(t, types.IsSkipRetryError(err))

	// ping_fail / panic → 502
	require.NotNil(t, StreamAbnormalEmptyError(newStreamEmptyTestCtx(), newRelayInfoWithStream(relaycommon.StreamEndReasonPingFail, 0)))
	require.NotNil(t, StreamAbnormalEmptyError(newStreamEmptyTestCtx(), newRelayInfoWithStream(relaycommon.StreamEndReasonPanic, 0)))
}

// 有部分数据 → 维持旧行为（返回 nil，照常走既有计费路径）
func TestStreamAbnormalEmptyError_PartialDataKeepsBilling(t *testing.T) {
	assert.Nil(t, StreamAbnormalEmptyError(newStreamEmptyTestCtx(), newRelayInfoWithStream(relaycommon.StreamEndReasonTimeout, 1)))
	assert.Nil(t, StreamAbnormalEmptyError(newStreamEmptyTestCtx(), newRelayInfoWithStream(relaycommon.StreamEndReasonClientGone, 3)))
	assert.Nil(t, StreamAbnormalEmptyError(newStreamEmptyTestCtx(), newRelayInfoWithStream(relaycommon.StreamEndReasonScannerErr, 100)))
}

// 正常流（done/eof/handler_stop）→ 不受影响
func TestStreamAbnormalEmptyError_NormalEndUnaffected(t *testing.T) {
	assert.Nil(t, StreamAbnormalEmptyError(newStreamEmptyTestCtx(), newRelayInfoWithStream(relaycommon.StreamEndReasonDone, 0)))
	assert.Nil(t, StreamAbnormalEmptyError(newStreamEmptyTestCtx(), newRelayInfoWithStream(relaycommon.StreamEndReasonDone, 10)))
	assert.Nil(t, StreamAbnormalEmptyError(newStreamEmptyTestCtx(), newRelayInfoWithStream(relaycommon.StreamEndReasonEOF, 0)))
	assert.Nil(t, StreamAbnormalEmptyError(newStreamEmptyTestCtx(), newRelayInfoWithStream(relaycommon.StreamEndReasonHandlerStop, 0)))
}

// 防御分支：nil info / nil StreamStatus
func TestStreamAbnormalEmptyError_NilSafe(t *testing.T) {
	assert.Nil(t, StreamAbnormalEmptyError(newStreamEmptyTestCtx(), nil))
	assert.Nil(t, StreamAbnormalEmptyError(newStreamEmptyTestCtx(), &relaycommon.RelayInfo{}))
}
