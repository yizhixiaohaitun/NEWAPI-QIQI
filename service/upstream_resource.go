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
	resource := strings.Contains(text, "额度不足") || strings.Contains(text, "余额不足") ||
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
	return newUpstreamResourceInsufficientError(http.StatusInternalServerError)
}

func newUpstreamResourceInsufficientError(status int) *types.NewAPIError {
	return types.NewErrorWithStatusCode(
		errors.New(upstreamResourceInsufficientMessage),
		types.ErrorCodeUpstreamResourceInsufficient,
		status,
	)
}

// SanitizeFinalRelayError is intentionally called only after channel retries
// and internal logging have finished. Thus retry decisions retain the original
// provider status/body while the terminal client response cannot leak balance
// or request-id details.
func SanitizeFinalRelayError(err *types.NewAPIError) *types.NewAPIError {
	if err == nil || err.GetErrorCode() == types.ErrorCodeInsufficientUserQuota {
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
