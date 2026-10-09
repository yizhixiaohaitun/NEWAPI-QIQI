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

func TestRetryParamIncreaseRetryCountsEveryControllerAttempt(t *testing.T) {
	retry := 0
	param := &RetryParam{Retry: &retry}

	require.True(t, param.IncreaseRetry())
	assert.Equal(t, 1, param.GetRetry())
	require.True(t, param.IncreaseRetry())
	assert.Equal(t, 2, param.GetRetry())
}
