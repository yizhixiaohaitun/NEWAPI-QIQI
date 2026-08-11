package service

import (
	"errors"
	"net/http"
	"testing"

	"github.com/QuantumNous/new-api/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsUpstreamResourceInsufficient(t *testing.T) {
	tests := []struct {
		name   string
		status int
		text   string
		want   bool
	}{
		{"sample", 403, "status_code=429, 用户额度不足, 剩余额度: ＄-58.829585 (request id: secret)", true},
		{"chinese pre-consume failure", 403, "status_code=403, 预扣费额度失败, 用户剩余额度: $0.226296, 需要预扣费额度: $0.576756 (request id: upstream-example)", true},
		{"incomplete chinese pre-consume failure", 403, "预扣费额度失败, 用户剩余额度: $0.226296", false},
		{"quota", 429, "insufficient quota", true},
		{"balance", 403, "Insufficient balance", true},
		{"credit", 403, "credit exhausted", true},
		{"non quota", 403, "invalid API key or permission denied", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, IsUpstreamResourceInsufficient(tt.status, tt.text))
		})
	}
}

func TestSanitizeFinalRelayError(t *testing.T) {
	raw := "status_code=403, 预扣费额度失败, 用户剩余额度: $0.226296, 需要预扣费额度: $0.576756 (request id: upstream-example)"
	upstream := types.NewOpenAIError(errors.New(raw), types.ErrorCodeBadResponseStatusCode, http.StatusForbidden)
	sanitized := SanitizeFinalRelayError(upstream)
	require.NotSame(t, upstream, sanitized)
	assert.Equal(t, http.StatusInternalServerError, sanitized.StatusCode)
	assert.Equal(t, types.ErrorCodeUpstreamResourceInsufficient, sanitized.GetErrorCode())
	assert.Equal(t, upstreamResourceInsufficientMessage, sanitized.ToOpenAIError().Message)
	assert.Equal(t, upstreamResourceInsufficientMessage, sanitized.ToClaudeError().Message)
	assert.Equal(t, "status_code=500, "+upstreamResourceInsufficientMessage, PublicRelayErrorLogContent(upstream))
	assert.NotContains(t, sanitized.Error(), "0.226296")
	assert.NotContains(t, sanitized.Error(), "0.576756")
	assert.NotContains(t, sanitized.Error(), "upstream-example")

	markedRaw := newRawUpstreamResourceInsufficientError(http.StatusForbidden, raw)
	assert.Equal(t, http.StatusForbidden, markedRaw.StatusCode)
	assert.Contains(t, markedRaw.Error(), "0.226296")
	assert.Contains(t, markedRaw.Error(), "0.576756")
	assert.Contains(t, markedRaw.Error(), "upstream-example")
	markedSanitized := SanitizeFinalRelayError(markedRaw)
	assert.Equal(t, http.StatusInternalServerError, markedSanitized.StatusCode)
	assert.Equal(t, upstreamResourceInsufficientMessage, markedSanitized.Error())

	// Retry and channel-health code continues to receive the untouched error.
	assert.Equal(t, http.StatusForbidden, markedRaw.StatusCode)
	assert.Equal(t, types.ErrorCodeUpstreamResourceInsufficient, markedRaw.GetErrorCode())
	assert.Contains(t, markedRaw.Error(), "upstream-example")

	nonQuota := types.NewOpenAIError(errors.New("invalid API key"), types.ErrorCodeBadResponseStatusCode, http.StatusForbidden)
	assert.Same(t, nonQuota, SanitizeFinalRelayError(nonQuota))

	localQuota := types.NewErrorWithStatusCode(
		errors.New("用户额度不足, 剩余额度: $1"),
		types.ErrorCodeInsufficientUserQuota,
		http.StatusForbidden,
	)
	assert.Same(t, localQuota, SanitizeFinalRelayError(localQuota))
}
