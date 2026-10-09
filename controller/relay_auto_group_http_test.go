package controller

import (
	"fmt"
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

func TestRelayHTTPAutoRetrySkipsFailedChannelSharedByNextGroup(t *testing.T) {
	gin.SetMode(gin.TestMode)
	oldDB, oldLogDB, oldCache, oldRetryTimes, oldCountToken, oldRedis := model.DB, model.LOG_DB, common.MemoryCacheEnabled, common.RetryTimes, constant.CountToken, common.RedisEnabled
	oldAutoGroups := setting.AutoGroups2JsonString()
	oldUsableGroups := setting.UserUsableGroups2JSONString()
	oldGroupRatios := ratio_setting.GroupRatio2JSONString()
	oldModelPrices := ratio_setting.ModelPrice2JSONString()
	oldFreePreConsume := operation_setting.GetQuotaSetting().EnableFreeModelPreConsume
	t.Cleanup(func() {
		model.DB = oldDB
		model.LOG_DB = oldLogDB
		common.MemoryCacheEnabled = oldCache
		common.RedisEnabled = oldRedis
		common.RetryTimes = oldRetryTimes
		constant.CountToken = oldCountToken
		require.NoError(t, setting.UpdateAutoGroupsByJsonString(oldAutoGroups))
		require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(oldUsableGroups))
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(oldGroupRatios))
		require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(oldModelPrices))
		operation_setting.GetQuotaSetting().EnableFreeModelPreConsume = oldFreePreConsume
	})

	var mu sync.Mutex
	calls := make([]string, 0, 2)
	upstreamA := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		calls = append(calls, "A")
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":{"message":"A failed","type":"upstream_error"}}`))
	}))
	defer upstreamA.Close()
	upstreamB := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		calls = append(calls, "B")
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"chatcmpl-ok","object":"chat.completion","created":1,"model":"test-model","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`))
	}))
	defer upstreamB.Close()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Ability{}, &model.Channel{}, &model.User{}, &model.Log{}, &model.ChannelPurityGroup{}, &model.ChannelPurityMember{}))
	model.DB = db
	model.LOG_DB = db
	common.MemoryCacheEnabled = true
	common.RedisEnabled = false
	common.RetryTimes = 3
	constant.CountToken = false
	require.NoError(t, setting.UpdateAutoGroupsByJsonString(`["group-a","group-b"]`))
	require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{"group-a":"A","group-b":"B"}`))
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"group-a":0,"group-b":0}`))
	require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(`{"test-model":0}`))
	operation_setting.GetQuotaSetting().EnableFreeModelPreConsume = false
	priority, weight := int64(10), uint(1)
	baseA, baseB := upstreamA.URL, upstreamB.URL
	require.NoError(t, db.Create(&model.User{Id: 1, Username: "auto-http-user", Quota: 1000000, Status: common.UserStatusEnabled}).Error)
	require.NoError(t, db.Create(&[]model.Channel{
		{Id: 461, Name: "shared-a", Key: "key-a", BaseURL: &baseA, Group: "group-a,group-b", Models: "test-model", Status: common.ChannelStatusEnabled, Priority: &priority, Weight: &weight},
		{Id: 464, Name: "only-b", Key: "key-b", BaseURL: &baseB, Group: "group-b", Models: "test-model", Status: common.ChannelStatusEnabled, Priority: &priority, Weight: common.GetPointer(uint(0))},
	}).Error)
	require.NoError(t, db.Create(&[]model.Ability{
		{Group: "group-a", Model: "test-model", ChannelId: 461, Enabled: true, Priority: &priority, Weight: 1},
		{Group: "group-b", Model: "test-model", ChannelId: 461, Enabled: true, Priority: &priority, Weight: 100},
		{Group: "group-b", Model: "test-model", ChannelId: 464, Enabled: true, Priority: &priority, Weight: 0},
	}).Error)
	model.InitChannelCache()
	service.InitHttpClient()

	var usedChannels []string
	router := gin.New()
	router.POST("/v1/chat/completions", func(c *gin.Context) {
		common.SetContextKey(c, constant.ContextKeyUsingGroup, "auto")
		common.SetContextKey(c, constant.ContextKeyTokenGroup, "auto")
		common.SetContextKey(c, constant.ContextKeyUserGroup, "")
		common.SetContextKey(c, constant.ContextKeyTokenCrossGroupRetry, true)
		common.SetContextKey(c, constant.ContextKeyUserSetting, dto.UserSetting{AcceptUnsetRatioModel: true})
		common.SetContextKey(c, constant.ContextKeyUserId, 1)
		common.SetContextKey(c, constant.ContextKeyUserQuota, 1000000)
		common.SetContextKey(c, common.RequestIdKey, "auto-http-overlap")
		c.Next()
	}, middleware.Distribute(), func(c *gin.Context) {
		Relay(c, types.RelayFormatOpenAI)
		usedChannels = append([]string(nil), c.GetStringSlice("use_channel")...)
	})

	request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"test-model","messages":[{"role":"user","content":"hi"}]}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	assert.Equal(t, http.StatusOK, response.Code, response.Body.String())
	assert.Contains(t, response.Body.String(), `"content":"ok"`)
	mu.Lock()
	actualCalls := append([]string(nil), calls...)
	mu.Unlock()
	assert.Equal(t, []string{"A", "B"}, actualCalls, fmt.Sprintf("calls=%v used_channels=%v", actualCalls, usedChannels))
	assert.Equal(t, []string{"461", "464"}, usedChannels)
}
