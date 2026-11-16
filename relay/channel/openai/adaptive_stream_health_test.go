package openai

import (
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/stretchr/testify/require"
)

func TestAdaptiveResponsesStreamErrorsDoNotBecomeSuccess(t *testing.T) {
	oldTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() { constant.StreamingTimeout = oldTimeout })
	old := operation_setting.GetQiqiSetting().ResponsesStreamErrorRetryEnabled
	t.Cleanup(func() { operation_setting.GetQiqiSetting().ResponsesStreamErrorRetryEnabled = old })
	for _, enabled := range []bool{false, true} {
		operation_setting.GetQiqiSetting().ResponsesStreamErrorRetryEnabled = enabled
		for _, prelude := range []string{"", "data: {\"type\":\"response.output_text.delta\",\"delta\":\"hello\"}\n\n"} {
			body := prelude + "data: {\"type\":\"response.failed\",\"response\":{\"error\":{\"type\":\"server_error\",\"code\":\"server_error\",\"message\":\"busy\"}}}\n\n"
			c, _, resp, info := newResponsesStreamTestContext(t, body)
			_, err := OaiResponsesStreamHandler(c, info, resp)
			require.Equal(t, service.AdaptiveRoutingFailure, service.AdaptiveRelayOutcome(err, info.StreamStatus, nil, nil, false), "retry=%v committed=%v", enabled, prelude != "")
		}
	}
}
