package controller

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/dto"
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

func TestUpstreamPreConsumeFailureRetriesWhenForbidden(t *testing.T) {
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{}`))

	upstream := types.NewOpenAIError(
		fmt.Errorf("预扣费额度失败, 用户剩余额度: $0.001, 需要预扣费额度: $0.25"),
		types.ErrorCodeUpstreamResourceInsufficient,
		http.StatusForbidden,
	)
	assert.True(t, shouldRetry(ctx, upstream, 1))
	assert.False(t, shouldRetry(ctx, upstream, 0))
}

func TestClientClosedRequestBecomes499AndDoesNotRetry(t *testing.T) {
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	requestContext, cancel := context.WithCancel(context.Background())
	cancel()
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{}`)).WithContext(requestContext)

	relayErr := types.NewErrorWithStatusCode(
		fmt.Errorf("do request failed"),
		types.ErrorCodeDoRequestFailed,
		http.StatusInternalServerError,
	)
	normalized := normalizeRelayContextError(relayErr, requestContext.Err(), context.Canceled)

	require.NotNil(t, normalized)
	assert.Equal(t, service.StatusClientClosedRequest, normalized.StatusCode)
	assert.Equal(t, types.ErrorCodeClientClosedRequest, normalized.GetErrorCode())
	assert.False(t, shouldRetry(ctx, normalized, 3))
}

func TestUpstreamDeadlineRemains500AndDoesNotRetry(t *testing.T) {
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{}`))

	normalized := normalizeRelayContextError(nil, nil, context.DeadlineExceeded)

	require.NotNil(t, normalized)
	assert.Equal(t, http.StatusInternalServerError, normalized.StatusCode)
	assert.Equal(t, types.ErrorCodeUpstreamTimeout, normalized.GetErrorCode())
	assert.False(t, shouldRetry(ctx, normalized, 3))

	// Preserve the semantic even when another NEWAPI upstream returns this code
	// without our local skip-retry flag.
	upstreamReportedTimeout := types.NewErrorWithStatusCode(
		fmt.Errorf("upstream service timed out"),
		types.ErrorCodeUpstreamTimeout,
		http.StatusInternalServerError,
	)
	assert.False(t, shouldRetry(ctx, upstreamReportedTimeout, 3))
}

func TestRelayRequestDeadlineSpansAllChannelAttempts(t *testing.T) {
	for _, testCase := range []struct {
		name        string
		isStream    bool
		relayFormat types.RelayFormat
	}{
		{name: "non-stream", relayFormat: types.RelayFormatOpenAI},
		{name: "stream", isStream: true, relayFormat: types.RelayFormatOpenAI},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			var attempts atomic.Int32
			upstreamCanceled := make(chan struct{}, 1)
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = io.Copy(io.Discard, r.Body)
				_ = r.Body.Close()
				attempt := attempts.Add(1)
				if attempt < 3 {
					time.Sleep(400 * time.Millisecond)
					w.WriteHeader(http.StatusInternalServerError)
					return
				}
				<-r.Context().Done()
				upstreamCanceled <- struct{}{}
			}))
			defer upstream.Close()
			client := &http.Client{Transport: &http.Transport{DisableKeepAlives: true}}
			defer client.CloseIdleConnections()

			nonStreamTimeout := 1
			info := &relaycommon.RelayInfo{
				IsStream:                 testCase.isStream,
				UpstreamTimeout:          1,
				NonStreamUpstreamTimeout: &nonStreamTimeout,
			}
			requestUpstreamCtx, cancelRequestUpstream := newRelayRequestUpstreamContext(context.Background(), info, testCase.relayFormat)
			defer cancelRequestUpstream()

			startedAt := time.Now()
			var relayErr *types.NewAPIError
			for attempt := 0; attempt < 3; attempt++ {
				attemptCtx, cancelAttempt := context.WithCancel(requestUpstreamCtx)
				req, err := http.NewRequestWithContext(attemptCtx, http.MethodPost, upstream.URL, strings.NewReader(`{}`))
				require.NoError(t, err)
				resp, err := client.Do(req)
				cancelAttempt()
				if err != nil {
					relayErr = types.NewError(err, types.ErrorCodeDoRequestFailed)
					break
				}
				resp.Body.Close()
				relayErr = types.NewErrorWithStatusCode(
					fmt.Errorf("retryable upstream response"),
					types.ErrorCodeBadResponseStatusCode,
					http.StatusInternalServerError,
				)
			}

			elapsed := time.Since(startedAt)
			normalized := normalizeRelayContextError(relayErr, nil, requestUpstreamCtx.Err())
			require.NotNil(t, normalized)
			assert.Equal(t, types.ErrorCodeUpstreamTimeout, normalized.GetErrorCode())
			assert.False(t, shouldRetry(nil, normalized, 3))
			assert.Equal(t, int32(3), attempts.Load())
			assert.Less(t, elapsed, 1300*time.Millisecond, "the one-second token timeout must not reset per channel")
			select {
			case <-upstreamCanceled:
			case <-time.After(time.Second):
				t.Fatal("active upstream request context was not canceled at the total deadline")
			}
		})
	}
}

func TestRetryableNonTimeoutChannelFailureStillRetries(t *testing.T) {
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{}`))
	retryable := types.NewErrorWithStatusCode(
		fmt.Errorf("temporary upstream failure"),
		types.ErrorCodeBadResponseStatusCode,
		http.StatusInternalServerError,
	)
	assert.True(t, shouldRetry(ctx, retryable, 1))

	taskRetryable := &dto.TaskError{Code: "temporary_failure", StatusCode: http.StatusInternalServerError}
	assert.True(t, shouldRetryTaskRelay(ctx, 1, taskRetryable, 1))
	taskTimeout := &dto.TaskError{Code: string(types.ErrorCodeUpstreamTimeout), StatusCode: http.StatusInternalServerError}
	assert.False(t, shouldRetryTaskRelay(ctx, 1, taskTimeout, 3))
}

func TestRespondTaskErrorPreservesUserQuota429Message(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	taskErr := &dto.TaskError{
		Code:       string(types.ErrorCodeInsufficientUserQuota),
		Message:    "用户额度不足，请充值后重试",
		StatusCode: http.StatusTooManyRequests,
	}

	respondTaskError(ctx, taskErr)

	assert.Equal(t, http.StatusTooManyRequests, recorder.Code)
	assert.Contains(t, recorder.Body.String(), "用户额度不足，请充值后重试")
	assert.NotContains(t, recorder.Body.String(), "当前分组上游负载已饱和")
}

func TestRespondTaskErrorRewritesGenericUpstream429Message(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	taskErr := &dto.TaskError{
		Code:       "rate_limit_exceeded",
		Message:    "raw upstream rate limit message",
		StatusCode: http.StatusTooManyRequests,
	}

	respondTaskError(ctx, taskErr)

	assert.Equal(t, http.StatusTooManyRequests, recorder.Code)
	assert.Contains(t, recorder.Body.String(), "当前分组上游负载已饱和，请稍后再试")
	assert.NotContains(t, recorder.Body.String(), "raw upstream rate limit message")
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
