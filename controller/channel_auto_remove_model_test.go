package controller

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/stretchr/testify/require"
)

func TestAutomaticChannelTestRemovesRequestedModelOnlyWhenAllowed(t *testing.T) {
	oldDB, oldLogDB := model.DB, model.LOG_DB
	oldMain, oldLogType := common.MainDatabaseType(), common.LogDatabaseType()
	oldCache, oldRedis := common.MemoryCacheEnabled, common.RedisEnabled
	oldDisable, oldErrorLog := common.AutomaticDisableChannelEnabled, constant.ErrorLogEnabled
	oldEnabled, oldKeywords := operation_setting.AutomaticRemoveModelEnabled(), operation_setting.AutomaticRemoveModelKeywordsToString()
	oldRatios, oldPrices := ratio_setting.GroupRatio2JSONString(), ratio_setting.ModelPrice2JSONString()
	t.Cleanup(func() {
		model.DB, model.LOG_DB = oldDB, oldLogDB
		common.SetDatabaseTypes(oldMain, oldLogType)
		common.MemoryCacheEnabled, common.RedisEnabled = oldCache, oldRedis
		common.AutomaticDisableChannelEnabled, constant.ErrorLogEnabled = oldDisable, oldErrorLog
		operation_setting.SetAutomaticRemoveModelEnabled(oldEnabled)
		operation_setting.AutomaticRemoveModelKeywordsFromString(oldKeywords)
		_ = ratio_setting.UpdateGroupRatioByJSONString(oldRatios)
		_ = ratio_setting.UpdateModelPriceByJSONString(oldPrices)
	})
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	db := openRequestCompatibilitySQLite(t, ":memory:")
	require.NoError(t, db.AutoMigrate(&model.Ability{}, &model.Channel{}, &model.User{}))
	model.DB, model.LOG_DB = db, db
	common.MemoryCacheEnabled, common.RedisEnabled = false, false
	common.AutomaticDisableChannelEnabled, constant.ErrorLogEnabled = false, false
	operation_setting.SetAutomaticRemoveModelEnabled(true)
	operation_setting.AutomaticRemoveModelKeywordsFromString("provider legacy")
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"local":1}`))
	require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(`{"request-alias":0}`))
	require.NoError(t, db.Create(&model.User{Id: 891, Username: "auto-test", Group: "local", Quota: 1000000, Status: common.UserStatusEnabled}).Error)
	received := make(chan string, 2)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		received <- string(body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, `{"error":{"message":"provider legacy","type":"invalid_request_error"}}`)
	}))
	defer upstream.Close()
	service.InitHttpClient()
	base, mapping, testModel := upstream.URL, `{"request-alias":"mapped-model","other-alias":"mapped-model"}`, "request-alias"
	channel := model.Channel{Id: 33, Type: constant.ChannelTypeOpenAI, Name: "local", Key: "local", BaseURL: &base, ModelMapping: &mapping, TestModel: &testModel, Group: "local", Models: "other-alias,request-alias,mapped-model", Status: common.ChannelStatusEnabled}
	require.NoError(t, db.Create(&channel).Error)
	require.NoError(t, channel.AddAbilities(nil))
	summary := performChannelTests(context.Background(), []*model.Channel{&channel}, 891, false, nil)
	require.Equal(t, 1, summary.Failed)
	require.Contains(t, <-received, `"model":"mapped-model"`)
	require.NoError(t, db.First(&channel, 33).Error)
	require.Equal(t, "other-alias,request-alias,mapped-model", channel.Models)
	summary = performChannelTests(context.Background(), []*model.Channel{&channel}, 891, true, nil)
	require.Equal(t, 1, summary.Failed)
	require.Equal(t, 0, summary.Disabled)
	require.Contains(t, <-received, `"model":"mapped-model"`)
	require.NoError(t, db.First(&channel, 33).Error)
	require.Equal(t, "other-alias,mapped-model", channel.Models)
	require.Equal(t, common.ChannelStatusEnabled, channel.Status)
	var abilities []model.Ability
	require.NoError(t, db.Where("channel_id = ?", 33).Find(&abilities).Error)
	require.Len(t, abilities, 2)
}
