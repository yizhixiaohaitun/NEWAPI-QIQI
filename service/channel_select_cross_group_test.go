package service

import (
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestAutoCrossGroupRetryPrefersNextUntriedGroup(t *testing.T) {
	gin.SetMode(gin.TestMode)
	oldDB, oldCache, oldRetryTimes := model.DB, common.MemoryCacheEnabled, common.RetryTimes
	oldAutoGroups := setting.AutoGroups2JsonString()
	oldUsableGroups := setting.UserUsableGroups2JSONString()
	t.Cleanup(func() {
		model.DB = oldDB
		common.MemoryCacheEnabled = oldCache
		common.RetryTimes = oldRetryTimes
		require.NoError(t, setting.UpdateAutoGroupsByJsonString(oldAutoGroups))
		require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(oldUsableGroups))
	})

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Ability{}, &model.Channel{}))
	model.DB = db
	common.MemoryCacheEnabled = true
	common.RetryTimes = 3
	require.NoError(t, setting.UpdateAutoGroupsByJsonString(`["default","empty","vip","backup","vip"]`))
	require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{"default":"Default","empty":"Empty","vip":"VIP","backup":"Backup"}`))

	priority := int64(10)
	weight := uint(1)
	require.NoError(t, db.Create(&[]model.Channel{
		{Id: 101, Name: "default-channel", Group: "default", Models: "test-model", Status: common.ChannelStatusEnabled, Priority: &priority, Weight: &weight},
		{Id: 202, Name: "vip-channel", Group: "vip", Models: "test-model", Status: common.ChannelStatusEnabled, Priority: &priority, Weight: &weight},
		{Id: 303, Name: "backup-channel", Group: "backup", Models: "test-model", Status: common.ChannelStatusEnabled, Priority: &priority, Weight: &weight},
	}).Error)
	require.NoError(t, db.Create(&[]model.Ability{
		{Group: "default", Model: "test-model", ChannelId: 101, Enabled: true, Priority: &priority, Weight: 1},
		{Group: "vip", Model: "test-model", ChannelId: 202, Enabled: true, Priority: &priority, Weight: 1},
		{Group: "backup", Model: "test-model", ChannelId: 303, Enabled: true, Priority: &priority, Weight: 1},
	}).Error)
	model.InitChannelCache()

	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	common.SetContextKey(ctx, constant.ContextKeyTokenCrossGroupRetry, true)
	retry := 0
	param := &RetryParam{Ctx: ctx, TokenGroup: "auto", ModelName: "test-model", Retry: &retry}

	first, firstGroup, err := CacheGetRandomSatisfiedChannel(param)
	require.NoError(t, err)
	require.NotNil(t, first)
	assert.Equal(t, 101, first.Id)
	assert.Equal(t, "default", firstGroup)

	// Distributor and controller share the request context and both select at
	// Retry=0. Repeating that initial selection must not consume another group.
	initialAgain, initialAgainGroup, err := CacheGetRandomSatisfiedChannel(param)
	require.NoError(t, err)
	require.NotNil(t, initialAgain)
	assert.Equal(t, 101, initialAgain.Id)
	assert.Equal(t, "default", initialAgainGroup)

	require.True(t, param.IncreaseRetry())
	second, secondGroup, err := CacheGetRandomSatisfiedChannel(param)
	require.NoError(t, err)
	require.NotNil(t, second)
	assert.Equal(t, 202, second.Id)
	assert.Equal(t, "vip", secondGroup)

	require.True(t, param.IncreaseRetry())
	third, thirdGroup, err := CacheGetRandomSatisfiedChannel(param)
	require.NoError(t, err)
	require.NotNil(t, third)
	assert.Equal(t, 303, third.Id)
	assert.Equal(t, "backup", thirdGroup)

	require.True(t, param.IncreaseRetry())
	exhausted, _, err := CacheGetRandomSatisfiedChannel(param)
	require.NoError(t, err)
	assert.Nil(t, exhausted, "duplicate configured groups must not wrap retries back to a failed group")
}

func TestAutoRetryBoundaries(t *testing.T) {
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
	require.NoError(t, setting.UpdateAutoGroupsByJsonString(`["default","vip","backup"]`))
	require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{"default":"Default","vip":"VIP","backup":"Backup"}`))
	priority, weight := int64(10), uint(1)
	require.NoError(t, db.Create(&[]model.Channel{
		{Id: 101, Group: "default", Models: "test-model", Status: common.ChannelStatusEnabled, Priority: &priority, Weight: &weight},
		{Id: 202, Group: "vip", Models: "test-model", Status: common.ChannelStatusEnabled, Priority: &priority, Weight: &weight},
		{Id: 303, Group: "backup", Models: "test-model", Status: common.ChannelStatusEnabled, Priority: &priority, Weight: &weight},
	}).Error)
	require.NoError(t, db.Create(&[]model.Ability{
		{Group: "default", Model: "test-model", ChannelId: 101, Enabled: true, Priority: &priority, Weight: 1},
		{Group: "vip", Model: "test-model", ChannelId: 202, Enabled: true, Priority: &priority, Weight: 1},
		{Group: "backup", Model: "test-model", ChannelId: 303, Enabled: true, Priority: &priority, Weight: 1},
	}).Error)
	model.InitChannelCache()

	t.Run("affinity hit in middle continues forward", func(t *testing.T) {
		ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
		common.SetContextKey(ctx, constant.ContextKeyTokenCrossGroupRetry, true)
		common.SetContextKey(ctx, constant.ContextKeyAutoGroupIndex, 1)
		retry := 0
		param := &RetryParam{Ctx: ctx, TokenGroup: "auto", ModelName: "test-model", Retry: &retry}
		first, group, err := CacheGetRandomSatisfiedChannel(param)
		require.NoError(t, err)
		require.NotNil(t, first)
		assert.Equal(t, 202, first.Id)
		assert.Equal(t, "vip", group)
		require.True(t, param.IncreaseRetry())
		second, group, err := CacheGetRandomSatisfiedChannel(param)
		require.NoError(t, err)
		require.NotNil(t, second)
		assert.Equal(t, 101, second.Id)
		assert.Equal(t, "default", group)
		require.True(t, param.IncreaseRetry())
		third, group, err := CacheGetRandomSatisfiedChannel(param)
		require.NoError(t, err)
		require.NotNil(t, third)
		assert.Equal(t, 303, third.Id)
		assert.Equal(t, "backup", group)
	})

	t.Run("affinity hit at end still tries earlier groups without repeats", func(t *testing.T) {
		ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
		common.SetContextKey(ctx, constant.ContextKeyTokenCrossGroupRetry, true)
		common.SetContextKey(ctx, constant.ContextKeyAutoGroupIndex, 2)
		common.SetContextKey(ctx, constant.ContextKeyAutoGroupVisited, map[string]bool{"backup": true})
		retry := 0
		param := &RetryParam{Ctx: ctx, TokenGroup: "auto", ModelName: "test-model", Retry: &retry}
		initial, group, err := CacheGetRandomSatisfiedChannel(param)
		require.NoError(t, err)
		require.NotNil(t, initial)
		assert.Equal(t, 303, initial.Id)
		assert.Equal(t, "backup", group)
		require.True(t, param.IncreaseRetry())
		firstRetry, group, err := CacheGetRandomSatisfiedChannel(param)
		require.NoError(t, err)
		require.NotNil(t, firstRetry)
		assert.Equal(t, 101, firstRetry.Id)
		assert.Equal(t, "default", group)
		require.True(t, param.IncreaseRetry())
		secondRetry, group, err := CacheGetRandomSatisfiedChannel(param)
		require.NoError(t, err)
		require.NotNil(t, secondRetry)
		assert.Equal(t, 202, secondRetry.Id)
		assert.Equal(t, "vip", group)
		require.True(t, param.IncreaseRetry())
		exhausted, _, err := CacheGetRandomSatisfiedChannel(param)
		require.NoError(t, err)
		assert.Nil(t, exhausted)
	})

	t.Run("cross group disabled preserves priority retry", func(t *testing.T) {
		ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
		retry := 1
		channel, group, err := CacheGetRandomSatisfiedChannel(&RetryParam{Ctx: ctx, TokenGroup: "auto", ModelName: "test-model", Retry: &retry})
		require.NoError(t, err)
		require.NotNil(t, channel)
		assert.Equal(t, 101, channel.Id)
		assert.Equal(t, "default", group)
	})

	t.Run("specified group never crosses", func(t *testing.T) {
		ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
		common.SetContextKey(ctx, constant.ContextKeyTokenCrossGroupRetry, true)
		retry := 2
		channel, group, err := CacheGetRandomSatisfiedChannel(&RetryParam{Ctx: ctx, TokenGroup: "vip", ModelName: "test-model", Retry: &retry})
		require.NoError(t, err)
		require.NotNil(t, channel)
		assert.Equal(t, 202, channel.Id)
		assert.Equal(t, "vip", group)
	})
}
