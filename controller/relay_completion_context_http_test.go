package controller

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

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
)

type cancelOnWriteRecorder struct {
	*httptest.ResponseRecorder
	cancel context.CancelFunc
}

func (w *cancelOnWriteRecorder) Write(p []byte) (int, error) {
	n, err := w.ResponseRecorder.Write(p)
	w.cancel()
	return n, err
}

// Exercise Relay itself: cancellation occurs after the successful response was
// written but before the handler returns, not before dispatch to the upstream.
func TestRelayCompletionContextHTTP(t *testing.T) {
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
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"context-test":1}`))
	require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(`{"context-model":0}`))
	require.NoError(t, db.Create(&model.User{Id: 981, Username: "context-user", Quota: 1000000, Status: common.UserStatusEnabled}).Error)
	require.NoError(t, db.Create(&model.Token{Id: 982, UserId: 981, Key: "context-local", RemainQuota: 1000000, Status: common.TokenStatusEnabled}).Error)
	entered := make(chan struct{}, 1)
	var waiting atomic.Bool
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		if waiting.Load() {
			entered <- struct{}{}
			<-r.Context().Done()
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":"ok","object":"chat.completion","created":1,"model":"context-model","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`)
	}))
	defer upstream.Close()
	base, priority, weight := upstream.URL, int64(1), uint(1)
	require.NoError(t, db.Create(&model.Channel{Id: 983, Type: constant.ChannelTypeOpenAI, Name: "context", Key: "key", BaseURL: &base, Group: "context-test", Models: "context-model", Status: common.ChannelStatusEnabled, Priority: &priority, Weight: &weight}).Error)
	require.NoError(t, db.Create(&model.Ability{Group: "context-test", Model: "context-model", ChannelId: 983, Enabled: true, Priority: &priority, Weight: 1}).Error)
	service.InitHttpClient()
	router := gin.New()
	router.POST("/v1/chat/completions", func(c *gin.Context) {
		common.SetContextKey(c, constant.ContextKeyUsingGroup, "context-test")
		common.SetContextKey(c, constant.ContextKeyTokenGroup, "context-test")
		common.SetContextKey(c, constant.ContextKeyUserSetting, dto.UserSetting{BillingPreference: "wallet_only", AcceptUnsetRatioModel: true})
		common.SetContextKey(c, constant.ContextKeyUserId, 981)
		common.SetContextKey(c, constant.ContextKeyUserQuota, 1000000)
		common.SetContextKey(c, constant.ContextKeyTokenId, 982)
		common.SetContextKey(c, constant.ContextKeyTokenKey, "context-local")
		if c.GetHeader("X-Test-Total") == "yes" {
			common.SetContextKey(c, constant.ContextKeyTokenUpstreamTimeout, 1)
		}
		c.Next()
	}, middleware.Distribute(), func(c *gin.Context) { Relay(c, types.RelayFormatOpenAI) })
	request := func(ctx context.Context, wait, total bool) *http.Request {
		req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"context-model","messages":[{"role":"user","content":"hi"}]}`)).WithContext(ctx)
		req.Header.Set("Content-Type", "application/json")
		if wait {
			req.Header.Set("X-Test-Wait", "yes")
		}
		if total {
			req.Header.Set("X-Test-Total", "yes")
		}
		return req
	}
	t.Run("success_then_disconnect", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		out := &cancelOnWriteRecorder{ResponseRecorder: httptest.NewRecorder(), cancel: cancel}
		router.ServeHTTP(out, request(ctx, false, false))
		require.ErrorIs(t, ctx.Err(), context.Canceled)
		require.Equal(t, http.StatusOK, out.Code, out.Body.String())
		require.Contains(t, out.Body.String(), `"content":"ok"`)
		var count int64
		require.NoError(t, db.Model(&model.Log{}).Where("type = ?", model.LogTypeError).Count(&count).Error)
		require.Zero(t, count, "successful completed handler must not record a 499")
	})
	t.Run("canceled_during_upstream", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		waiting.Store(true)
		out := httptest.NewRecorder()
		done := make(chan struct{})
		go func() { router.ServeHTTP(out, request(ctx, true, false)); close(done) }()
		select {
		case <-entered:
			cancel()
		case <-time.After(5 * time.Second):
			t.Fatal("upstream not reached")
		}
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatal("relay did not finish")
		}
		require.Equal(t, 499, out.Code, out.Body.String())
	})
	t.Run("total_timeout_during_upstream", func(t *testing.T) {
		waiting.Store(true)
		out := httptest.NewRecorder()
		router.ServeHTTP(out, request(context.Background(), true, true))
		require.Equal(t, http.StatusInternalServerError, out.Code, out.Body.String())
		require.Contains(t, out.Body.String(), "upstream_timeout")
		<-entered
	})
}
