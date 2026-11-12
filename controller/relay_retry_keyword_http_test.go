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
	"github.com/QuantumNous/new-api/setting/model_setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const flaggedPromptUpstreamBody = `{"error":{"message":"Invalid prompt: your prompt was flagged as potentially violating our usage policy. Please try again with a different prompt: https://platform.example.com/docs/guides/reasoning#advice-on-prompting","type":"invalid_request_error","param":null,"code":"invalid_prompt"}}`

// A 400 is normally final (default retry ranges exclude 400). The flagged
// prompt 400 must still fail over through the real HTTP relay path, and only
// while its keyword is configured.
func TestRelayHTTPRetryKeywordFlaggedPrompt400(t *testing.T) {
	for _, tc := range []struct {
		name           string
		body           string
		status         int
		keywordEnabled bool
		wantRetry      bool
	}{
		{"keyword_configured_fails_over", flaggedPromptUpstreamBody, 400, true, true},
		{"keyword_removed_stays_final_400", flaggedPromptUpstreamBody, 400, false, false},
		{"plural_overload_without_message", `{"error":{"message":"busy","type":"new_api_error","code":"servers_is_overloaded"}}`, 503, false, true},
		{"singular_overload_without_message", `{"error":{"message":"busy","type":"new_api_error","code":"server_is_overloaded"}}`, 503, false, true},
		{"plural_overload_original_message", `{"error":{"message":"Our servers are currently overloaded","type":"new_api_error","code":"servers_is_overloaded"}}`, 503, false, true},
		{"plural_overload_bad_request_final", `{"error":{"message":"busy","type":"new_api_error","code":"servers_is_overloaded"}}`, 400, false, false},
		{"unrelated_server_error_final", `{"error":{"message":"busy","type":"new_api_error","code":"some_other_error"}}`, 503, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			oldMode := gin.Mode()
			gin.SetMode(gin.TestMode)
			oldDB, oldLogDB := model.DB, model.LOG_DB
			oldMainType, oldLogType := common.MainDatabaseType(), common.LogDatabaseType()
			oldCache, oldRedis, oldBatch := common.MemoryCacheEnabled, common.RedisEnabled, common.BatchUpdateEnabled
			oldRetry, oldCount, oldLog := common.RetryTimes, constant.CountToken, common.LogConsumeEnabled
			oldErrorLog := constant.ErrorLogEnabled
			oldGroupRatios, oldModelPrices := ratio_setting.GroupRatio2JSONString(), ratio_setting.ModelPrice2JSONString()
			oldFree := operation_setting.GetQuotaSetting().EnableFreeModelPreConsume
			oldPing := operation_setting.GetGeneralSetting().PingIntervalEnabled
			oldRetryRanges := operation_setting.AutomaticRetryStatusCodeRanges
			oldKeywords := operation_setting.AutomaticRetryKeywordsToString()
			oldAuto := model_setting.GetGlobalSettings().AutoChatToResponsesEnabled
			model_setting.GetGlobalSettings().AutoChatToResponsesEnabled = false
			oldAutoDisable := common.AutomaticDisableChannelEnabled
			t.Cleanup(func() {
				model_setting.GetGlobalSettings().AutoChatToResponsesEnabled = oldAuto
				gin.SetMode(oldMode)
				model.DB, model.LOG_DB = oldDB, oldLogDB
				common.SetDatabaseTypes(oldMainType, oldLogType)
				common.MemoryCacheEnabled, common.RedisEnabled, common.BatchUpdateEnabled = oldCache, oldRedis, oldBatch
				common.RetryTimes, constant.CountToken, common.LogConsumeEnabled = oldRetry, oldCount, oldLog
				constant.ErrorLogEnabled = oldErrorLog
				require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(oldGroupRatios))
				require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(oldModelPrices))
				operation_setting.GetQuotaSetting().EnableFreeModelPreConsume = oldFree
				operation_setting.GetGeneralSetting().PingIntervalEnabled = oldPing
				operation_setting.AutomaticRetryStatusCodeRanges = oldRetryRanges
				operation_setting.AutomaticRetryKeywordsFromString(oldKeywords)
				common.AutomaticDisableChannelEnabled = oldAutoDisable
			})

			common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
			db := openRequestCompatibilitySQLite(t, ":memory:")
			require.NoError(t, db.AutoMigrate(&model.Ability{}, &model.Channel{}, &model.User{}, &model.Token{}, &model.Log{}, &model.ChannelPurityGroup{}, &model.ChannelPurityMember{}))
			model.DB, model.LOG_DB = db, db
			common.MemoryCacheEnabled, common.RedisEnabled, common.BatchUpdateEnabled = false, false, false
			common.RetryTimes, constant.CountToken, common.LogConsumeEnabled = 2, false, true
			constant.ErrorLogEnabled = false
			common.AutomaticDisableChannelEnabled = false
			operation_setting.GetQuotaSetting().EnableFreeModelPreConsume = false
			operation_setting.GetGeneralSetting().PingIntervalEnabled = false
			// Production default ranges: 400 is NOT retryable by status code.
			require.NoError(t, operation_setting.AutomaticRetryStatusCodesFromString("100-199,300-399,401-407,409-499,500-503,505-523,525-599"))
			require.False(t, operation_setting.ShouldRetryByStatusCode(http.StatusBadRequest))
			if tc.keywordEnabled {
				// Admin pastes the message fragment; line format from the textarea.
				operation_setting.AutomaticRetryKeywordsFromString("some other error\r\nYour prompt was flagged as potentially violating our usage policy\n")
			} else {
				operation_setting.AutomaticRetryKeywordsFromString("")
			}
			if strings.Contains(tc.name, "overload") || tc.name == "unrelated_server_error_final" {
				require.NoError(t, operation_setting.AutomaticRetryStatusCodesFromString("429"))
			}
			require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"retry-keyword-http":1}`))
			require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(`{"gpt-6-astra":0}`))
			const initialQuota = 1000000
			require.NoError(t, db.Create(&model.User{Id: 891, Username: "retry-keyword-http-user", Quota: initialQuota, Status: common.UserStatusEnabled}).Error)
			require.NoError(t, db.Create(&model.Token{Id: 892, UserId: 891, Key: "retry-keyword-http-local-only", RemainQuota: initialQuota, Status: common.TokenStatusEnabled}).Error)

			var mu sync.Mutex
			var calls []string
			record := func(name string) { mu.Lock(); calls = append(calls, name); mu.Unlock() }
			upstreamA := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				record("A")
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tc.status)
				_, _ = fmt.Fprint(w, tc.body)
			}))
			defer upstreamA.Close()
			upstreamB := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				record("B")
				w.Header().Set("Content-Type", "application/json")
				_, _ = fmt.Fprint(w, `{"id":"ok-b","object":"chat.completion","model":"gpt-6-astra","choices":[{"index":0,"message":{"role":"assistant","content":"hello"},"finish_reason":"stop"}],"usage":{"prompt_tokens":5,"completion_tokens":1,"total_tokens":6}}`)
			}))
			defer upstreamB.Close()
			baseA, baseB := upstreamA.URL, upstreamB.URL
			high, low, weight := int64(10), int64(0), uint(1)
			require.NoError(t, db.Create(&[]model.Channel{
				{Id: 893, Type: constant.ChannelTypeOpenAI, Name: "kw-a", Key: "local-a", BaseURL: &baseA, Group: "retry-keyword-http", Models: "gpt-6-astra", Status: common.ChannelStatusEnabled, Priority: &high, Weight: &weight},
				{Id: 894, Type: constant.ChannelTypeOpenAI, Name: "kw-b", Key: "local-b", BaseURL: &baseB, Group: "retry-keyword-http", Models: "gpt-6-astra", Status: common.ChannelStatusEnabled, Priority: &low, Weight: &weight},
			}).Error)
			require.NoError(t, db.Create(&[]model.Ability{
				{Group: "retry-keyword-http", Model: "gpt-6-astra", ChannelId: 893, Enabled: true, Priority: &high, Weight: 1},
				{Group: "retry-keyword-http", Model: "gpt-6-astra", ChannelId: 894, Enabled: true, Priority: &low, Weight: 1},
			}).Error)
			service.InitHttpClient()
			finished := make(chan []string, 1)
			router := gin.New()
			router.POST("/v1/chat/completions", func(c *gin.Context) {
				common.SetContextKey(c, constant.ContextKeyUsingGroup, "retry-keyword-http")
				common.SetContextKey(c, constant.ContextKeyTokenGroup, "retry-keyword-http")
				common.SetContextKey(c, constant.ContextKeyUserSetting, dto.UserSetting{BillingPreference: "wallet_only", AcceptUnsetRatioModel: true})
				common.SetContextKey(c, constant.ContextKeyUserId, 891)
				common.SetContextKey(c, constant.ContextKeyUserQuota, initialQuota)
				common.SetContextKey(c, constant.ContextKeyTokenId, 892)
				common.SetContextKey(c, constant.ContextKeyTokenKey, "retry-keyword-http-local-only")
				common.SetContextKey(c, common.RequestIdKey, "retry-keyword-http")
				c.Next()
			}, middleware.Distribute(), func(c *gin.Context) {
				defer func() { finished <- append([]string(nil), c.GetStringSlice("use_channel")...) }()
				Relay(c, types.RelayFormatOpenAI)
			})
			gateway := httptest.NewServer(router)
			defer gateway.Close()
			client := gateway.Client()
			client.Timeout = 10 * time.Second
			response, err := client.Post(gateway.URL+"/v1/chat/completions", "application/json", strings.NewReader(`{"model":"gpt-6-astra","messages":[{"role":"user","content":"hi"}]}`))
			require.NoError(t, err)
			defer response.Body.Close()
			bodyBytes, err := io.ReadAll(response.Body)
			require.NoError(t, err)
			body := string(bodyBytes)
			var used []string
			select {
			case used = <-finished:
			case <-time.After(10 * time.Second):
				t.Fatal("local relay handler did not finish")
			}
			mu.Lock()
			actual := append([]string(nil), calls...)
			mu.Unlock()

			if tc.wantRetry {
				assert.Equal(t, http.StatusOK, response.StatusCode, body)
				assert.Equal(t, []string{"A", "B"}, actual)
				assert.Equal(t, []string{"893", "894"}, used)
				assert.Contains(t, body, `"content":"hello"`)
				assert.NotContains(t, body, "flagged")
				return
			}
			assert.Equal(t, tc.status, response.StatusCode, body)
			assert.Equal(t, []string{"A"}, actual)
			assert.Equal(t, []string{"893"}, used)
		})
	}
}
