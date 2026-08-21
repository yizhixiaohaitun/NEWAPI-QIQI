package service

import (
	"errors"
	"net/http"
	"regexp"
	"strings"

	"github.com/QuantumNous/new-api/types"
)

const upstreamResourceInsufficientMessage = "上游资源不足，请稍后重试"

var embedded429Pattern = regexp.MustCompile(`(?i)(status[_ ]?code|http status|status)\s*[:=]?\s*429`)

// IsUpstreamResourceInsufficient recognizes quota failures returned by an
// upstream provider. Callers must not use it for locally generated errors.
func IsUpstreamResourceInsufficient(status int, text string) bool {
	lower := strings.ToLower(text)
	chinesePreConsumeFailure := strings.Contains(text, "预扣费额度失败") &&
		strings.Contains(text, "用户剩余额度") && strings.Contains(text, "需要预扣费额度")
	resource := chinesePreConsumeFailure || strings.Contains(text, "额度不足") || strings.Contains(text, "余额不足") ||
		strings.Contains(lower, "insufficient quota") || strings.Contains(lower, "quota exceeded") ||
		strings.Contains(lower, "quota exhausted") || strings.Contains(lower, "insufficient balance") ||
		strings.Contains(lower, "credit exhausted") || strings.Contains(lower, "insufficient credit") ||
		strings.Contains(lower, "not enough credit")
	if !resource {
		return false
	}
	return status == http.StatusTooManyRequests || embedded429Pattern.MatchString(text) ||
		strings.Contains(lower, "quota") || strings.Contains(lower, "balance") ||
		strings.Contains(lower, "credit") || strings.Contains(text, "额度") || strings.Contains(text, "余额")
}

func NewUpstreamResourceInsufficientError() *types.NewAPIError {
	return types.NewErrorWithStatusCode(
		errors.New(upstreamResourceInsufficientMessage),
		types.ErrorCodeUpstreamResourceInsufficient,
		http.StatusTooManyRequests,
	)
}

// newRawUpstreamResourceInsufficientError marks a provider response without
// destroying it. Retry policy, channel health and internal logs still receive
// the provider status/message; SanitizeFinalRelayError replaces it only at the
// terminal client boundary.
func newRawUpstreamResourceInsufficientError(status int, raw string) *types.NewAPIError {
	return types.NewOpenAIError(
		errors.New(raw),
		types.ErrorCodeUpstreamResourceInsufficient,
		status,
	)
}

// SanitizeFinalRelayError is intentionally called only after channel retries
// and internal logging have finished. Thus retry decisions retain the original
// provider status/body while the terminal client response cannot leak balance
// or request-id details.
func SanitizeFinalRelayError(err *types.NewAPIError) *types.NewAPIError {
	if err == nil {
		return err
	}
	// These errors originate from this instance's own billing path, not from a
	// provider. Keep their existing client contract unchanged.
	if err.GetErrorCode() == types.ErrorCodeInsufficientUserQuota ||
		err.GetErrorCode() == types.ErrorCodePreConsumeTokenQuotaFailed {
		return err
	}
	if err.GetErrorCode() == types.ErrorCodeUpstreamResourceInsufficient {
		return NewUpstreamResourceInsufficientError()
	}
	if !IsUpstreamResourceInsufficient(err.StatusCode, err.ErrorWithStatusCode()) {
		return err
	}
	return NewUpstreamResourceInsufficientError()
}

// PublicRelayErrorLogContent returns the error text safe to persist in the
// request log that is exposed through the API.
func PublicRelayErrorLogContent(err *types.NewAPIError) string {
	return SanitizeFinalRelayError(err).MaskSensitiveErrorWithStatusCode()
}
