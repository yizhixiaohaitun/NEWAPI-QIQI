package controller

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestQiqiEC003ResponsesResourceMismatchDoesNotRetry(t *testing.T) {
	gin.SetMode(gin.TestMode)
	affinitySetting := operation_setting.GetChannelAffinitySetting()
	originalRules := affinitySetting.Rules
	rule := operation_setting.ChannelAffinityRule{
		Name:              "qiqi-ec-003-controller-test",
		ModelRegex:        []string{"^gpt-5$"},
		PathRegex:         []string{"/v1/responses"},
		KeySources:        []operation_setting.ChannelAffinityKeySource{{Type: "responses_state"}},
		IncludeRuleName:   true,
		IncludeUsingGroup: true,
	}
	affinitySetting.Rules = []operation_setting.ChannelAffinityRule{rule}
	qiqiSetting := operation_setting.GetQiqiSetting()
	originalQiqi := *qiqiSetting
	qiqiSetting.AzureResponsesResourceAffinityEnabled = true
	t.Cleanup(func() {
		affinitySetting.Rules = originalRules
		*qiqiSetting = originalQiqi
	})

	stateID := "resp_controller_mismatch"
	recordCtx, _ := gin.CreateTestContext(httptest.NewRecorder())
	recordCtx.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"input":"hi"}`))
	service.RecordResponsesStateChannelAffinity(recordCtx, 3010, "gpt-5", "default", []string{stateID})

	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(fmt.Sprintf(`{"previous_response_id":%q}`, stateID)))
	channelID, found := service.GetPreferredChannelByAffinity(ctx, "gpt-5", "default")
	require.True(t, found)
	require.Equal(t, 3010, channelID)
	t.Cleanup(func() { service.ClearCurrentChannelAffinityCache(ctx) })

	mismatch := types.NewErrorWithStatusCode(
		fmt.Errorf("requested response was created under a different Azure OpenAI resource"),
		types.ErrorCodeInvalidRequest,
		http.StatusBadRequest,
	)
	assert.False(t, shouldRetry(ctx, mismatch, 2))
}

func TestRetryLimitForEarlyResponsesStreamError(t *testing.T) {
	setting := operation_setting.GetQiqiSetting()
	original := *setting
	t.Cleanup(func() { *setting = original })

	setting.ResponsesStreamErrorRetryEnabled = true
	setting.ResponsesStreamErrorRetryTimes = 2
	info := &relaycommon.RelayInfo{ResponsesStreamErrorBeforeCommit: true}
	assert.Equal(t, 2, retryLimitForRelayError(info, 0))

	info.ResponsesStreamErrorBeforeCommit = false
	assert.Equal(t, 4, retryLimitForRelayError(info, 4))

	info.ResponsesStreamErrorBeforeCommit = true
	setting.ResponsesStreamErrorRetryEnabled = false
	assert.Equal(t, 4, retryLimitForRelayError(info, 4))
}
