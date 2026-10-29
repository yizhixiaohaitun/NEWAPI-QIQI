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

// Exercise the real HTTP 200 stream path, not RelayErrorHandler's non-200 path.
func TestRelayHTTPChatStreamTokenRateLimitSafety(t *testing.T) {
	for _, partial := range []bool{false, true} {
		name := "before_content_retries_without_leaking_failed_prelude"
		if partial {
			name = "after_content_never_switches_channel"
		}
		t.Run(name, func(t *testing.T) {
			oldMode := gin.Mode()
			gin.SetMode(gin.TestMode)
			oldDB, oldLogDB := model.DB, model.LOG_DB
			oldMainType, oldLogType := common.MainDatabaseType(), common.LogDatabaseType()
			oldCache, oldRedis, oldBatch := common.MemoryCacheEnabled, common.RedisEnabled, common.BatchUpdateEnabled
			oldRetry, oldCount, oldLog := common.RetryTimes, constant.CountToken, common.LogConsumeEnabled
			oldErrorLog := constant.ErrorLogEnabled
			oldStreamingTimeout := constant.StreamingTimeout
			oldGroupRatios, oldModelPrices := ratio_setting.GroupRatio2JSONString(), ratio_setting.ModelPrice2JSONString()
			oldFree := operation_setting.GetQuotaSetting().EnableFreeModelPreConsume
			oldPing := operation_setting.GetGeneralSetting().PingIntervalEnabled
			oldRetryRanges := operation_setting.AutomaticRetryStatusCodeRanges
			oldQuotaPerUnit := common.QuotaPerUnit
			t.Cleanup(func() {
				gin.SetMode(oldMode)
				model.DB, model.LOG_DB = oldDB, oldLogDB
				common.SetDatabaseTypes(oldMainType, oldLogType)
				common.MemoryCacheEnabled, common.RedisEnabled, common.BatchUpdateEnabled = oldCache, oldRedis, oldBatch
				common.RetryTimes, constant.CountToken, common.LogConsumeEnabled = oldRetry, oldCount, oldLog
				constant.ErrorLogEnabled = oldErrorLog
				constant.StreamingTimeout = oldStreamingTimeout
				require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(oldGroupRatios))
				require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(oldModelPrices))
				operation_setting.GetQuotaSetting().EnableFreeModelPreConsume = oldFree
				operation_setting.GetGeneralSetting().PingIntervalEnabled = oldPing
				operation_setting.AutomaticRetryStatusCodeRanges = oldRetryRanges
				common.QuotaPerUnit = oldQuotaPerUnit
			})

			common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
			// Existing fixture initializes SQL quoting with SQL_DSN=local and an
			// in-memory SQLite database, explicitly overriding external DB settings.
			db := openRequestCompatibilitySQLite(t, ":memory:")
			require.NoError(t, db.AutoMigrate(&model.Ability{}, &model.Channel{}, &model.User{}, &model.Token{}, &model.Log{}, &model.ChannelPurityGroup{}, &model.ChannelPurityMember{}))
			model.DB, model.LOG_DB = db, db
			// No shared channel cache, Redis, batch writer, production DB or credentials.
			common.MemoryCacheEnabled, common.RedisEnabled, common.BatchUpdateEnabled = false, false, false
			common.RetryTimes, constant.CountToken, common.LogConsumeEnabled = 2, false, true
			constant.ErrorLogEnabled = false
			constant.StreamingTimeout = 30
			operation_setting.GetQuotaSetting().EnableFreeModelPreConsume = false
			operation_setting.GetGeneralSetting().PingIntervalEnabled = false
			// Only a classified 429 may retry; a generic 502 must not accidentally pass.
			operation_setting.AutomaticRetryStatusCodeRanges = []operation_setting.StatusCodeRange{{Start: 429, End: 429}}
			if partial {
				// Both rate-limit and generic stream failures are retryable in policy;
				// only the already-committed content boundary may stop failover.
				operation_setting.AutomaticRetryStatusCodeRanges = append(operation_setting.AutomaticRetryStatusCodeRanges, operation_setting.StatusCodeRange{Start: 502, End: 502})
			}
			common.QuotaPerUnit = 500000
			require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"stream-rate-limit-http":1}`))
			price := `{"gpt-6-astra":0.002}`
			if partial {
				// Partial-output billing policy is not the retry contract under test.
				// A free fixture also avoids an asynchronous terminal refund outliving it.
				price = `{"gpt-6-astra":0}`
			}
			require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(price))
			const initialQuota = 1000000
			require.NoError(t, db.Create(&model.User{Id: 791, Username: "stream-rate-limit-http-user", Quota: initialQuota, Status: common.UserStatusEnabled}).Error)
			require.NoError(t, db.Create(&model.Token{Id: 792, UserId: 791, Key: "stream-rate-limit-http-local-only", RemainQuota: initialQuota, Status: common.TokenStatusEnabled}).Error)

			const errorMessage = "Your requests to gpt-6-astra for gpt-6-astra in eastus2 have exceeded token rate limit."
			var mu sync.Mutex
			var calls []string
			type quotaCheckpoint struct {
				channel     string
				user, token int
				err         error
			}
			var checkpoints []quotaCheckpoint
			recordCall := func(channel string) {
				var user model.User
				var token model.Token
				queryErr := db.First(&user, 791).Error
				if queryErr == nil {
					queryErr = db.First(&token, 792).Error
				}
				mu.Lock()
				defer mu.Unlock()
				calls = append(calls, channel)
				checkpoints = append(checkpoints, quotaCheckpoint{channel: channel, user: user.Quota, token: token.RemainQuota, err: queryErr})
			}
			upstreamA := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				recordCall("A")
				w.Header().Set("Content-Type", "text/event-stream")
				w.WriteHeader(http.StatusOK)
				_, _ = fmt.Fprint(w, "data: {\"id\":\"failed-attempt-a\",\"object\":\"chat.completion.chunk\",\"model\":\"gpt-6-astra\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"content\":\"\"}}]}\n\n")
				if partial {
					_, _ = fmt.Fprint(w, "data: {\"id\":\"failed-attempt-a\",\"object\":\"chat.completion.chunk\",\"model\":\"gpt-6-astra\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"partial\"}}]}\n\n")
				}
				_, _ = fmt.Fprintf(w, "data: {\"error\":{\"message\":%q,\"type\":\"tokens\",\"code\":429}}\n\n", errorMessage)
				w.(http.Flusher).Flush()
			}))
			defer upstreamA.Close()
			upstreamB := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				recordCall("B")
				w.Header().Set("Content-Type", "text/event-stream")
				_, _ = fmt.Fprint(w, "data: {\"id\":\"successful-attempt-b\",\"object\":\"chat.completion.chunk\",\"model\":\"gpt-6-astra\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"content\":\"hello\"}}]}\n\n")
				_, _ = fmt.Fprint(w, "data: {\"id\":\"successful-attempt-b\",\"object\":\"chat.completion.chunk\",\"model\":\"gpt-6-astra\",\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":5,\"completion_tokens\":1,\"total_tokens\":6}}\n\ndata: [DONE]\n\n")
				w.(http.Flusher).Flush()
			}))
			defer upstreamB.Close()
			baseA, baseB := upstreamA.URL, upstreamB.URL
			high, low, weight := int64(10), int64(0), uint(1)
			require.NoError(t, db.Create(&[]model.Channel{
				{Id: 793, Type: constant.ChannelTypeOpenAI, Name: "stream-a", Key: "local-a", BaseURL: &baseA, Group: "stream-rate-limit-http", Models: "gpt-6-astra", Status: common.ChannelStatusEnabled, Priority: &high, Weight: &weight},
				{Id: 794, Type: constant.ChannelTypeOpenAI, Name: "stream-b", Key: "local-b", BaseURL: &baseB, Group: "stream-rate-limit-http", Models: "gpt-6-astra", Status: common.ChannelStatusEnabled, Priority: &low, Weight: &weight},
			}).Error)
			require.NoError(t, db.Create(&[]model.Ability{
				{Group: "stream-rate-limit-http", Model: "gpt-6-astra", ChannelId: 793, Enabled: true, Priority: &high, Weight: 1},
				{Group: "stream-rate-limit-http", Model: "gpt-6-astra", ChannelId: 794, Enabled: true, Priority: &low, Weight: 1},
			}).Error)
			service.InitHttpClient()
			finished := make(chan []string, 1)
			router := gin.New()
			router.POST("/v1/chat/completions", func(c *gin.Context) {
				common.SetContextKey(c, constant.ContextKeyUsingGroup, "stream-rate-limit-http")
				common.SetContextKey(c, constant.ContextKeyTokenGroup, "stream-rate-limit-http")
				common.SetContextKey(c, constant.ContextKeyUserSetting, dto.UserSetting{BillingPreference: "wallet_only", AcceptUnsetRatioModel: true})
				common.SetContextKey(c, constant.ContextKeyUserId, 791)
				common.SetContextKey(c, constant.ContextKeyUserQuota, initialQuota)
				common.SetContextKey(c, constant.ContextKeyTokenId, 792)
				common.SetContextKey(c, constant.ContextKeyTokenKey, "stream-rate-limit-http-local-only")
				// Force real pre-consumption instead of the high-balance trust bypass.
				c.Set("token_quota", 0)
				common.SetContextKey(c, common.RequestIdKey, "stream-rate-limit-http")
				c.Next()
			}, middleware.Distribute(), func(c *gin.Context) {
				defer func() { finished <- append([]string(nil), c.GetStringSlice("use_channel")...) }()
				Relay(c, types.RelayFormatOpenAI)
			})
			gateway := httptest.NewServer(router)
			defer gateway.Close()
			client := gateway.Client()
			client.Timeout = 10 * time.Second
			response, err := client.Post(gateway.URL+"/v1/chat/completions", "application/json", strings.NewReader(`{"model":"gpt-6-astra","stream":true,"stream_options":{"include_usage":true},"messages":[{"role":"user","content":"hi"}]}`))
			require.NoError(t, err)
			defer response.Body.Close()
			bodyBytes, err := io.ReadAll(response.Body)
			require.NoError(t, err)
			body := string(bodyBytes)
			var usedChannels []string
			select {
			case usedChannels = <-finished:
			case <-time.After(10 * time.Second):
				t.Fatal("local relay handler did not finish")
			}
			assert.Equal(t, http.StatusOK, response.StatusCode, body)
			mu.Lock()
			actualCalls := append([]string(nil), calls...)
			actualCheckpoints := append([]quotaCheckpoint(nil), checkpoints...)
			mu.Unlock()
			if partial {
				assert.Equal(t, []string{"A"}, actualCalls)
				assert.Equal(t, []string{"793"}, usedChannels)
				assert.Contains(t, body, `"content":"partial"`)
				assert.NotContains(t, body, "hello")
				assert.NotContains(t, body, "successful-attempt-b")
				return
			}
			assert.Equal(t, []string{"A", "B"}, actualCalls)
			assert.Equal(t, []string{"793", "794"}, usedChannels)
			assert.Contains(t, body, `"content":"hello"`)
			assert.Contains(t, body, "successful-attempt-b")
			assert.Contains(t, body, "data: [DONE]")
			assert.NotContains(t, body, "failed-attempt-a")
			assert.NotContains(t, body, errorMessage)
			assert.NotContains(t, body, `"error"`)
			// Fixed per-request pricing: 0.002 * 500000 = 1000. Both upstream
			// attempts must see ONE outstanding pre-consumption, not two or a refund.
			assert.Len(t, actualCheckpoints, 2)
			for _, checkpoint := range actualCheckpoints {
				require.NoError(t, checkpoint.err)
				assert.Equal(t, initialQuota-1000, checkpoint.user, checkpoint.channel)
				assert.Equal(t, initialQuota-1000, checkpoint.token, checkpoint.channel)
			}
			var user model.User
			var token model.Token
			require.NoError(t, db.First(&user, 791).Error)
			require.NoError(t, db.First(&token, 792).Error)
			assert.Equal(t, initialQuota-1000, user.Quota)
			assert.Equal(t, 1000, user.UsedQuota)
			assert.Equal(t, 1, user.RequestCount)
			assert.Equal(t, initialQuota-1000, token.RemainQuota)
			assert.Equal(t, 1000, token.UsedQuota)
			var logs []model.Log
			require.NoError(t, db.Where("type = ?", model.LogTypeConsume).Find(&logs).Error)
			if assert.Len(t, logs, 1, "failed attempt must not settle or create a consumption record") {
				assert.Equal(t, 794, logs[0].ChannelId)
				assert.Equal(t, 1000, logs[0].Quota)
			}
			var channels []model.Channel
			require.NoError(t, db.Order("id").Find(&channels).Error)
			require.Len(t, channels, 2)
			assert.Zero(t, channels[0].UsedQuota, "failed attempt must not be billed")
			assert.Equal(t, int64(1000), channels[1].UsedQuota)
		})
	}
}
