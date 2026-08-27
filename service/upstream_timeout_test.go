package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTokenRequestTimeoutSeconds(t *testing.T) {
	dedicated := 15
	disabled := 0

	assert.Equal(t, 45, TokenRequestTimeoutSeconds(45, &dedicated, true), "stream keeps the existing SK total-duration cutoff")
	assert.Equal(t, 15, TokenRequestTimeoutSeconds(45, &dedicated, false), "non-stream uses its dedicated cutoff")
	assert.Equal(t, 45, TokenRequestTimeoutSeconds(45, nil, false), "legacy rows inherit the historical cutoff")
	assert.Equal(t, 0, TokenRequestTimeoutSeconds(45, &disabled, false), "an explicit zero disables only the non-stream cutoff")
}

func TestNewUpstreamRequestContextDeadlineAndUnlimited(t *testing.T) {
	t.Run("positive timeout sets a deadline", func(t *testing.T) {
		started := time.Now()
		ctx, cancel := NewUpstreamRequestContext(context.Background(), 10)
		defer cancel()

		deadline, ok := ctx.Deadline()
		require.True(t, ok)
		assert.WithinDuration(t, started.Add(10*time.Second), deadline, time.Second)
	})

	t.Run("huge timeout does not overflow into an expired deadline", func(t *testing.T) {
		ctx, cancel := NewUpstreamRequestContext(context.Background(), int(^uint(0)>>1))
		defer cancel()

		deadline, ok := ctx.Deadline()
		require.True(t, ok)
		assert.True(t, deadline.After(time.Now()))
		assert.NoError(t, ctx.Err())
	})

	t.Run("zero has no deadline", func(t *testing.T) {
		ctx, cancel := NewUpstreamRequestContext(context.Background(), 0)
		_, hasDeadline := ctx.Deadline()
		assert.False(t, hasDeadline)

		cancel()
		require.ErrorIs(t, ctx.Err(), context.Canceled)
	})
}

func TestNewUpstreamRequestContextCancelsAtDeadline(t *testing.T) {
	ctx, cancel := NewUpstreamRequestContext(context.Background(), 1)
	defer cancel()

	select {
	case <-ctx.Done():
		require.ErrorIs(t, ctx.Err(), context.DeadlineExceeded)
	case <-time.After(2 * time.Second):
		t.Fatal("upstream context was not cancelled at its deadline")
	}
}

func TestNewUpstreamTimeoutErrorReturnsHTTP500(t *testing.T) {
	err := NewUpstreamTimeoutError()
	require.NotNil(t, err)
	assert.Equal(t, 500, err.StatusCode)
	assert.Equal(t, "upstream_timeout", string(err.GetErrorCode()))
}
