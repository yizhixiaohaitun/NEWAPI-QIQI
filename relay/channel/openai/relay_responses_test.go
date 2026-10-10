package openai

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newResponsesStreamTestContext(t *testing.T, body string) (*gin.Context, *httptest.ResponseRecorder, *http.Response, *relaycommon.RelayInfo) {
	t.Helper()
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	startTime := time.Now()
	info := &relaycommon.RelayInfo{
		ChannelMeta:       &relaycommon.ChannelMeta{},
		IsStream:          true,
		StartTime:         startTime,
		FirstResponseTime: startTime.Add(-time.Second),
		OriginModelName:   "gpt-5.5",
	}
	info.SetEstimatePromptTokens(10)
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(body)),
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
	}
	return c, recorder, resp, info
}

func enableResponsesStreamRetryForTest(t *testing.T) {
	t.Helper()
	setting := operation_setting.GetQiqiSetting()
	original := *setting
	setting.ResponsesStreamErrorRetryEnabled = true
	setting.ResponsesStreamErrorRetryTimes = operation_setting.DefaultResponsesStreamErrorRetryTimes
	t.Cleanup(func() { *setting = original })

	originalTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() { constant.StreamingTimeout = originalTimeout })
}

func TestOaiResponsesStreamHandlerReturnsEarlyRateLimitForRetry(t *testing.T) {
	enableResponsesStreamRetryForTest(t)
	body := strings.Join([]string{
		`event: response.created`,
		`data: {"type":"response.created","response":{"id":"resp_1","status":"in_progress"}}`,
		``,
		`event: error`,
		`data: {"type":"error","error":{"type":"too_many_requests","code":"rate_limit_exceeded","message":"rate limited","param":null}}`,
		``,
		`event: response.failed`,
		`data: {"type":"response.failed","response":{"id":"resp_1","status":"failed"}}`,
		``,
	}, "\n")
	c, recorder, resp, info := newResponsesStreamTestContext(t, body)

	usage, newAPIError := OaiResponsesStreamHandler(c, info, resp)

	require.Nil(t, usage)
	require.NotNil(t, newAPIError)
	assert.Equal(t, http.StatusTooManyRequests, newAPIError.StatusCode)
	assert.Equal(t, "rate_limit_exceeded", newAPIError.ToOpenAIError().Code)
	assert.True(t, info.ResponsesStreamErrorBeforeCommit)
	assert.Empty(t, recorder.Body.String())
	assert.Empty(t, recorder.Header().Get("Content-Type"))
}

func TestOaiResponsesStreamHandlerCommitsSuccessfulStreamAndUsage(t *testing.T) {
	enableResponsesStreamRetryForTest(t)
	body := strings.Join([]string{
		`event: response.created`,
		`data: {"type":"response.created","response":{"id":"resp_1","status":"in_progress"}}`,
		``,
		`event: response.output_text.delta`,
		`data: {"type":"response.output_text.delta","delta":"hello"}`,
		``,
		`event: response.completed`,
		`data: {"type":"response.completed","response":{"id":"resp_1","status":"completed","usage":{"input_tokens":10,"output_tokens":2,"total_tokens":12,"input_tokens_details":{"cached_tokens":3}}}}`,
		``,
	}, "\n")
	c, recorder, resp, info := newResponsesStreamTestContext(t, body)

	usage, newAPIError := OaiResponsesStreamHandler(c, info, resp)

	require.Nil(t, newAPIError)
	require.NotNil(t, usage)
	assert.Equal(t, 10, usage.PromptTokens)
	assert.Equal(t, 2, usage.CompletionTokens)
	assert.Equal(t, 12, usage.TotalTokens)
	assert.Equal(t, 3, usage.PromptTokensDetails.CachedTokens)
	assert.False(t, info.ResponsesStreamErrorBeforeCommit)
	assert.Equal(t, "text/event-stream", recorder.Header().Get("Content-Type"))
	output := recorder.Body.String()
	assert.Less(t, strings.Index(output, "event: response.created"), strings.Index(output, "event: response.output_text.delta"))
	assert.Contains(t, output, `"delta":"hello"`)
}

