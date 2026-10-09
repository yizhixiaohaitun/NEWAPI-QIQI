package controller

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestGetChannelAutoRetryUsesEachConfiguredGroupOnce(t *testing.T) {
	gin.SetMode(gin.TestMode)
	oldDB, oldCache := model.DB, common.MemoryCacheEnabled
	oldAutoGroups := setting.AutoGroups2JsonString()
	oldUsableGroups := setting.UserUsableGroups2JSONString()
	t.Cleanup(func() {
		model.DB = oldDB
		common.MemoryCacheEnabled = oldCache
		require.NoError(t, setting.UpdateAutoGroupsByJsonString(oldAutoGroups))
		require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(oldUsableGroups))
	})

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Ability{}, &model.Channel{}))
	model.DB = db
	common.MemoryCacheEnabled = true
	require.NoError(t, setting.UpdateAutoGroupsByJsonString(`["default","vip"]`))
	require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{"default":"Default","vip":"VIP"}`))
	priority, weight := int64(10), uint(1)
	require.NoError(t, db.Create(&[]model.Channel{
		{Id: 101, Name: "default", Key: "key-a", Group: "default", Models: "test-model", Status: common.ChannelStatusEnabled, Priority: &priority, Weight: &weight},
		{Id: 202, Name: "vip", Key: "key-b", Group: "vip", Models: "test-model", Status: common.ChannelStatusEnabled, Priority: &priority, Weight: &weight},
	}).Error)
	require.NoError(t, db.Create(&[]model.Ability{
		{Group: "default", Model: "test-model", ChannelId: 101, Enabled: true, Priority: &priority, Weight: 1},
		{Group: "vip", Model: "test-model", ChannelId: 202, Enabled: true, Priority: &priority, Weight: 1},
	}).Error)
	model.InitChannelCache()

	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{}`))
	common.SetContextKey(ctx, constant.ContextKeyTokenCrossGroupRetry, true)
	common.SetContextKey(ctx, constant.ContextKeyUsingGroup, "auto")
	info := &relaycommon.RelayInfo{
		TokenGroup:      "auto",
		OriginModelName: "test-model",
		ChannelMeta:     &relaycommon.ChannelMeta{},
	}
	retry := 0
	param := &service.RetryParam{Ctx: ctx, TokenGroup: "auto", ModelName: "test-model", RequestPath: ctx.Request.URL.Path, Retry: &retry}

	first, apiErr := getChannel(ctx, info, param)
	require.Nil(t, apiErr)
	require.NotNil(t, first)
	assert.Equal(t, 101, first.Id)
	require.True(t, param.IncreaseRetry())
	second, apiErr := getChannel(ctx, info, param)
	require.Nil(t, apiErr)
	require.NotNil(t, second)
	assert.Equal(t, 202, second.Id)
	require.True(t, param.IncreaseRetry())
	exhausted, apiErr := getChannel(ctx, info, param)
	assert.Nil(t, exhausted)
	require.NotNil(t, apiErr)
}
