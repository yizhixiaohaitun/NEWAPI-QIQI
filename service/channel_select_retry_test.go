package service

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRetryParamIncreaseRetryStopsAtIntegerBoundary(t *testing.T) {
	maxInt := int(^uint(0) >> 1)
	param := &RetryParam{Retry: &maxInt}

	assert.False(t, param.IncreaseRetry())
	assert.Equal(t, maxInt, param.GetRetry())
}

func TestRetryParamIncreaseRetryPreservesCrossGroupReset(t *testing.T) {
	retry := 0
	param := &RetryParam{Retry: &retry}
	param.ResetRetryNextTry()

	require.True(t, param.IncreaseRetry())
	assert.Zero(t, param.GetRetry())
	require.True(t, param.IncreaseRetry())
	assert.Equal(t, 1, param.GetRetry())
}