func TestOaiResponsesStreamHandlerPublishesDeductedUsageAliasesOnce(t *testing.T) {
	enableResponsesStreamRetryForTest(t)
	body := strings.Join([]string{
		`event: response.output_text.delta`,
		`data: {"type":"response.output_text.delta","delta":""}`,
		``,
		`event: response.completed`,
		`data: {"type":"response.completed","response":{"id":"resp_cache","status":"completed","choices":[{"index":0,"usage":{"cached_tokens":450}}],"timings":{"cache_n":450},"usage":{"cached_tokens":450,"input_tokens":506,"output_tokens":0,"total_tokens":506,"prompt_cache_hit_tokens":450,"input_tokens_details":{"cached_tokens":450,"cached_creation_tokens":0,"cache_write_tokens":0,"text_tokens":506}}}}`,
		``,
	}, "\n")
	c, recorder, resp, info := newResponsesStreamTestContext(t, body)
	info.UsingGroup = "default"
	info.ChannelMeta.ChannelSetting = dto.ChannelSettings{InputTokenDeduction: 490}

	usage, newAPIError := OaiResponsesStreamHandler(c, info, resp)

	require.Nil(t, newAPIError)
	require.NotNil(t, usage)
	require.Equal(t, 506, usage.PromptTokens, "settlement keeps the original usage")
	require.Equal(t, 450, usage.PromptTokensDetails.CachedTokens)
	output := recorder.Body.String()
	assertDeductedProviderAliasesInSSE(t, output, true)
	require.Contains(t, output, `"input_tokens":16`)
	require.Contains(t, output, `"prompt_cache_hit_tokens":16`)
	require.Contains(t, output, `"cached_tokens":16`)
	require.Contains(t, output, `"text_tokens":16`)
	require.NotContains(t, output, `"input_tokens":506`)
}

func TestOaiResponsesStreamHandlerDoesNotRetryAfterOutputCommit(t *testing.T) {
	enableResponsesStreamRetryForTest(t)
	body := strings.Join([]string{
		`event: response.created`,
		`data: {"type":"response.created","response":{"id":"resp_1","status":"in_progress"}}`,
		``,
		`event: response.output_text.delta`,
		`data: {"type":"response.output_text.delta","delta":"partial"}`,
		``,
		`event: error`,
		`data: {"type":"error","error":{"type":"server_error","code":"server_error","message":"failed after output"}}`,
		``,
	}, "\n")
	c, recorder, resp, info := newResponsesStreamTestContext(t, body)

	usage, newAPIError := OaiResponsesStreamHandler(c, info, resp)

	require.Nil(t, newAPIError)
	require.NotNil(t, usage)
	assert.Positive(t, usage.TotalTokens)
	assert.False(t, info.ResponsesStreamErrorBeforeCommit)
	assert.Contains(t, recorder.Body.String(), "partial")
	assert.Contains(t, recorder.Body.String(), "failed after output")
}

func TestOaiResponsesStreamHandlerRetriesPrematureEOFBeforeOutput(t *testing.T) {
	enableResponsesStreamRetryForTest(t)
	body := strings.Join([]string{
		`event: response.created`,
		`data: {"type":"response.created","response":{"id":"resp_1","status":"in_progress"}}`,
		``,
		`event: response.in_progress`,
		`data: {"type":"response.in_progress","response":{"id":"resp_1","status":"in_progress"}}`,
		``,
	}, "\n")
	c, recorder, resp, info := newResponsesStreamTestContext(t, body)

	usage, newAPIError := OaiResponsesStreamHandler(c, info, resp)

	require.Nil(t, usage)
	require.NotNil(t, newAPIError)
	assert.Equal(t, http.StatusBadGateway, newAPIError.StatusCode)
	assert.True(t, info.ResponsesStreamErrorBeforeCommit)
	assert.Empty(t, recorder.Body.String())
}
