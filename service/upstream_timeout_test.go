package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
