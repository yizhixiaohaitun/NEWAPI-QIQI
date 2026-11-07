package controller

import (
	"errors"
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"net/http/httptest"
	"testing"
)

func TestProcessChannelErrorRemovesOnlyRequestModel(t *testing.T) {
	oldDB, oldCache := model.DB, common.MemoryCacheEnabled
	oldEnabled, oldKeywords := operation_setting.AutomaticRemoveModelEnabled(), operation_setting.AutomaticRemoveModelKeywordsToString()
	t.Cleanup(func() {
		model.DB = oldDB
		common.MemoryCacheEnabled = oldCache
		operation_setting.SetAutomaticRemoveModelEnabled(oldEnabled)
		operation_setting.AutomaticRemoveModelKeywordsFromString(oldKeywords)
	})
	db, e := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, e)
	require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.Ability{}))
	model.DB = db
	common.MemoryCacheEnabled = false
	ch := model.Channel{Id: 33, Group: "default", Models: "alias,other", Status: common.ChannelStatusEnabled}
	require.NoError(t, db.Create(&ch).Error)
	require.NoError(t, ch.AddAbilities(nil))
	operation_setting.SetAutomaticRemoveModelEnabled(true)
	operation_setting.AutomaticRemoveModelKeywordsFromString("This Model is marked by provider as Legacy")
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/messages", nil)
	message := "Bedrock 404: This Model is marked by provider as Legacy"
	apiErr := types.NewOpenAIError(errors.New(message), types.ErrorCodeBadResponseStatusCode, 404)
	processChannelError(c, *types.NewChannelError(33, 0, "test", false, "", true), apiErr, "alias")
	require.Contains(t, apiErr.Error(), message)
	require.NoError(t, db.First(&ch, 33).Error)
	require.Equal(t, "other", ch.Models)
	require.Equal(t, common.ChannelStatusEnabled, ch.Status)
}
