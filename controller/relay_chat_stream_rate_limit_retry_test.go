package controller

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// Azure can answer a streaming Chat request with HTTP 200 and then put the
// token rate-limit error inside the SSE body. Before any visible output, that
// attempt must go back to channel retry instead of reaching the client.
func TestRelayHTTPChatStreamRateLimitBeforeOutputRetriesNextChannel(t *testing.T) {
	gin.SetMode(gin.TestMode)
	oldDB, oldLogDB, oldCache, oldRetryTimes, oldCountToken, oldRedis := model.DB, model.LOG_DB, common.MemoryCacheEnabled, common.RetryTimes, constant.CountToken, common.RedisEnabled
	oldUsableGroups := setting.UserUsableGroups2JSONString()
	oldGroupRatios := ratio_setting.GroupRatio2JSONString()
	oldModelPrices := ratio_setting.ModelPrice2JSONString()
	oldFreePreConsume := operation_setting.GetQuotaSetting().EnableFreeModelPreConsume
	qiqi := operation_setting.GetQiqiSetting()
	oldQiqi := *qiqi
	oldStreamingTimeout := constant.StreamingTimeout
	t.Cleanup(func() {
		model.DB = oldDB
		model.LOG_DB = oldLogDB
		common.MemoryCacheEnabled = oldCache
		common.RedisEnabled = oldRedis
		common.RetryTimes = oldRetryTimes
		constant.CountToken = oldCountToken
		constant.StreamingTimeout = oldStreamingTimeout
		*qiqi = oldQiqi
		require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(oldUsableGroups))
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(oldGroupRatios))
		require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(oldModelPrices))
		operation_setting.GetQuotaSetting().EnableFreeModelPreConsume = oldFreePreConsume
	})

	var mu sync.Mutex
	calls := make([]string, 0, 2)
	record := func(name string) {
		mu.Lock()
		calls = append(calls, name)
		mu.Unlock()
	}
	rateLimited := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		record("A")
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(strings.Join([]string{
			`data: {"choices":[],"created":0,"id":"","model":"","object":"","prompt_filter_results":[{"prompt_index":0,"content_filter_results":{}}]}`,
			``,
			`data: {"id":"chatcmpl-a","object":"chat.completion.chunk","created":1,"model":"gpt-6-astra","choices":[{"index":0,"delta":{"role":"assistant","content":""},"finish_reason":null}]}`,
			``,
			`data: {"error":{"code":"429","message":"Your requests to gpt-6-astra for gpt-6-astra in eastus2 have exceeded token rate limit."}}`,
			``,
			`data: {"error":{"code":"429","message":"Your requests to gpt-6-astra for gpt-6-astra in eastus2 have exceeded token rate limit."}}`,
			``,
		}, "\n")))
	}))
	defer rateLimited.Close()
	healthy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		record("B")
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(strings.Join([]string{
			`data: {"id":"chatcmpl-b","object":"chat.completion.chunk","created":1,"model":"gpt-6-astra","choices":[{"index":0,"delta":{"role":"assistant","content":""},"finish_reason":null}]}`,
			``,
			`data: {"id":"chatcmpl-b","object":"chat.completion.chunk","created":1,"model":"gpt-6-astra","choices":[{"index":0,"delta":{"content":"ok-from-b"},"finish_reason":null}]}`,
			``,
			`data: {"id":"chatcmpl-b","object":"chat.completion.chunk","created":1,"model":"gpt-6-astra","choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":3,"completion_tokens":2,"total_tokens":5}}`,
			``,
			`data: [DONE]`,
			``,
		}, "\n")))
	}))
	defer healthy.Close()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Ability{}, &model.Channel{}, &model.User{}, &model.Log{}, &model.ChannelPurityGroup{}, &model.ChannelPurityMember{}))
	model.DB = db
	model.LOG_DB = db
	common.MemoryCacheEnabled = true
	common.RedisEnabled = false
	// Production default can be zero; the early stream rule supplies its own budget.
	common.RetryTimes = 0
	constant.CountToken = false
	constant.StreamingTimeout = 30
	qiqi.ResponsesStreamErrorRetryEnabled = true
	qiqi.ResponsesStreamErrorRetryTimes = operation_setting.DefaultResponsesStreamErrorRetryTimes
	require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{"default":"default"}`))
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":0}`))
	require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(`{"gpt-6-astra":0}`))
	operation_setting.GetQuotaSetting().EnableFreeModelPreConsume = false
	high, low, weight := int64(10), int64(5), uint(1)
	baseA, baseB := rateLimited.URL, healthy.URL
	require.NoError(t, db.Create(&model.User{Id: 1, Username: "stream-retry-user", Quota: 1000000, Status: common.UserStatusEnabled}).Error)
	require.NoError(t, db.Create(&[]model.Channel{
		{Id: 501, Name: "azure-limited", Key: "key-a", BaseURL: &baseA, Group: "default", Models: "gpt-6-astra", Status: common.ChannelStatusEnabled, Priority: &high, Weight: &weight},
		{Id: 502, Name: "healthy", Key: "key-b", BaseURL: &baseB, Group: "default", Models: "gpt-6-astra", Status: common.ChannelStatusEnabled, Priority: &low, Weight: &weight},
	}).Error)
	require.NoError(t, db.Create(&[]model.Ability{
		{Group: "default", Model: "gpt-6-astra", ChannelId: 501, Enabled: true, Priority: &high, Weight: 1},
		{Group: "default", Model: "gpt-6-astra", ChannelId: 502, Enabled: true, Priority: &low, Weight: 1},
	}).Error)
	model.InitChannelCache()
	service.InitHttpClient()

	var usedChannels []string
	router := gin.New()
	router.POST("/v1/chat/completions", func(c *gin.Context) {
		common.SetContextKey(c, constant.ContextKeyUsingGroup, "default")
		common.SetContextKey(c, constant.ContextKeyTokenGroup, "default")
		common.SetContextKey(c, constant.ContextKeyUserGroup, "default")
		common.SetContextKey(c, constant.ContextKeyUserSetting, dto.UserSetting{AcceptUnsetRatioModel: true})
		common.SetContextKey(c, constant.ContextKeyUserId, 1)
		common.SetContextKey(c, constant.ContextKeyUserQuota, 1000000)
		common.SetContextKey(c, common.RequestIdKey, "chat-stream-rate-limit-retry")
		c.Next()
	}, middleware.Distribute(), func(c *gin.Context) {
		Relay(c, types.RelayFormatOpenAI)
		usedChannels = append([]string(nil), c.GetStringSlice("use_channel")...)
	})

	request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"gpt-6-astra","stream":true,"messages":[{"role":"user","content":"hi"}]}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	body := response.Body.String()
	assert.Equal(t, http.StatusOK, response.Code, body)
	assert.Contains(t, body, "ok-from-b")
	assert.Contains(t, body, "[DONE]")
	assert.NotContains(t, body, "exceeded token rate limit")
	assert.NotContains(t, body, "chatcmpl-a")
	assert.NotContains(t, body, "prompt_filter_results")
	mu.Lock()
	actualCalls := append([]string(nil), calls...)
	mu.Unlock()
	assert.Equal(t, []string{"A", "B"}, actualCalls)
	assert.Equal(t, []string{"501", "502"}, usedChannels)
}
