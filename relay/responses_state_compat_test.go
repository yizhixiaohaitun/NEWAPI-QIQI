package relay

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestMissingResponsesReasoningItemIDMatchesExactUpstreamError(t *testing.T) {
	err := types.WithOpenAIError(types.OpenAIError{
		Message: "Item with id 'rs_02698e67ed266c42006a4fe3f343ac819395bd611715dd4638' not found.",
		Type:    "invalid_request_error",
		Param:   "input",
	}, http.StatusBadRequest)

	itemID, ok := missingResponsesReasoningItemID(err)
	require.True(t, ok)
	require.Equal(t, "rs_02698e67ed266c42006a4fe3f343ac819395bd611715dd4638", itemID)
}

func TestMissingResponsesReasoningItemIDRejectsUnrelatedBadRequest(t *testing.T) {
	testCases := []types.OpenAIError{
		{Message: "Item with id 'rs_missing' not found.", Type: "invalid_request_error", Param: "tools"},
		{Message: "Item with id 'msg_missing' not found.", Type: "invalid_request_error", Param: "input"},
		{Message: "Item with id 'rs_missing' not found.", Type: "server_error", Param: "input"},
	}

	for _, openAIError := range testCases {
		itemID, ok := missingResponsesReasoningItemID(types.WithOpenAIError(openAIError, http.StatusBadRequest))
		require.False(t, ok)
		require.Empty(t, itemID)
	}
}

func TestRetryMissingResponsesReasoningItemRetriesOnceAndRecordsEvent(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(nil)
	body := []byte(`{"model":"gpt-5.5","input":[{"type":"reasoning","id":"rs_missing","summary":[],"content":[]},{"role":"user","content":"continue"}]}`)
	storage, err := common.CreateBodyStorage(body)
	require.NoError(t, err)
	t.Cleanup(func() { _ = storage.Close() })

	setting := operation_setting.GetQiqiSetting()
	originalEnabled := setting.ResponsesMissingReasoningItemRetryEnabled
	setting.ResponsesMissingReasoningItemRetryEnabled = true
	t.Cleanup(func() { setting.ResponsesMissingReasoningItemRetryEnabled = originalEnabled })

	upstreamError := types.WithOpenAIError(types.OpenAIError{
		Message: "Item with id 'rs_missing' not found.",
		Type:    "invalid_request_error",
		Param:   "input",
	}, http.StatusBadRequest)
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{ChannelId: 17}}
	requestCount := 0

	resp, retryErr, retried := retryMissingResponsesReasoningItem(
		ctx,
		info,
		storage,
		upstreamError,
		func(reader io.Reader) (any, error) {
			requestCount++
			retryBody, readErr := io.ReadAll(reader)
			require.NoError(t, readErr)
			require.NotContains(t, string(retryBody), "rs_missing")
			require.Contains(t, string(retryBody), "continue")
			return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("{}"))}, nil
		},
	)

	require.True(t, retried)
	require.Nil(t, retryErr)
	require.NotNil(t, resp)
	require.Equal(t, 1, requestCount)
	require.Positive(t, info.UpstreamRequestBodySize)

	adminInfo := map[string]interface{}{}
	service.AppendRelayCompatibilityAdminInfo(ctx, adminInfo)
	events, ok := adminInfo["compatibility_events"].([]service.RelayCompatibilityEvent)
	require.True(t, ok)
	require.Len(t, events, 1)
	require.Equal(t, responsesMissingReasoningItemCompatibilityKey, events[0].Key)
	require.Equal(t, "accepted", events[0].Outcome)
	require.Equal(t, "rs_missing", events[0].ItemID)
	require.Equal(t, 1, events[0].Count)

	_, _, retried = retryMissingResponsesReasoningItem(
		ctx,
		info,
		storage,
		upstreamError,
		func(io.Reader) (any, error) {
			requestCount++
			return nil, nil
		},
	)
	require.False(t, retried)
	require.Equal(t, 1, requestCount)
}

func TestRetryMissingResponsesReasoningItemRespectsGlobalSwitch(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(nil)
	storage, err := common.CreateBodyStorage([]byte(`{"input":[{"type":"reasoning","id":"rs_missing","summary":[],"content":[]}]}`))
	require.NoError(t, err)
	t.Cleanup(func() { _ = storage.Close() })

	setting := operation_setting.GetQiqiSetting()
	originalEnabled := setting.ResponsesMissingReasoningItemRetryEnabled
	setting.ResponsesMissingReasoningItemRetryEnabled = false
	t.Cleanup(func() { setting.ResponsesMissingReasoningItemRetryEnabled = originalEnabled })

	upstreamError := types.WithOpenAIError(types.OpenAIError{
		Message: "Item with id 'rs_missing' not found.",
		Type:    "invalid_request_error",
		Param:   "input",
	}, http.StatusBadRequest)
	requestCount := 0

	_, _, retried := retryMissingResponsesReasoningItem(
		ctx,
		&relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{ChannelId: 17}},
		storage,
		upstreamError,
		func(io.Reader) (any, error) {
			requestCount++
			return nil, nil
		},
	)

	require.False(t, retried)
	require.Zero(t, requestCount)
}
