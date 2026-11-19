package controller

import (
	"fmt"
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
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestClaudeRelayStatusMappingMatchesUserLog(t *testing.T) {
	gin.SetMode(gin.TestMode)
	oldDB, oldLogDB := model.DB, model.LOG_DB
	oldCache, oldRedis, oldRetryTimes := common.MemoryCacheEnabled, common.RedisEnabled, common.RetryTimes
	oldCountToken, oldErrorLog := constant.CountToken, constant.ErrorLogEnabled
	oldGroupRatios, oldModelPrices := ratio_setting.GroupRatio2JSONString(), ratio_setting.ModelPrice2JSONString()
	quotaSetting := operation_setting.GetQuotaSetting()
	oldFreePreConsume := quotaSetting.EnableFreeModelPreConsume
	t.Cleanup(func() {
		model.DB, model.LOG_DB = oldDB, oldLogDB
		common.MemoryCacheEnabled, common.RedisEnabled, common.RetryTimes = oldCache, oldRedis, oldRetryTimes
		constant.CountToken, constant.ErrorLogEnabled = oldCountToken, oldErrorLog
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(oldGroupRatios))
		require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(oldModelPrices))
		quotaSetting.EnableFreeModelPreConsume = oldFreePreConsume
	})

	tests := []struct {
		name, errorType, message string
		upstream, want           int
	}{
		{"mapped forbidden", "permission_error", "not allowed", 403, 429},
		{"unmapped unauthorized", "authentication_error", "wrong credentials", 401, 401},
		{"genuine upstream 500", "api_error", "provider failed", 500, 500},
		{"embedded forbidden", "permission_error", "not allowed", 200, 429},
		{"embedded rate limit", "rate_limit_error", "too many requests", 200, 429},
		{"transport failure", "", "upstream error: do request failed", 0, 502},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tc.upstream)
				_, _ = fmt.Fprintf(w, `{"type":"error","error":{"type":%q,"message":%q}}`, tc.errorType, tc.message)
			}))
			if tc.upstream == 0 {
				upstream.Close()
			}
			t.Cleanup(upstream.Close)
			db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
			require.NoError(t, err)
			sqlDB, err := db.DB()
			require.NoError(t, err)
			sqlDB.SetMaxOpenConns(1)
			t.Cleanup(func() { _ = sqlDB.Close() })
			require.NoError(t, db.AutoMigrate(&model.Ability{}, &model.Channel{}, &model.User{}, &model.Log{}, &model.ChannelPurityGroup{}, &model.ChannelPurityMember{}))
			model.DB, model.LOG_DB = db, db
			common.MemoryCacheEnabled, common.RedisEnabled, common.RetryTimes = true, false, 0
			constant.CountToken, constant.ErrorLogEnabled = false, true
			quotaSetting.EnableFreeModelPreConsume = false
			require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"fixed":0}`))
			require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(`{"test-model":0}`))
			require.NoError(t, db.Create(&model.User{Id: 1, Username: "status-test", Quota: 1000000, Status: common.UserStatusEnabled}).Error)
			require.NoError(t, db.Create(&model.Channel{
				Id: 461, Type: constant.ChannelTypeAnthropic, Name: "mock", Key: "test-key", BaseURL: &upstream.URL,
				Group: "fixed", Models: "test-model", Status: common.ChannelStatusEnabled,
				StatusCodeMapping: common.GetPointer(`{"403":429}`),
			}).Error)
			require.NoError(t, db.Create(&model.Ability{Group: "fixed", Model: "test-model", ChannelId: 461, Enabled: true, Priority: common.GetPointer(int64(0)), Weight: 1}).Error)
			model.InitChannelCache()
			service.InitHttpClient()

			router := gin.New()
			router.POST("/v1/messages", func(c *gin.Context) {
				common.SetContextKey(c, constant.ContextKeyUsingGroup, "fixed")
				common.SetContextKey(c, constant.ContextKeyTokenGroup, "fixed")
				common.SetContextKey(c, constant.ContextKeyUserSetting, dto.UserSetting{AcceptUnsetRatioModel: true})
				common.SetContextKey(c, constant.ContextKeyUserId, 1)
				common.SetContextKey(c, constant.ContextKeyUserQuota, 1000000)
				common.SetContextKey(c, constant.ContextKeyTokenId, 123)
				c.Next()
			}, middleware.Distribute(), func(c *gin.Context) { Relay(c, types.RelayFormatClaude) })
			req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(`{"model":"test-model","max_tokens":16,"messages":[{"role":"user","content":"hi"}]}`))
			req.Header.Set("Content-Type", "application/json")
			resp := httptest.NewRecorder()
			router.ServeHTTP(resp, req)
			require.Equal(t, tc.want, resp.Code, resp.Body.String())
			assert.Contains(t, resp.Body.String(), tc.message)
			logs, err := model.GetLogByTokenId(123)
			require.NoError(t, err)
			require.Len(t, logs, 1)
			assert.Equal(t, model.LogTypeError, logs[0].Type)
			assert.Equal(t, fmt.Sprintf("status_code=%d, %s", tc.want, tc.message), logs[0].Content)
			other, err := common.StrToMap(logs[0].Other)
			require.NoError(t, err)
			assert.EqualValues(t, tc.want, other["status_code"])
		})
	}
}
