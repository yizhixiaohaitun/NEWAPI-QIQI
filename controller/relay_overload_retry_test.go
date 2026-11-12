package controller

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestProviderOverloadCodesRespectRetryStops(t *testing.T) {
	oldRanges, oldKeywords := operation_setting.AutomaticRetryStatusCodeRanges, operation_setting.AutomaticRetryKeywordsToString()
	t.Cleanup(func() {
		operation_setting.AutomaticRetryStatusCodeRanges = oldRanges
		operation_setting.AutomaticRetryKeywordsFromString(oldKeywords)
	})
	operation_setting.AutomaticRetryStatusCodeRanges = nil
	operation_setting.AutomaticRetryKeywordsFromString("")
	for _, code := range []types.ErrorCode{"server_is_overloaded", "servers_is_overloaded"} {
		for _, scenario := range []string{"503", "429", "500", "400", "401", "403", "budget", "specific", "skip", "cancelled", "deadline", "committed", "moderation"} {
			t.Run(string(code)+"/"+scenario, func(t *testing.T) {
				c, _ := gin.CreateTestContext(httptest.NewRecorder())
				c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
				status, budget, want := http.StatusServiceUnavailable, 1, false
				switch scenario {
				case "503":
					want = true
				case "429":
					status = 429
					want = true
				case "500":
					status = 500
					want = true
				case "400":
					status = 400
				case "401":
					status = 401
				case "403":
					status = 403
				case "budget":
					budget = 0
				case "specific":
					c.Set("specific_channel_id", 461)
				case "skip":
				case "cancelled":
					ctx, cancel := context.WithCancel(c.Request.Context())
					cancel()
					c.Request = c.Request.WithContext(ctx)
				case "deadline":
					ctx, cancel := context.WithTimeout(c.Request.Context(), -1)
					defer cancel()
					c.Request = c.Request.WithContext(ctx)
				case "committed":
					_, _ = c.Writer.Write([]byte("partial"))
				case "moderation":
					c.Set("upstream_moderation_committed", true)
				}
				err := types.NewErrorWithStatusCode(assert.AnError, code, status)
				if scenario == "skip" {
					err = types.NewErrorWithStatusCode(assert.AnError, code, status, types.ErrOptionWithSkipRetry())
				}
				assert.Equal(t, want, shouldRetry(c, err, budget))
			})
		}
	}
}
