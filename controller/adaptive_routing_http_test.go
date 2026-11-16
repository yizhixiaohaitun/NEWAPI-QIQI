package controller

import (
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
	"github.com/stretchr/testify/require"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestAdaptiveRoutingRealRelayHTTP(t *testing.T) {
	oldEnabled := operation_setting.GetAdaptiveRoutingSetting().Enabled
	operation_setting.GetAdaptiveRoutingSetting().Enabled = true
	t.Cleanup(func() { operation_setting.GetAdaptiveRoutingSetting().Enabled = oldEnabled })
	oldDB, oldLogDB := model.DB, model.LOG_DB
	oldMain, oldLogType := common.MainDatabaseType(), common.LogDatabaseType()
	oldCache, oldRedis, oldBatch := common.MemoryCacheEnabled, common.RedisEnabled, common.BatchUpdateEnabled
	oldRetry, oldCount, oldLog, oldError := common.RetryTimes, constant.CountToken, common.LogConsumeEnabled, constant.ErrorLogEnabled
	oldFirst := constant.FirstByteTimeout
	oldRatios, oldPrices := ratio_setting.GroupRatio2JSONString(), ratio_setting.ModelPrice2JSONString()
	oldFree := operation_setting.GetQuotaSetting().EnableFreeModelPreConsume
	oldAuto := model_setting.GetGlobalSettings().AutoChatToResponsesEnabled
	t.Cleanup(func() {
		model.DB, model.LOG_DB = oldDB, oldLogDB
		common.SetDatabaseTypes(oldMain, oldLogType)
		common.MemoryCacheEnabled, common.RedisEnabled, common.BatchUpdateEnabled = oldCache, oldRedis, oldBatch
		common.RetryTimes, constant.CountToken, common.LogConsumeEnabled, constant.ErrorLogEnabled = oldRetry, oldCount, oldLog, oldError
		constant.FirstByteTimeout = oldFirst
		operation_setting.GetQuotaSetting().EnableFreeModelPreConsume = oldFree
		model_setting.GetGlobalSettings().AutoChatToResponsesEnabled = oldAuto
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(oldRatios))
		require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(oldPrices))
	})
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	db := openRequestCompatibilitySQLite(t, ":memory:")
	require.NoError(t, db.AutoMigrate(&model.Ability{}, &model.Channel{}, &model.User{}, &model.Token{}, &model.Log{}, &model.ChannelPurityGroup{}, &model.ChannelPurityMember{}))
	model.DB, model.LOG_DB = db, db
	common.MemoryCacheEnabled, common.RedisEnabled, common.BatchUpdateEnabled = false, false, false
	common.RetryTimes, constant.CountToken, common.LogConsumeEnabled, constant.ErrorLogEnabled = 0, false, true, true
	constant.FirstByteTimeout = 0
	operation_setting.GetQuotaSetting().EnableFreeModelPreConsume = false
	model_setting.GetGlobalSettings().AutoChatToResponsesEnabled = false
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"adaptive-http-group":1}`))
	require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(`{"adaptive-http-model":0}`))
	require.NoError(t, db.Create(&model.User{Id: 981, Username: "adaptive-http-user", Quota: 1000000, Status: common.UserStatusEnabled}).Error)
	require.NoError(t, db.Create(&model.Token{Id: 982, UserId: 981, Key: "adaptive-http-key", RemainQuota: 1000000, Status: common.TokenStatusEnabled}).Error)

	var failing atomic.Bool
	failing.Store(true)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		w.Header().Set("Content-Type", "application/json")
		if failing.Load() {
			w.WriteHeader(503)
			_, _ = io.WriteString(w, `{"error":{"message":"busy","type":"server_error"}}`)
			return
		}
		_, _ = io.WriteString(w, `{"id":"ok","object":"chat.completion","created":1,"model":"adaptive-http-model","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`)
	}))
	defer upstream.Close()
	base, priority, weight := upstream.URL, int64(1), uint(1)
	require.NoError(t, db.Create(&model.Channel{Id: 1983, Type: constant.ChannelTypeOpenAI, Name: "adaptive-http", Key: "key", BaseURL: &base, Group: "adaptive-http-group", Models: "adaptive-http-model", Status: common.ChannelStatusEnabled, Priority: &priority, Weight: &weight}).Error)
	require.NoError(t, db.Create(&model.Ability{Group: "adaptive-http-group", Model: "adaptive-http-model", ChannelId: 1983, Enabled: true, Priority: &priority, Weight: 1}).Error)
	service.InitHttpClient()
	router := gin.New()
	router.POST("/v1/chat/completions", func(c *gin.Context) {
		common.SetContextKey(c, constant.ContextKeyUsingGroup, "adaptive-http-group")
		common.SetContextKey(c, constant.ContextKeyTokenGroup, "adaptive-http-group")
		common.SetContextKey(c, constant.ContextKeyUserSetting, dto.UserSetting{BillingPreference: "wallet_only", AcceptUnsetRatioModel: true})
		common.SetContextKey(c, constant.ContextKeyUserId, 981)
		common.SetContextKey(c, constant.ContextKeyUserQuota, 1000000)
		common.SetContextKey(c, constant.ContextKeyTokenId, 982)
		common.SetContextKey(c, constant.ContextKeyTokenKey, "adaptive-http-key")
		c.Next()
	}, middleware.Distribute(), func(c *gin.Context) { Relay(c, types.RelayFormatOpenAI) })
	request := func() int {
		req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"adaptive-http-model","messages":[{"role":"user","content":"hi"}]}`))
		req.Header.Set("Content-Type", "application/json")
		out := httptest.NewRecorder()
		router.ServeHTTP(out, req)
		return out.Code
	}
	for i := 0; i < 5; i++ {
		require.NotEqual(t, 200, request())
	}
	require.InDelta(t, 0.05, model.AdaptiveRoutingMultiplier(1983, "adaptive-http-model"), 0.0001)
	require.Equal(t, 1.0, model.AdaptiveRoutingMultiplier(1983, "another-model"))
	failing.Store(false)
	for i := 0; i < 5; i++ {
		require.Equal(t, 200, request())
	}
	require.InDelta(t, 0.5, model.AdaptiveRoutingMultiplier(1983, "adaptive-http-model"), 0.0001)
	for i := 0; i < 15; i++ {
		require.Equal(t, 200, request())
	}
	require.Equal(t, 1.0, model.AdaptiveRoutingMultiplier(1983, "adaptive-http-model"))
	// Build a proxy client failure before any HTTP transport. Five local
	// failures must not become channel-health samples.
	require.NoError(t, db.Model(&model.Channel{}).Where("id = ?", 1983).Update("setting", `{"proxy":"invalid-proxy://bad"}`).Error)
	for i := 0; i < 20; i++ {
		require.NotEqual(t, 200, request())
	}
	require.Equal(t, 1.0, model.AdaptiveRoutingMultiplier(1983, "adaptive-http-model"))
}
