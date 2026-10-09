package service

import (
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestRetryExcludesFailedChannelForFixedGroup(t *testing.T) {
	for _, memoryCache := range []bool{true} {
		t.Run("cache", func(t *testing.T) {
			oldDB, oldCache := model.DB, common.MemoryCacheEnabled
			t.Cleanup(func() {
				model.DB = oldDB
				common.MemoryCacheEnabled = oldCache
			})

			db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
			require.NoError(t, err)
			require.NoError(t, db.AutoMigrate(&model.Ability{}, &model.Channel{}))
			model.DB = db
			common.MemoryCacheEnabled = memoryCache
			p10, p5, p0, weight := int64(10), int64(5), int64(0), uint(1)
			require.NoError(t, db.Create(&[]model.Channel{
				{Id: 461, Group: "fixed", Models: "test-model", Status: common.ChannelStatusEnabled, Priority: &p10, Weight: &weight},
				{Id: 464, Group: "fixed", Models: "test-model", Status: common.ChannelStatusEnabled, Priority: &p5, Weight: &weight},
				{Id: 465, Group: "fixed", Models: "test-model", Status: common.ChannelStatusEnabled, Priority: &p0, Weight: &weight},
			}).Error)
			require.NoError(t, db.Create(&[]model.Ability{
				{Group: "fixed", Model: "test-model", ChannelId: 461, Enabled: true, Priority: &p10, Weight: 1},
				{Group: "fixed", Model: "test-model", ChannelId: 464, Enabled: true, Priority: &p5, Weight: 1},
				{Group: "fixed", Model: "test-model", ChannelId: 465, Enabled: true, Priority: &p0, Weight: 1},
			}).Error)
			model.InitChannelCache()

			ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
			retry := 0
			param := &RetryParam{Ctx: ctx, TokenGroup: "fixed", ModelName: "test-model", Retry: &retry}
			first, _, err := CacheGetRandomSatisfiedChannel(param)
			require.NoError(t, err)
			require.NotNil(t, first)
			assert.Equal(t, 461, first.Id)

			MarkChannelFailed(ctx, first.Id)
			require.True(t, param.IncreaseRetry())
			second, _, err := CacheGetRandomSatisfiedChannel(param)
			require.NoError(t, err)
			require.NotNil(t, second)
			assert.Equal(t, 464, second.Id, "retry must choose the highest-priority unfailed channel, not skip to priority zero")

			MarkChannelFailed(ctx, second.Id)
			require.True(t, param.IncreaseRetry())
			third, _, err := CacheGetRandomSatisfiedChannel(param)
			require.NoError(t, err)
			require.NotNil(t, third)
			assert.Equal(t, 465, third.Id)

			MarkChannelFailed(ctx, third.Id)
			require.True(t, param.IncreaseRetry())
			exhausted, _, err := CacheGetRandomSatisfiedChannel(param)
			require.NoError(t, err)
			assert.Nil(t, exhausted)
		})
	}
}
