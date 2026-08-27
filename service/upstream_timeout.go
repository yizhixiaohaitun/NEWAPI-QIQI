package service

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/QuantumNous/new-api/types"
)

// TokenRequestTimeoutSeconds selects the token-level total-duration cutoff.
// Existing tokens with no dedicated non-stream value inherit upstreamTimeout,
// preserving their historical behavior after the nullable column is added.
func TokenRequestTimeoutSeconds(upstreamTimeout int, nonStreamTimeout *int, isStream bool) int {
	if isStream || nonStreamTimeout == nil {
		return upstreamTimeout
	}
	return *nonStreamTimeout
}

// NewUpstreamRequestContext creates the context used for one complete upstream
// attempt. Its lifetime covers both client.Do and response body reads.
func NewUpstreamRequestContext(parent context.Context, timeoutSeconds int) (context.Context, context.CancelFunc) {
	if timeoutSeconds <= 0 {
		return context.WithCancel(parent)
	}
	maxSeconds := int64(time.Duration(1<<63-1) / time.Second)
	if int64(timeoutSeconds) > maxSeconds {
		timeoutSeconds = int(maxSeconds)
	}
	return context.WithTimeout(parent, time.Duration(timeoutSeconds)*time.Second)
}

func NewUpstreamTimeoutError() *types.NewAPIError {
	return types.NewErrorWithStatusCode(
		errors.New("上游请求超时"),
		types.ErrorCodeUpstreamTimeout,
		http.StatusInternalServerError,
		types.ErrOptionWithSkipRetry(),
	)
}
