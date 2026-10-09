package controller

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

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
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

var fixedGroupAffinityTestSequence atomic.Uint64

func TestRelayHTTPFixedGroupAffinityFailureImmediatelyUsesUntriedChannel(t *testing.T) {
	gin.SetMode(gin.TestMode)
	oldDB, oldLogDB, oldCache, oldRetryTimes, oldCountToken, oldRedis := model.DB, model.LOG_DB, common.MemoryCacheEnabled, common.RetryTimes, constant.CountToken, common.RedisEnabled
	oldGroupRatios, oldModelPrices := ratio_setting.GroupRatio2JSONString(), ratio_setting.ModelPrice2JSONString()
	oldFreePreConsume := operation_setting.GetQuotaSetting().EnableFreeModelPreConsume
	affinitySetting := operation_setting.GetChannelAffinitySetting()
	oldRules := affinitySetting.Rules
	t.Cleanup(func() {
		model.DB, model.LOG_DB = oldDB, oldLogDB
		common.MemoryCacheEnabled, common.RedisEnabled = oldCache, oldRedis
		common.RetryTimes, constant.CountToken = oldRetryTimes, oldCountToken
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(oldGroupRatios))
		require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(oldModelPrices))
		operation_setting.GetQuotaSetting().EnableFreeModelPreConsume = oldFreePreConsume
		affinitySetting.Rules = oldRules
	})

	var mu sync.Mutex
	aCalls := 0
	calls := make([]string, 0, 3)
	upstreamA := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		aCalls++
		attempt := aCalls
		calls = append(calls, "A")
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		if attempt > 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"error":{"message":"请求的上游分组无可用渠道","type":"upstream_error"}}`))
			return
		}
		_, _ = w.Write([]byte(`{"id":"warmup","object":"chat.completion","created":1,"model":"test-model","choices":[{"index":0,"message":{"role":"assistant","content":"warm"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`))
	}))
	defer upstreamA.Close()
	upstreamB := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		calls = append(calls, "B")
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"ok","object":"chat.completion","created":1,"model":"test-model","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`))
	}))
	defer upstreamB.Close()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Ability{}, &model.Channel{}, &model.User{}, &model.Log{}, &model.ChannelPurityGroup{}, &model.ChannelPurityMember{}))
	model.DB, model.LOG_DB = db, db
	common.MemoryCacheEnabled, common.RedisEnabled = true, false
	common.RetryTimes, constant.CountToken = 3, false
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"fixed":0}`))
	require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(`{"test-model":0}`))
	operation_setting.GetQuotaSetting().EnableFreeModelPreConsume = false
	affinitySetting.Rules = []operation_setting.ChannelAffinityRule{{
		Name: "fixed-http-affinity", ModelRegex: []string{"^test-model$"}, PathRegex: []string{"^/v1/messages$"},
		KeySources: []operation_setting.ChannelAffinityKeySource{{Type: "request_header", Key: "X-Test-Affinity"}}, TTLSeconds: 60,
	}}
	priority, weight := int64(10), uint(1)
	baseA, baseB := upstreamA.URL, upstreamB.URL
	require.NoError(t, db.Create(&model.User{Id: 1, Username: "fixed-http-user", Quota: 1000000, Status: common.UserStatusEnabled}).Error)
	require.NoError(t, db.Create(&[]model.Channel{
		{Id: 461, Name: "affinity-a", Key: "key-a", BaseURL: &baseA, Group: "fixed", Models: "test-model", Status: common.ChannelStatusEnabled, Priority: &priority, Weight: &weight},
		{Id: 464, Name: "fallback-b", Key: "key-b", BaseURL: &baseB, Group: "fixed", Models: "test-model", Status: common.ChannelStatusEnabled, Priority: &priority, Weight: common.GetPointer(uint(0))},
	}).Error)
	require.NoError(t, db.Create(&[]model.Ability{
		{Group: "fixed", Model: "test-model", ChannelId: 461, Enabled: true, Priority: &priority, Weight: 1},
		{Group: "fixed", Model: "test-model", ChannelId: 464, Enabled: true, Priority: &priority, Weight: 0},
	}).Error)
	model.InitChannelCache()
	service.InitHttpClient()

	var usedChannels []string
	var affinityInfo map[string]interface{}
	router := gin.New()
	router.POST("/v1/messages", func(c *gin.Context) {
		common.SetContextKey(c, constant.ContextKeyUsingGroup, "fixed")
		common.SetContextKey(c, constant.ContextKeyTokenGroup, "fixed")
		common.SetContextKey(c, constant.ContextKeyTokenCrossGroupRetry, false)
		common.SetContextKey(c, constant.ContextKeyUserSetting, dto.UserSetting{AcceptUnsetRatioModel: true})
		common.SetContextKey(c, constant.ContextKeyUserId, 1)
		common.SetContextKey(c, constant.ContextKeyUserQuota, 1000000)
		common.SetContextKey(c, common.RequestIdKey, "fixed-http-affinity")
		c.Next()
	}, middleware.Distribute(), func(c *gin.Context) {
		affinityInfo = make(map[string]interface{})
		service.AppendChannelAffinityAdminInfo(c, affinityInfo)
		Relay(c, types.RelayFormatClaude)
		usedChannels = append([]string(nil), c.GetStringSlice("use_channel")...)
	})

	affinityKey := fmt.Sprintf("same-session-%d", fixedGroupAffinityTestSequence.Add(1))
	doRequest := func() *httptest.ResponseRecorder {
		request := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(`{"model":"test-model","max_tokens":16,"messages":[{"role":"user","content":"hi"}]}`))
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("X-Test-Affinity", affinityKey)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		return response
	}
	warmup := doRequest()
	require.Equal(t, http.StatusOK, warmup.Code, warmup.Body.String())
	mu.Lock()
	require.Equal(t, []string{"A"}, append([]string(nil), calls...), "warmup must select channel 461 before affinity can be asserted")
	calls = calls[:0]
	mu.Unlock()
	usedChannels = nil

	response := doRequest()
	require.Contains(t, affinityInfo, "channel_affinity", "second request must hit middleware affinity, not merely random selection")
	usedAffinity, ok := affinityInfo["channel_affinity"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, 461, usedAffinity["channel_id"])
	assert.Equal(t, "fixed", usedAffinity["selected_group"])
	assert.Equal(t, http.StatusOK, response.Code, response.Body.String())
	assert.Contains(t, response.Body.String(), `"text":"ok"`)
	mu.Lock()
	actualCalls := append([]string(nil), calls...)
	mu.Unlock()
	assert.Equal(t, []string{"A", "B"}, actualCalls)
	assert.Equal(t, []string{"461", "464"}, usedChannels)
}
