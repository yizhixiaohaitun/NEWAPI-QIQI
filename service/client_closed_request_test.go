package service

import (
	"context"
	"testing"

	"github.com/QuantumNous/new-api/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewClientClosedRequestErrorReturnsHTTP499AndSkipsRetry(t *testing.T) {
	err := NewClientClosedRequestError(context.Canceled)
	require.NotNil(t, err)
	assert.Equal(t, StatusClientClosedRequest, err.StatusCode)
	assert.Equal(t, types.ErrorCodeClientClosedRequest, err.GetErrorCode())
	assert.True(t, types.IsSkipRetryError(err))
	assert.ErrorIs(t, err, context.Canceled)
}
