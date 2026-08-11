package doubao

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newDoubaoTaskContext(t *testing.T, body string) (*gin.Context, *relaycommon.RelayInfo) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	request := httptest.NewRequest(http.MethodPost, "/v1/video/generations", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	context, _ := gin.CreateTestContext(httptest.NewRecorder())
	context.Request = request
	return context, &relaycommon.RelayInfo{
		TaskRelayInfo: &relaycommon.TaskRelayInfo{},
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: "doubao-seedance-2-0-260128",
		},
	}
}

func forwardedDoubaoPayload(t *testing.T, adaptor *TaskAdaptor, context *gin.Context, info *relaycommon.RelayInfo) map[string]any {
	t.Helper()
	reader, err := adaptor.BuildRequestBody(context, info)
	require.NoError(t, err)
	body, err := io.ReadAll(reader)
	require.NoError(t, err)
	var payload map[string]any
	require.NoError(t, common.Unmarshal(body, &payload))
	return payload
}

func TestSeedanceDurationOnlyIsUsedForBillingAndForwarding(t *testing.T) {
	for _, duration := range []string{`5`, `"5"`} {
		t.Run(duration, func(t *testing.T) {
			body := `{"model":"seedance-2.0","prompt":"a paper boat","duration":` + duration + `,"metadata":{"resolution":"720p","ratio":"16:9","generate_audio":false}}`
			context, info := newDoubaoTaskContext(t, body)
			adaptor := &TaskAdaptor{}

			require.Nil(t, adaptor.ValidateRequestAndSetAction(context, info))
			ratios := adaptor.EstimateBilling(context, info)
			assert.Equal(t, float64(5), ratios["seconds"])

			payload := forwardedDoubaoPayload(t, adaptor, context, info)
			assert.Equal(t, float64(5), payload["duration"])
			assert.Equal(t, "720p", payload["resolution"])
			assert.Equal(t, "16:9", payload["ratio"])
			assert.Equal(t, false, payload["generate_audio"])
			assert.NotContains(t, payload, "seconds")
		})
	}
}

func TestSeedanceLegacySecondsIsUsedForBillingAndForwarding(t *testing.T) {
	context, info := newDoubaoTaskContext(t, `{"model":"doubao-seedance-1-5-pro-251215","prompt":"clouds","seconds":"6","metadata":{"resolution":"1080p"}}`)
	adaptor := &TaskAdaptor{}

	require.Nil(t, adaptor.ValidateRequestAndSetAction(context, info))
	ratios := adaptor.EstimateBilling(context, info)
	assert.Equal(t, float64(6), ratios["seconds"])

	payload := forwardedDoubaoPayload(t, adaptor, context, info)
	assert.Equal(t, float64(6), payload["duration"])
	assert.NotContains(t, payload, "seconds")
}

func TestSeedanceDurationTakesPrecedenceOverLegacySeconds(t *testing.T) {
	context, info := newDoubaoTaskContext(t, `{"model":"doubao-seedance-1-0-pro-250528","prompt":"clouds","duration":7,"seconds":"4"}`)
	adaptor := &TaskAdaptor{}

	require.Nil(t, adaptor.ValidateRequestAndSetAction(context, info))
	assert.Equal(t, float64(7), adaptor.EstimateBilling(context, info)["seconds"])
	assert.Equal(t, float64(7), forwardedDoubaoPayload(t, adaptor, context, info)["duration"])
}
