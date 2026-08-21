package service

import (
	"context"

	"github.com/QuantumNous/new-api/types"
)

// StatusClientClosedRequest is the de facto status code used by gateways when
// the downstream client closes or cancels the request before a response is sent.
const StatusClientClosedRequest = 499

func NewClientClosedRequestError(err error) *types.NewAPIError {
	if err == nil {
		err = context.Canceled
	}
	return types.NewErrorWithStatusCode(
		err,
		types.ErrorCodeClientClosedRequest,
		StatusClientClosedRequest,
		types.ErrOptionWithSkipRetry(),
	)
}
