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

func TestEncryptedContentVerificationErrorMatchesWithoutRetainingCiphertext(t *testing.T) {
	ciphertext := "gAAAAABsecret-payload-that-must-not-be-returned"
	upstreamError := types.WithOpenAIError(types.OpenAIError{
		Message: "The encrypted content " + ciphertext + " could not be verified. Reason: Encrypted content could not be decrypted or parsed.",
		Type:    "invalid_request_error",
		Param:   "input",
	}, http.StatusBadRequest)

	require.True(t, isEncryptedContentVerificationError(upstreamError))
	userError := encryptedContentRecoveryUserError(upstreamError)
	require.Equal(t, http.StatusBadRequest, userError.StatusCode)
	require.Contains(t, userError.Error(), "加密推理内容")
	require.NotContains(t, userError.Error(), ciphertext)
	require.True(t, types.IsSkipRetryError(userError))

	unrelated := types.WithOpenAIError(types.OpenAIError{
		Message: "The encrypted content is invalid",
		Type:    "invalid_request_error",
	}, http.StatusBadRequest)
	require.False(t, isEncryptedContentVerificationError(unrelated))
}

func TestRetryUndecryptableResponsesReasoningContentRetriesOnlyOnce(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(nil)
	storage, err := common.CreateBodyStorage([]byte(`{"input":[{"type":"reasoning","id":"rs_bad","encrypted_content":"gAAAAA-private"},{"role":"user","content":"continue"}]}`))
	require.NoError(t, err)
	t.Cleanup(func() { _ = storage.Close() })

	upstreamError := types.WithOpenAIError(types.OpenAIError{
		Message: "The encrypted content gAAAAA-private could not be verified. Reason: Encrypted content could not be decrypted or parsed.",
		Type:    "invalid_request_error",
		Param:   "input",
	}, http.StatusBadRequest)
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{ChannelId: 23}}
	requestCount := 0
	doRequest := func(reader io.Reader) (any, error) {
		requestCount++
		body, readErr := io.ReadAll(reader)
		require.NoError(t, readErr)
		require.NotContains(t, string(body), "gAAAAA-private")
		require.Contains(t, string(body), "continue")
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{}`))}, nil
	}

	resp, retryErr, retried := retryUndecryptableResponsesReasoningContent(ctx, info, storage, upstreamError, doRequest)
	require.True(t, retried)
	require.Nil(t, retryErr)
	require.NotNil(t, resp)
	require.Equal(t, 1, requestCount)

	_, _, retried = retryUndecryptableResponsesReasoningContent(ctx, info, storage, upstreamError, doRequest)
	require.False(t, retried)
	require.Equal(t, 1, requestCount)

	adminInfo := map[string]interface{}{}
	service.AppendRelayCompatibilityAdminInfo(ctx, adminInfo)
	events, ok := adminInfo["compatibility_events"].([]service.RelayCompatibilityEvent)
	require.True(t, ok)
	require.Len(t, events, 1)
	require.Equal(t, service.ResponsesEncryptedContentRecoveryRule.ID, events[0].RuleID)
	require.Equal(t, 1, events[0].Count)
	require.True(t, events[0].Retried)
}

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
	body := []byte(`{"model":"gpt-5.5","input":[{"type":"reasoning","id":"rs_missing","summary":[],"content":[]},{"type":"reasoning","id":"rs_also_missing","summary":[],"content":[]},{"role":"user","content":"continue"}]}`)
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
			require.NotContains(t, string(retryBody), "rs_also_missing")
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
	require.Equal(t, service.ResponsesMissingReasoningItemRule.ID, events[0].RuleID)
	require.Equal(t, service.ResponsesMissingReasoningItemRule.Key, events[0].Key)
	require.Equal(t, service.ResponsesMissingReasoningItemRule.SettingKey, events[0].SettingKey)
	require.Equal(t, service.RelayCompatibilityEventTypeApplied, events[0].EventType)
	require.Equal(t, "accepted", events[0].Outcome)
	require.Equal(t, "rs_missing", events[0].ItemID)
	require.Equal(t, 2, events[0].Count)
	require.Equal(t, "remove_all_empty_reasoning_references_and_retry_same_channel", events[0].Action)

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

	adminInfo := map[string]interface{}{}
	service.AppendRelayCompatibilityAdminInfo(ctx, adminInfo)
	events, ok := adminInfo["compatibility_events"].([]service.RelayCompatibilityEvent)
	require.True(t, ok)
	require.Len(t, events, 1)
	require.Equal(t, service.ResponsesMissingReasoningItemRule.ID, events[0].RuleID)
	require.Equal(t, service.RelayCompatibilityEventTypeRecommendation, events[0].EventType)
	require.Equal(t, "disabled", events[0].Outcome)
	require.False(t, events[0].Retried)

	_, _, retried = retryMissingResponsesReasoningItem(
		ctx,
		&relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{ChannelId: 18}},
		storage,
		upstreamError,
		nil,
	)
	require.False(t, retried)
	adminInfo = map[string]interface{}{}
	service.AppendRelayCompatibilityAdminInfo(ctx, adminInfo)
	events, ok = adminInfo["compatibility_events"].([]service.RelayCompatibilityEvent)
	require.True(t, ok)
	require.Len(t, events, 1)
}
