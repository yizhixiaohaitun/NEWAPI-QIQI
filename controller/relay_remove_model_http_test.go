package controller

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
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
	"github.com/stretchr/testify/require"
)

func TestRelayHTTPRemoveMappedRequestModel(t *testing.T) {
	oldDB, oldLogDB := model.DB, model.LOG_DB
	oldMain, oldLogType := common.MainDatabaseType(), common.LogDatabaseType()
	oldCache, oldRedis, oldBatch := common.MemoryCacheEnabled, common.RedisEnabled, common.BatchUpdateEnabled
	oldRetry, oldCount, oldLog := common.RetryTimes, constant.CountToken, common.LogConsumeEnabled
	oldErrorLog, oldDisable := constant.ErrorLogEnabled, common.AutomaticDisableChannelEnabled
	oldEnabled, oldKeywords := operation_setting.AutomaticRemoveModelEnabled(), operation_setting.AutomaticRemoveModelKeywordsToString()
	oldRatios, oldPrices := ratio_setting.GroupRatio2JSONString(), ratio_setting.ModelPrice2JSONString()
	oldFree, oldPing := operation_setting.GetQuotaSetting().EnableFreeModelPreConsume, operation_setting.GetGeneralSetting().PingIntervalEnabled
	t.Cleanup(func() {
		model.DB, model.LOG_DB = oldDB, oldLogDB
		common.SetDatabaseTypes(oldMain, oldLogType)
		common.MemoryCacheEnabled, common.RedisEnabled, common.BatchUpdateEnabled = oldCache, oldRedis, oldBatch
		common.RetryTimes, constant.CountToken, common.LogConsumeEnabled = oldRetry, oldCount, oldLog
		constant.ErrorLogEnabled, common.AutomaticDisableChannelEnabled = oldErrorLog, oldDisable
		operation_setting.SetAutomaticRemoveModelEnabled(oldEnabled)
		operation_setting.AutomaticRemoveModelKeywordsFromString(oldKeywords)
		_ = ratio_setting.UpdateGroupRatioByJSONString(oldRatios)
		_ = ratio_setting.UpdateModelPriceByJSONString(oldPrices)
		operation_setting.GetQuotaSetting().EnableFreeModelPreConsume = oldFree
		operation_setting.GetGeneralSetting().PingIntervalEnabled = oldPing
	})
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	db := openRequestCompatibilitySQLite(t, ":memory:")
	require.NoError(t, db.AutoMigrate(&model.Ability{}, &model.Channel{}, &model.User{}, &model.Token{}, &model.Log{}, &model.ChannelPurityGroup{}, &model.ChannelPurityMember{}))
	model.DB, model.LOG_DB = db, db
	common.MemoryCacheEnabled, common.RedisEnabled, common.BatchUpdateEnabled = true, false, false
	common.RetryTimes, constant.CountToken, common.LogConsumeEnabled = 0, false, false
	constant.ErrorLogEnabled, common.AutomaticDisableChannelEnabled = false, true
	operation_setting.SetAutomaticRemoveModelEnabled(true)
	operation_setting.AutomaticRemoveModelKeywordsFromString("This Model is marked by provider as Legacy")
	operation_setting.GetQuotaSetting().EnableFreeModelPreConsume = false
	operation_setting.GetGeneralSetting().PingIntervalEnabled = false
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"local":1}`))
	require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(`{"request-alias":0}`))
	require.NoError(t, db.Create(&model.User{Id: 891, Username: "local-test", Quota: 1000000, Status: common.UserStatusEnabled}).Error)
	require.NoError(t, db.Create(&model.Token{Id: 892, UserId: 891, Key: "local-test", RemainQuota: 1000000, Status: common.TokenStatusEnabled}).Error)
	received := make(chan string, 1)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		received <- string(b)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(404)
		_, _ = io.WriteString(w, `{"error":{"message":"This Model is marked by provider as Legacy","type":"invalid_request_error"}}`)
	}))
	defer upstream.Close()
	base, mapping := upstream.URL, `{"request-alias":"mapped-model","other-alias":"mapped-model"}`
	channel := model.Channel{Id: 33, Type: constant.ChannelTypeOpenAI, Name: "local", Key: "local", BaseURL: &base, ModelMapping: &mapping, Group: "local", Models: "request-alias,mapped-model,other-alias", Status: common.ChannelStatusEnabled}
	require.NoError(t, db.Create(&channel).Error)
	require.NoError(t, channel.AddAbilities(nil))
	model.InitChannelCache()
	service.InitHttpClient()
	router := gin.New()
	router.POST("/v1/chat/completions", func(c *gin.Context) {
		common.SetContextKey(c, constant.ContextKeyUsingGroup, "local")
		common.SetContextKey(c, constant.ContextKeyTokenGroup, "local")
		common.SetContextKey(c, constant.ContextKeyUserSetting, dto.UserSetting{BillingPreference: "wallet_only", AcceptUnsetRatioModel: true})
		common.SetContextKey(c, constant.ContextKeyUserId, 891)
		common.SetContextKey(c, constant.ContextKeyUserQuota, 1000000)
		common.SetContextKey(c, constant.ContextKeyTokenId, 892)
		common.SetContextKey(c, constant.ContextKeyTokenKey, "local-test")
		common.SetContextKey(c, common.RequestIdKey, "local-test")
		c.Next()
	}, middleware.Distribute(), func(c *gin.Context) { Relay(c, types.RelayFormatOpenAI) })
	gateway := httptest.NewServer(router)
	defer gateway.Close()
	response, err := gateway.Client().Post(gateway.URL+"/v1/chat/completions", "application/json", strings.NewReader(`{"model":"request-alias","messages":[{"role":"user","content":"hi"}]}`))
	require.NoError(t, err)
	body, err := io.ReadAll(response.Body)
	response.Body.Close()
	require.NoError(t, err)
	require.Equal(t, 404, response.StatusCode, string(body))
	require.Contains(t, string(body), "This Model is marked by provider as Legacy")
	require.Contains(t, <-received, `"model":"mapped-model"`)
	require.NoError(t, db.First(&channel, 33).Error)
	require.Equal(t, "mapped-model,other-alias", channel.Models)
	require.Equal(t, common.ChannelStatusEnabled, channel.Status)
	var abilities []model.Ability
	require.NoError(t, db.Where("channel_id = ?", 33).Find(&abilities).Error)
	require.Len(t, abilities, 2)
	model.InitChannelCache()
	selected, err := model.GetRandomSatisfiedChannel("local", "request-alias", 0, "")
	require.NoError(t, err)
	require.Nil(t, selected)
}
