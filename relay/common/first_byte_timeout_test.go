package common

import (
	"context"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFirstByteWatch(t *testing.T) {
	t.Run("body byte stops timer and records time", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		watch := NewFirstByteWatch(cancel, time.Second)
		start := time.Now()
		body := watch.Wrap(io.NopCloser(strings.NewReader(": heartbeat\n")))
		data, err := io.ReadAll(body)
		require.NoError(t, err)
		assert.Equal(t, ": heartbeat\n", string(data))
		assert.False(t, watch.Finish())
		assert.False(t, watch.FirstByteTime().Before(start))
		assert.NoError(t, ctx.Err())
	})
	t.Run("stalled attempt is cancelled", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		watch := NewFirstByteWatch(cancel, 10*time.Millisecond)
		select {
		case <-ctx.Done():
		case <-time.After(time.Second):
			t.Fatal("attempt not cancelled")
		}
		assert.True(t, watch.Finish())
		assert.True(t, watch.FirstByteTime().IsZero())
	})
	t.Run("finishing prevents late cancellation", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		watch := NewFirstByteWatch(cancel, time.Second)
		assert.False(t, watch.Finish())
		assert.NoError(t, ctx.Err())
	})
}
