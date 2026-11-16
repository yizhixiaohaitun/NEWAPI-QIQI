package service

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"
)

// AdaptiveRoutingOutcome is deliberately tri-state: uncertain or local outcomes
// must not change channel health.
type AdaptiveRoutingOutcome int

const (
	AdaptiveRoutingIgnore AdaptiveRoutingOutcome = iota
	AdaptiveRoutingSuccess
	AdaptiveRoutingFailure
)

func adaptiveRoutingRefusal(s string) bool {
	s = strings.ToLower(s)
	for _, term := range []string{"context_length", "context length", "context window", "maximum context", "prompt_blocked", "content_filter", "content filter", "content_policy", "safety", "moderation", "violat", "flagged", "refusal", "invalid_request", "invalid request"} {
		if strings.Contains(s, term) {
			return true
		}
	}
	return false
}

// AdaptiveRelayOutcome classifies one completed attempt, not the final request.
// Context errors refer to the client and request-wide budget before cancellation.
func AdaptiveRelayOutcome(err *types.NewAPIError, stream *relaycommon.StreamStatus, clientErr, budgetErr error, firstByteTimeout bool) AdaptiveRoutingOutcome {
	if clientErr != nil || budgetErr != nil {
		return AdaptiveRoutingIgnore
	}
	if firstByteTimeout {
		return AdaptiveRoutingFailure
	}
	if err != nil {
		status := err.StatusCode
		if err.OriginalStatusCode != 0 {
			status = err.OriginalStatusCode
		}
		code := err.GetErrorCode()
		switch code {
		case types.ErrorCodeClientClosedRequest, types.ErrorCodeUpstreamTimeout, types.ErrorCodePromptBlocked, types.ErrorCodeInvalidRequest, types.ErrorCodeSensitiveWordsDetected, types.ErrorCodeInsufficientUserQuota, types.ErrorCodePreConsumeTokenQuotaFailed, types.ErrorCodeGetChannelFailed:
			return AdaptiveRoutingIgnore
		}
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || adaptiveRoutingRefusal(err.Error()+" "+string(code)) {
			return AdaptiveRoutingIgnore
		}
		if code == types.ErrorCodeFirstByteTimeout || code == types.ErrorCodeReadResponseBodyFailed {
			return AdaptiveRoutingFailure
		}
		if code == types.ErrorCodeDoRequestFailed {
			// The controller additionally requires an actual transport dispatch;
			// this branch remains conservative for direct classifier callers.
			return AdaptiveRoutingFailure
		}
		if err.GetErrorType() != types.ErrorTypeNewAPIError && (status == http.StatusTooManyRequests || status >= 500 && status <= 599) {
			return AdaptiveRoutingFailure
		}
		if code == types.ErrorCodeBadResponseStatusCode && (status == http.StatusTooManyRequests || status >= 500 && status <= 599) {
			return AdaptiveRoutingFailure
		}
		return AdaptiveRoutingIgnore
	}
	if stream != nil {
		providerErrors := stream.UpstreamErrors()
		if len(providerErrors) > 0 {
			for _, providerErr := range providerErrors {
				if AdaptiveRelayOutcome(providerErr, nil, nil, nil, false) == AdaptiveRoutingFailure {
					return AdaptiveRoutingFailure
				}
			}
			return AdaptiveRoutingIgnore
		}
		switch stream.EndReason {
		case relaycommon.StreamEndReasonScannerErr:
			if errors.Is(stream.EndError, context.Canceled) || errors.Is(stream.EndError, context.DeadlineExceeded) {
				return AdaptiveRoutingIgnore
			}
			return AdaptiveRoutingFailure
		case relaycommon.StreamEndReasonClientGone, relaycommon.StreamEndReasonPingFail, relaycommon.StreamEndReasonTimeout, relaycommon.StreamEndReasonPanic:
			return AdaptiveRoutingIgnore
		}
		if stream.EndError != nil || stream.HasErrors() {
			return AdaptiveRoutingIgnore
		}
	}
	return AdaptiveRoutingSuccess
}

// AdaptiveAssetOutcome classifies a completed synchronous asset fetch. Unlike
// task submission, a successful asset response is the actual upstream result.
func AdaptiveAssetOutcome(err *dto.TaskError, clientErr, budgetErr error) AdaptiveRoutingOutcome {
	if err == nil {
		if clientErr != nil || budgetErr != nil {
			return AdaptiveRoutingIgnore
		}
		return AdaptiveRoutingSuccess
	}
	if err.LocalError || clientErr != nil || budgetErr != nil {
		return AdaptiveRoutingIgnore
	}
	detail := err.Code + " " + err.Message
	if err.Error != nil {
		detail += " " + err.Error.Error()
	}
	if adaptiveRoutingRefusal(detail) || errors.Is(err.Error, context.Canceled) || errors.Is(err.Error, context.DeadlineExceeded) {
		return AdaptiveRoutingIgnore
	}
	if err.Code == "do_request_failed" || (err.Code == "fail_to_fetch_task" || err.Code == "bad_response_status_code") && (err.StatusCode == 429 || err.StatusCode >= 500 && err.StatusCode <= 599) {
		return AdaptiveRoutingFailure
	}
	return AdaptiveRoutingIgnore
}
