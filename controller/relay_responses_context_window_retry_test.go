package controller

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// HTTP 200 + SSE failures before output retry without depending on keywords
// or enabling ordinary HTTP 400 retries. Terminal errors must also finish when
// the upstream deliberately leaves its response body open.
func TestRelayHTTPResponsesContextWindowRetry(t *testing.T) {
	service.InitTokenEncoders()
	for _, scenario := range []string{"before_output", "after_output", "after_tool", "after_reasoning", "empty_delta_done", "empty_completed", "empty_eof", "malformed", "flat_error", "unmatched_error", "budget_exhausted", "before_output_held_open", "after_output_held_open", "after_tool_held_open", "after_reasoning_held_open", "after_audit_held_open", "retry_disabled_held_open", "budget_exhausted_held_open"} {
		t.Run(scenario, func(t *testing.T) {
			heldOpen := strings.HasSuffix(scenario, "_held_open")
			scenario := strings.TrimSuffix(scenario, "_held_open")
			oldMode := gin.Mode()
			gin.SetMode(gin.TestMode)
			oldDB, oldLogDB := model.DB, model.LOG_DB
			oldMainType, oldLogType := common.MainDatabaseType(), common.LogDatabaseType()
			oldCache, oldRedis, oldBatch := common.MemoryCacheEnabled, common.RedisEnabled, common.BatchUpdateEnabled
			oldRetry, oldCount, oldLog := common.RetryTimes, constant.CountToken, common.LogConsumeEnabled
			oldErrorLog, oldTimeout := constant.ErrorLogEnabled, constant.StreamingTimeout
			oldRatios, oldPrices := ratio_setting.GroupRatio2JSONString(), ratio_setting.ModelPrice2JSONString()
			oldFree, oldPing := operation_setting.GetQuotaSetting().EnableFreeModelPreConsume, operation_setting.GetGeneralSetting().PingIntervalEnabled
			oldRanges, oldKeywords := operation_setting.AutomaticRetryStatusCodeRanges, operation_setting.AutomaticRetryKeywordsToString()
			qiqi := operation_setting.GetQiqiSetting()
			oldQiqi := *qiqi
			t.Cleanup(func() {
				gin.SetMode(oldMode)
				model.DB, model.LOG_DB = oldDB, oldLogDB
				common.SetDatabaseTypes(oldMainType, oldLogType)
				common.MemoryCacheEnabled, common.RedisEnabled, common.BatchUpdateEnabled = oldCache, oldRedis, oldBatch
				common.RetryTimes, constant.CountToken, common.LogConsumeEnabled = oldRetry, oldCount, oldLog
				constant.ErrorLogEnabled, constant.StreamingTimeout = oldErrorLog, oldTimeout
				require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(oldRatios))
				require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(oldPrices))
				operation_setting.GetQuotaSetting().EnableFreeModelPreConsume = oldFree
				operation_setting.GetGeneralSetting().PingIntervalEnabled = oldPing
				operation_setting.AutomaticRetryStatusCodeRanges = oldRanges
				operation_setting.AutomaticRetryKeywordsFromString(oldKeywords)
				*qiqi = oldQiqi
			})
			common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
			db := openRequestCompatibilitySQLite(t, ":memory:")
			require.NoError(t, db.AutoMigrate(&model.Ability{}, &model.Channel{}, &model.User{}, &model.Token{}, &model.Log{}, &model.ChannelPurityGroup{}, &model.ChannelPurityMember{}))
			model.DB, model.LOG_DB = db, db
			common.MemoryCacheEnabled, common.RedisEnabled, common.BatchUpdateEnabled = false, false, false
			common.RetryTimes, constant.CountToken, common.LogConsumeEnabled = 0, false, false
			constant.ErrorLogEnabled, constant.StreamingTimeout = false, 30
			operation_setting.GetQuotaSetting().EnableFreeModelPreConsume = false
			operation_setting.GetGeneralSetting().PingIntervalEnabled = false
			operation_setting.AutomaticRetryStatusCodeRanges = []operation_setting.StatusCodeRange{{Start: 429, End: 429}}
			operation_setting.AutomaticRetryKeywordsFromString("") // Stream failures must not depend on configured error text.
			qiqi.ResponsesStreamErrorRetryEnabled = scenario != "retry_disabled"
			qiqi.ResponsesStreamErrorRetryTimes = 1
			require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"context-window-http":1}`))
			require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(`{"gpt-6.1-terra":0}`))
			require.NoError(t, db.Create(&model.User{Id: 891, Username: "context-window-http-user", Quota: 1000000, Status: common.UserStatusEnabled}).Error)
			require.NoError(t, db.Create(&model.Token{Id: 892, UserId: 891, Key: "context-window-http-local-only", RemainQuota: 1000000, Status: common.TokenStatusEnabled}).Error)

			const errorMessage = "Your input exceeds the context window of this model. Please adjust your input and try again."
			const requestBody = `{"model":"gpt-6.1-terra","stream":true,"input":"hi"}`
			var mu sync.Mutex
			var calls, bodies []string
			upstream := func(name string, fails bool) *httptest.Server {
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					b, readErr := io.ReadAll(r.Body)
					mu.Lock()
					calls = append(calls, name)
					bodies = append(bodies, string(b))
					mu.Unlock()
					assert.NoError(t, readErr)
					assert.NoError(t, r.Context().Err(), "retry must use a fresh live upstream context")
					assert.Equal(t, "/v1/responses", r.URL.Path)
					w.Header().Set("Content-Type", "text/event-stream")
					_, _ = fmt.Fprintf(w, "event: response.created\ndata: {\"type\":\"response.created\",\"response\":{\"id\":\"attempt-%s\",\"status\":\"in_progress\"}}\n\n", name)
					if fails {
						if scenario == "after_output" {
							_, _ = fmt.Fprint(w, "event: response.output_text.delta\ndata: {\"type\":\"response.output_text.delta\",\"delta\":\"partial\"}\n\n")
						}
						switch scenario {
						case "after_audit":
							_, _ = fmt.Fprint(w, `data: {"type":"response.in_progress","safeguard_results":[]}`+"\n\n")
						case "after_tool":
							_, _ = fmt.Fprint(w, "data: {\"type\":\"response.output_item.done\",\"item\":{\"type\":\"function_call\",\"name\":\"lookup\",\"call_id\":\"call_1\",\"arguments\":\"{}\"}}\n\n")
						case "after_reasoning":
							_, _ = fmt.Fprint(w, "data: {\"type\":\"response.reasoning_summary_text.delta\",\"delta\":\"thinking\"}\n\n")
						case "empty_delta_done":
							_, _ = fmt.Fprint(w, "data: {\"type\":\"response.output_text.delta\",\"delta\":\"\"}\n\ndata: {\"type\":\"response.output_text.done\",\"text\":\"\"}\n\ndata: {\"type\":\"response.output_item.done\",\"item\":{\"type\":\"message\",\"content\":[]}}\n\n")
						case "empty_completed":
							_, _ = fmt.Fprint(w, "data: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\",\"output\":[],\"usage\":{\"input_tokens\":0,\"output_tokens\":0}}}\n\n")
							return
						case "empty_eof":
							return
						case "malformed":
							_, _ = fmt.Fprint(w, "data: {invalid-json}\n\n")
							return
						case "flat_error":
							_, _ = fmt.Fprintf(w, "data: {\"type\":\"error\",\"code\":\"context_length_exceeded\",\"message\":%q}\n\n", errorMessage)
							return
						case "unmatched_error":
							_, _ = fmt.Fprint(w, "data: {\"type\":\"response.failed\",\"response\":{\"status\":\"failed\",\"error\":{\"type\":\"invalid_request_error\",\"message\":\"upstream could not produce output\"}}}\n\n")
							return
						}
						_, _ = fmt.Fprintf(w, "event: error\ndata: {\"type\":\"error\",\"error\":{\"type\":\"invalid_request_error\",\"code\":\"context_length_exceeded\",\"message\":%q}}\n\n", errorMessage)
						if heldOpen {
							w.(http.Flusher).Flush()
							<-r.Context().Done()
							return
						}
					} else {
						_, _ = fmt.Fprint(w, "event: response.output_text.delta\ndata: {\"type\":\"response.output_text.delta\",\"delta\":\"hello\"}\n\n")
						_, _ = fmt.Fprint(w, "event: response.completed\ndata: {\"type\":\"response.completed\",\"response\":{\"id\":\"attempt-B\",\"status\":\"completed\",\"usage\":{\"input_tokens\":5,\"output_tokens\":1,\"total_tokens\":6}}}\n\n")
					}
					w.(http.Flusher).Flush()
				}))
			}
			a, b := upstream("A", true), upstream("B", scenario == "budget_exhausted")
			defer a.Close()
			defer b.Close()
			baseA, baseB := a.URL, b.URL
			high, low, weight := int64(10), int64(0), uint(1)
			require.NoError(t, db.Create(&[]model.Channel{
				{Id: 893, Type: constant.ChannelTypeOpenAI, Name: "context-a", Key: "local-a", BaseURL: &baseA, Group: "context-window-http", Models: "gpt-6.1-terra", Status: common.ChannelStatusEnabled, Priority: &high, Weight: &weight},
				{Id: 894, Type: constant.ChannelTypeOpenAI, Name: "context-b", Key: "local-b", BaseURL: &baseB, Group: "context-window-http", Models: "gpt-6.1-terra", Status: common.ChannelStatusEnabled, Priority: &low, Weight: &weight},
			}).Error)
			require.NoError(t, db.Create(&[]model.Ability{
				{Group: "context-window-http", Model: "gpt-6.1-terra", ChannelId: 893, Enabled: true, Priority: &high, Weight: 1},
				{Group: "context-window-http", Model: "gpt-6.1-terra", ChannelId: 894, Enabled: true, Priority: &low, Weight: 1},
			}).Error)
			service.InitHttpClient()
			finished := make(chan []string, 1)
			router := gin.New()
			router.POST("/v1/responses", func(c *gin.Context) {
				common.SetContextKey(c, constant.ContextKeyUsingGroup, "context-window-http")
				common.SetContextKey(c, constant.ContextKeyTokenGroup, "context-window-http")
				common.SetContextKey(c, constant.ContextKeyUserSetting, dto.UserSetting{BillingPreference: "wallet_only", AcceptUnsetRatioModel: true})
				common.SetContextKey(c, constant.ContextKeyUserId, 891)
				common.SetContextKey(c, constant.ContextKeyUserQuota, 1000000)
				common.SetContextKey(c, constant.ContextKeyTokenId, 892)
				common.SetContextKey(c, constant.ContextKeyTokenKey, "context-window-http-local-only")
				common.SetContextKey(c, common.RequestIdKey, "context-window-http")
				c.Next()
			}, middleware.Distribute(), func(c *gin.Context) {
				defer func() { finished <- append([]string(nil), c.GetStringSlice("use_channel")...) }()
				Relay(c, types.RelayFormatOpenAIResponses)
			})
			gateway := httptest.NewServer(router)
			defer gateway.Close()
			client := gateway.Client()
			client.Timeout = 2 * time.Second
			// One client HTTP request encompasses both upstream attempts.
			response, err := client.Post(gateway.URL+"/v1/responses", "application/json", strings.NewReader(requestBody))
			require.NoError(t, err)
			defer response.Body.Close()
			output, err := io.ReadAll(response.Body)
			require.NoError(t, err, "no prematurely closed client stream")
			body := string(output)
			var used []string
			select {
			case used = <-finished:
			case <-time.After(10 * time.Second):
				t.Fatal("relay did not finish within the retry budget")
			}
			mu.Lock()
			actualCalls, actualBodies := append([]string(nil), calls...), append([]string(nil), bodies...)
			mu.Unlock()
			for _, actual := range actualBodies {
				assert.JSONEq(t, requestBody, actual, "failover must preserve the original input")
			}
			if scenario == "after_output" || scenario == "after_tool" || scenario == "after_reasoning" || scenario == "after_audit" || scenario == "retry_disabled" {
				assert.Equal(t, http.StatusOK, response.StatusCode, body)
				assert.Equal(t, []string{"A"}, actualCalls)
				assert.Equal(t, []string{"893"}, used)
				switch scenario {
				case "after_output":
					assert.Contains(t, body, "partial")
				case "after_tool":
					assert.Contains(t, body, "call_1")
				case "after_audit":
					assert.Equal(t, 1, strings.Count(body, `"safeguard_results":[]`))
				case "after_reasoning":
					assert.Contains(t, body, "thinking")
				}
				assert.Equal(t, 1, strings.Count(body, errorMessage), "terminal error must be delivered exactly once")
				assert.NotContains(t, body, "hello")
				return
			}
			assert.Equal(t, []string{"A", "B"}, actualCalls)
			assert.Equal(t, []string{"893", "894"}, used)
			assert.NotContains(t, body, "attempt-A", "failed attempt's buffered prelude must not leak")
			if scenario == "budget_exhausted" {
				assert.Equal(t, http.StatusBadRequest, response.StatusCode, body)
				assert.Contains(t, body, errorMessage)
				assert.NotContains(t, body, "hello")
				return
			}
			assert.Equal(t, http.StatusOK, response.StatusCode, body)
			assert.Contains(t, response.Header.Get("Content-Type"), "text/event-stream")
			assert.Contains(t, body, `"delta":"hello"`)
			assert.Contains(t, body, "response.completed")
			assert.Contains(t, body, "attempt-B")
			assert.NotContains(t, body, errorMessage)
			assert.NotContains(t, body, `"error"`)
		})
	}
}
