package service

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/QuantumNous/new-api/types"
)

// NewUpstreamRequestContext creates the context used for one complete upstream
// attempt. Its lifetime covers both client.Do and response body reads.
func NewUpstreamRequestContext(parent context.Context, timeoutSeconds int) (context.Context, context.CancelFunc) {
	if timeoutSeconds == 0 {
		return context.WithCancel(parent)
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
