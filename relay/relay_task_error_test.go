package relay

import (
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/service"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSummarizeTaskUpstreamErrorHidesSPAHTML(t *testing.T) {
	summary := summarizeTaskUpstreamError(
		"text/html; charset=utf-8",
		[]byte(`<!doctype html><html><body><script>large frontend bundle</script></body></html>`),
	)

	assert.Equal(t, "HTML page (possible wrong API path or SPA fallback)", summary)
	assert.NotContains(t, summary, "frontend bundle")
}

func TestTaskUpstreamModelPriceErrorKeepsCodeAndAddsAttribution(t *testing.T) {
	resp := &http.Response{
		StatusCode: http.StatusInternalServerError,
		Header:     http.Header{"X-Oneapi-Request-Id": []string{"upstream-request-123"}},
		Body:       io.NopCloser(strings.NewReader(`{"error":{"message":"model price not configured","type":"new_api_error","code":"model_price_error"}}`)),
	}
	err := taskUpstreamError(resp, []byte(`{"error":{"message":"model price not configured","type":"new_api_error","code":"model_price_error"}}`))
	require.NotNil(t, err)
	assert.Equal(t, "model_price_error", err.Code)
	assert.Equal(t, http.StatusBadGateway, err.StatusCode)
	assert.Contains(t, err.Message, "上游渠道返回")
	assert.Contains(t, err.Message, "价格配置错误/缓存未同步")
	assert.Contains(t, err.Message, "HTTP 500")
	assert.Contains(t, err.Message, "model price not configured")
	assert.Contains(t, err.Message, "upstream-request-123")
	assert.NotContains(t, err.Message, "Bearer")
}

func TestTaskUpstreamTopLevelModelPriceErrorKeepsCodeBodyRequestIDAndAttribution(t *testing.T) {
	resp := &http.Response{StatusCode: http.StatusInternalServerError, Header: make(http.Header)}
	err := taskUpstreamError(resp, []byte(`{"code":"model_price_error","message":"模型倍率或价格未配置","request_id":"body-request-789"}`))
	require.NotNil(t, err)
	assert.Equal(t, "model_price_error", err.Code)
	assert.Equal(t, http.StatusBadGateway, err.StatusCode)
	assert.Contains(t, err.Message, "上游渠道返回价格配置错误/缓存未同步")
	assert.Contains(t, err.Message, "模型倍率或价格未配置")
	assert.Contains(t, err.Message, "body-request-789")
}

func TestTaskUpstreamValidationErrorKeeps422CodeAndMessage(t *testing.T) {
	resp := &http.Response{
		StatusCode: http.StatusUnprocessableEntity,
		Header:     make(http.Header),
	}
	err := taskUpstreamError(resp, []byte(`{"error":{"message":"Extra inputs are not permitted: aspect_ratio","type":"extra_forbidden","code":"extra_forbidden"}}`))
	require.NotNil(t, err)
	assert.Equal(t, "extra_forbidden", err.Code)
	assert.Equal(t, http.StatusUnprocessableEntity, err.StatusCode)
	assert.Contains(t, err.Message, "上游渠道返回错误（HTTP 422）")
	assert.Contains(t, err.Message, "Extra inputs are not permitted: aspect_ratio")
}

func TestTaskUpstreamRateLimitKeeps429CodeAndMessage(t *testing.T) {
	resp := &http.Response{
		StatusCode: http.StatusTooManyRequests,
		Header:     make(http.Header),
	}
	err := taskUpstreamError(resp, []byte(`{"error":{"message":"provider quota reached","type":"rate_limit_error","code":"rate_limit_exceeded"}}`))
	require.NotNil(t, err)
	assert.Equal(t, "rate_limit_exceeded", err.Code)
	assert.Equal(t, http.StatusTooManyRequests, err.StatusCode)
	assert.Contains(t, err.Message, "上游渠道返回错误（HTTP 429）")
	assert.Contains(t, err.Message, "provider quota reached")
}

func TestTaskUpstreamErrorSanitizesBearerToken(t *testing.T) {
	resp := &http.Response{StatusCode: http.StatusBadRequest, Header: make(http.Header)}
	err := taskUpstreamError(resp, []byte(`{"error":{"message":"authorization failed for Bearer sk-secret-value","code":"invalid_key"}}`))
	require.NotNil(t, err)
	assert.Contains(t, err.Message, "Bearer ***")
	assert.NotContains(t, err.Message, "sk-secret-value")
}

func TestTaskUpstreamErrorSanitizesBareKey(t *testing.T) {
	resp := &http.Response{StatusCode: http.StatusBadRequest, Header: make(http.Header)}
	err := taskUpstreamError(resp, []byte(`{"error":{"message":"invalid key sk-1234567890abcdef","code":"invalid_key"}}`))
	require.NotNil(t, err)
	assert.Contains(t, err.Message, "sk-***")
	assert.NotContains(t, err.Message, "1234567890abcdef")
}

func TestTaskUpstreamErrorPreservesGenericCodeAndRequestID(t *testing.T) {
	resp := &http.Response{
		StatusCode: http.StatusServiceUnavailable,
		Header: http.Header{
			"Content-Type":          []string{"application/json"},
			"X-Upstream-Request-Id": []string{"provider-request-456"},
		},
	}
	err := taskUpstreamError(resp, []byte(`{"error":{"message":"temporarily unavailable","code":"provider_busy"}}`))
	require.NotNil(t, err)
	assert.Equal(t, "provider_busy", err.Code)
	assert.Equal(t, http.StatusBadGateway, err.StatusCode)
	assert.Contains(t, err.Message, "上游渠道返回错误（HTTP 503）")
	assert.Contains(t, err.Message, "provider-request-456")
}

func TestLocalModelPriceErrorRemainsLocal(t *testing.T) {
	err := service.TaskErrorWrapperLocal(errors.New("模型倍率或价格未配置"), "model_price_error", http.StatusBadRequest)
	assert.True(t, err.LocalError)
	assert.Equal(t, "模型倍率或价格未配置", err.Message)
	assert.NotContains(t, err.Message, "上游")
}

func TestSummarizeTaskUpstreamErrorTruncatesLargeBody(t *testing.T) {
	summary := summarizeTaskUpstreamError("application/json", []byte(strings.Repeat("x", 300)))

	assert.Len(t, summary, 243)
	assert.True(t, strings.HasSuffix(summary, "..."))
}
