package service

import (
	"context"
	"io"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFirstByteTimingInConsumeLog(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)
	start := time.Now().Add(-time.Second)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	_ = ctx
	watch := relaycommon.NewFirstByteWatch(cancel, time.Second)
	body := watch.Wrap(io.NopCloser(strings.NewReader("data")))
	_, err := io.ReadAll(body)
	require.NoError(t, err)
	defer watch.Finish()
	info := &relaycommon.RelayInfo{StartTime: start, FirstResponseTime: start.Add(500 * time.Millisecond), FirstByteWatch: watch, ChannelMeta: &relaycommon.ChannelMeta{}}
	other := GenerateTextOtherInfo(c, info, 1, 1, 1, 0, 1, 0, 1)
	assert.Equal(t, float64(500), other["frt"])
	elapsed, ok := other["upstream_first_byte_ms"].(int64)
	require.True(t, ok)
	assert.GreaterOrEqual(t, elapsed, int64(1000))
}
