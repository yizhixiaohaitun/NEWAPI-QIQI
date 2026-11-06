package controller

import (
	"context"
	"errors"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestFirstByteTimeoutRetryStops(t *testing.T) {
	for _, scenario := range []string{"ready", "budget", "specific", "cancelled", "written", "affinity", "skip", "token_override"} {
		t.Run(scenario, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
			c.Set("upstream_first_byte_timeout", true)
			err := types.NewErrorWithStatusCode(errors.New("first byte timeout"), types.ErrorCodeFirstByteTimeout, http.StatusGatewayTimeout)
			budget := 1
			switch scenario {
			case "budget":
				budget = 0
			case "specific":
				c.Set("specific_channel_id", 1)
			case "cancelled":
				ctx, cancel := context.WithCancel(c.Request.Context())
				cancel()
				c.Request = c.Request.WithContext(ctx)
			case "written":
				_, _ = c.Writer.WriteString("partial")
			case "affinity":
				c.Set("channel_affinity_skip_retry_on_failure", true)
			case "skip":
				err = types.NewErrorWithStatusCode(errors.New("timeout"), types.ErrorCodeFirstByteTimeout, http.StatusGatewayTimeout, types.ErrOptionWithSkipRetry())
			case "token_override":
				timeout := 5
				common.SetContextKey(c, constant.ContextKeyTokenFirstByteTimeout, &timeout)
			}
			assert.Equal(t, scenario == "ready", shouldRetry(c, err, budget))
		})
	}
}
