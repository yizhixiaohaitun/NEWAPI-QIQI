package middleware

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestDistributorErrorsDoNotExposeGroup(t *testing.T) {
	require.NoError(t, i18n.Init())
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
		for _, lang := range []string{"en", "zh-CN", "zh-TW"} {
			t.Run(lang+map[bool]string{false: "/no-channel", true: "/database-error"}[brokenDB], func(t *testing.T) {
				var logs bytes.Buffer
				gin.DefaultErrorWriter = &logs
				recorder := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(recorder)
				c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"claude-opus-5-5"}`))
				c.Request.Header.Set("Content-Type", "application/json")
				c.Request.Header.Set("Accept-Language", lang)
				c.Set(common.RequestIdKey, "privacy-request-id")
				common.SetContextKey(c, constant.ContextKeyUsingGroup, "claude-enterprise")
				Distribute()(c)
				assert.Equal(t, http.StatusServiceUnavailable, recorder.Code)
				assert.True(t, c.IsAborted())
				var response struct {
					Error struct{ Message, Type, Code string } `json:"error"`
				}
				require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
				assert.Contains(t, response.Error.Message, "claude-opus-5-5")
				assert.Contains(t, response.Error.Message, "privacy-request-id")
				assert.NotContains(t, recorder.Body.String(), "claude-enterprise")
				assert.NotContains(t, recorder.Body.String(), "under group")
				assert.NotContains(t, recorder.Body.String(), "SQL")
				assert.Equal(t, "model_not_found", response.Error.Code)
				assert.Contains(t, logs.String(), "claude-enterprise")
			})
		}
		sqlDB, err := db.DB()
		require.NoError(t, err)
		require.NoError(t, sqlDB.Close())
	}
}
