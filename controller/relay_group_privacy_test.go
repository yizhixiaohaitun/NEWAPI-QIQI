package controller

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestRetryChannelErrorsDoNotExposeGroup(t *testing.T) {
	oldDB, oldCache, oldWriter := model.DB, common.MemoryCacheEnabled, gin.DefaultErrorWriter
	common.MemoryCacheEnabled = false
	t.Cleanup(func() { model.DB = oldDB; common.MemoryCacheEnabled = oldCache; gin.DefaultErrorWriter = oldWriter })
	for _, brokenDB := range []bool{false, true} {
		db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
		require.NoError(t, err)
		require.NoError(t, db.AutoMigrate(&model.Ability{}))
		if brokenDB {
			require.NoError(t, db.Migrator().DropTable(&model.Ability{}))
		}
		model.DB = db
		var logs bytes.Buffer
		gin.DefaultErrorWriter = &logs
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
		info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{}, OriginModelName: "claude-opus-5-5", UsingGroup: "claude-enterprise"}
		channel, apiErr := getChannel(c, info, &service.RetryParam{Ctx: c, TokenGroup: "claude-enterprise", ModelName: info.OriginModelName})
		assert.Nil(t, channel)
		require.NotNil(t, apiErr)
		assert.Contains(t, apiErr.Error(), info.OriginModelName)
		assert.NotContains(t, apiErr.Error(), "claude-enterprise")
		assert.NotContains(t, apiErr.Error(), "SQL")
		assert.Equal(t, types.ErrorCodeGetChannelFailed, apiErr.GetErrorCode())
		assert.True(t, types.IsSkipRetryError(apiErr))
		assert.Contains(t, logs.String(), "claude-enterprise")
		sqlDB, err := db.DB()
		require.NoError(t, err)
		require.NoError(t, sqlDB.Close())
	}
}
