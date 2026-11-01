package controller

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestEmptyStreamRetryRespectsStops(t *testing.T) {
	setting := operation_setting.GetQiqiSetting()
	original := *setting
	originalRanges := operation_setting.AutomaticRetryStatusCodeRanges
	originalKeywords := operation_setting.AutomaticRetryKeywordsToString()
	t.Cleanup(func() {
		*setting = original
		operation_setting.AutomaticRetryStatusCodeRanges = originalRanges
		operation_setting.AutomaticRetryKeywordsFromString(originalKeywords)
	})
	setting.ResponsesStreamErrorRetryEnabled = true
	operation_setting.AutomaticRetryStatusCodeRanges = nil
	operation_setting.AutomaticRetryKeywordsFromString("")
	for _, scenario := range []string{"early_error", "not_marked", "budget", "specific", "skip", "cancelled", "deadline", "committed", "disabled", "affinity"} {
		t.Run(scenario, func(t *testing.T) {
			setting.ResponsesStreamErrorRetryEnabled = true
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
			c.Set("upstream_stream_error_before_commit", true)
			err := types.NewOpenAIError(fmt.Errorf("provider failure"), types.ErrorCodeBadResponse, http.StatusBadRequest)
			budget := 1
			switch scenario {
			case "not_marked":
				c.Set("upstream_stream_error_before_commit", false)
			case "budget":
				budget = 0
			case "specific":
				c.Set("specific_channel_id", 1)
			case "skip":
				err = types.NewOpenAIError(fmt.Errorf("provider failure"), types.ErrorCodeBadResponse, 400, types.ErrOptionWithSkipRetry())
			case "cancelled":
				ctx, cancel := context.WithCancel(c.Request.Context())
				cancel()
				c.Request = c.Request.WithContext(ctx)
			case "deadline":
				err = service.NewUpstreamTimeoutError()
			case "committed":
				_, _ = c.Writer.WriteString("partial")
			case "disabled":
				setting.ResponsesStreamErrorRetryEnabled = false
			case "affinity":
				c.Set("channel_affinity_skip_retry_on_failure", true)
			}
			require.Equal(t, scenario == "early_error", shouldRetry(c, err, budget))
		})
	}
}
