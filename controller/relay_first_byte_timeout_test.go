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
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRelayFirstByteTimeoutHTTP(t *testing.T) {
	oldDB, oldLogDB := model.DB, model.LOG_DB
	oldMain, oldLogType := common.MainDatabaseType(), common.LogDatabaseType()
	oldCache, oldRedis, oldBatch := common.MemoryCacheEnabled, common.RedisEnabled, common.BatchUpdateEnabled
	oldRetry, oldCount, oldLog, oldError := common.RetryTimes, constant.CountToken, common.LogConsumeEnabled, constant.ErrorLogEnabled
	oldTimeout, oldIdle := constant.FirstByteTimeout, constant.StreamingTimeout
	oldRatios, oldPrices := ratio_setting.GroupRatio2JSONString(), ratio_setting.ModelPrice2JSONString()
	oldQiqi := *operation_setting.GetQiqiSetting()
	oldPing := operation_setting.GetGeneralSetting().PingIntervalEnabled
	t.Cleanup(func() {
		model.DB, model.LOG_DB = oldDB, oldLogDB
		common.SetDatabaseTypes(oldMain, oldLogType)
		common.MemoryCacheEnabled, common.RedisEnabled, common.BatchUpdateEnabled = oldCache, oldRedis, oldBatch
		common.RetryTimes, constant.CountToken, common.LogConsumeEnabled, constant.ErrorLogEnabled = oldRetry, oldCount, oldLog, oldError
		constant.FirstByteTimeout, constant.StreamingTimeout = oldTimeout, oldIdle
		*operation_setting.GetQiqiSetting() = oldQiqi
		operation_setting.GetGeneralSetting().PingIntervalEnabled = oldPing
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(oldRatios))
		require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(oldPrices))
	})
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	db := openRequestCompatibilitySQLite(t, ":memory:")
	require.NoError(t, db.AutoMigrate(&model.Ability{}, &model.Channel{}, &model.User{}, &model.Token{}, &model.Log{}, &model.ChannelPurityGroup{}, &model.ChannelPurityMember{}))
	model.DB, model.LOG_DB = db, db
	common.MemoryCacheEnabled, common.RedisEnabled, common.BatchUpdateEnabled = false, false, false
	common.RetryTimes, constant.CountToken, common.LogConsumeEnabled, constant.ErrorLogEnabled = 0, false, true, false
	constant.FirstByteTimeout, constant.StreamingTimeout = 1, 30
	operation_setting.GetQiqiSetting().ResponsesStreamErrorRetryEnabled = true
	operation_setting.GetQiqiSetting().ResponsesStreamErrorRetryTimes = 2
	operation_setting.GetGeneralSetting().PingIntervalEnabled = false
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"first-byte":1}`))
	require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(`{"first-byte-model":0}`))
	require.NoError(t, db.Create(&model.User{Id: 891, Username: "first-byte-user", Quota: 1000000, Status: common.UserStatusEnabled}).Error)
	require.NoError(t, db.Create(&model.Token{Id: 892, UserId: 891, Key: "first-byte-local", RemainQuota: 1000000, Status: common.TokenStatusEnabled}).Error)
	var callsA, callsB atomic.Int32
	var scenario atomic.Int32
	a := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callsA.Add(1)
		_, _ = io.Copy(io.Discard, r.Body)
		switch scenario.Load() {
		case 1, 4, 5:
			w.Header().Set("Content-Type", "text/event-stream")
			w.(http.Flusher).Flush()
		case 2:
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = io.WriteString(w, `data: {"id":"audit","prompt_filter_results":[{"content_filter_results":{"hate":{"filtered":false}}}],"choices":[]}`+string([]byte{10, 10}))
			w.(http.Flusher).Flush()
			select {
			case <-r.Context().Done():
				return
			case <-time.After(1200 * time.Millisecond):
			}
			_, _ = io.WriteString(w, `data: {"id":"audit","choices":[{"index":0,"delta":{"content":"audited"}}]}`+string([]byte{10, 10})+"data: [DONE]"+string([]byte{10, 10}))
			return
		case 3:
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = io.WriteString(w, `data: {"id":"partial","choices":[{"index":0,"delta":{"content":"partial"}}]}`+string([]byte{10, 10}))
			w.(http.Flusher).Flush()
			select {
			case <-r.Context().Done():
				return
			case <-time.After(1200 * time.Millisecond):
			}
			_, _ = io.WriteString(w, "data: [DONE]"+string([]byte{10, 10}))
			return
		}
		<-r.Context().Done()
	}))
	defer a.Close()
	b := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if scenario.Load() == 6 {
			callsA.Add(1)
			_, _ = io.Copy(io.Discard, r.Body)
			<-r.Context().Done()
			return
		}
		callsB.Add(1)
		w.Header().Set("Content-Type", "text/event-stream")
		if scenario.Load() == 4 {
			_, _ = io.WriteString(w, `data: {"type":"response.output_text.delta","delta":"hello"}`+string([]byte{10, 10})+"data: [DONE]"+string([]byte{10, 10}))
			return
		}
		_, _ = io.WriteString(w, `data: {"id":"success","choices":[{"index":0,"delta":{"content":"hello"}}]}`+string([]byte{10, 10})+"data: [DONE]"+string([]byte{10, 10}))
	}))
	defer b.Close()
	baseA, baseB := a.URL, b.URL
	high, low, weight := int64(10), int64(0), uint(1)
	require.NoError(t, db.Create(&[]model.Channel{{Id: 893, Type: constant.ChannelTypeOpenAI, Name: "a", Key: "a", BaseURL: &baseA, Group: "first-byte", Models: "first-byte-model", Status: common.ChannelStatusEnabled, Priority: &high, Weight: &weight}, {Id: 894, Type: constant.ChannelTypeOpenAI, Name: "b", Key: "b", BaseURL: &baseB, Group: "first-byte", Models: "first-byte-model", Status: common.ChannelStatusEnabled, Priority: &low, Weight: &weight}}).Error)
	require.NoError(t, db.Create(&[]model.Ability{{Group: "first-byte", Model: "first-byte-model", ChannelId: 893, Enabled: true, Priority: &high, Weight: 1}, {Group: "first-byte", Model: "first-byte-model", ChannelId: 894, Enabled: true, Priority: &low, Weight: 1}}).Error)
	service.InitHttpClient()
	router := gin.New()
	identity := func(c *gin.Context) {
		common.SetContextKey(c, constant.ContextKeyUsingGroup, "first-byte")
		common.SetContextKey(c, constant.ContextKeyTokenGroup, "first-byte")
		common.SetContextKey(c, constant.ContextKeyUserSetting, dto.UserSetting{BillingPreference: "wallet_only", AcceptUnsetRatioModel: true})
		common.SetContextKey(c, constant.ContextKeyUserId, 891)
		common.SetContextKey(c, constant.ContextKeyUserQuota, 1000000)
		common.SetContextKey(c, constant.ContextKeyTokenId, 892)
		common.SetContextKey(c, constant.ContextKeyTokenKey, "first-byte-local")
		c.Next()
	}
	router.POST("/v1/chat/completions", identity, middleware.Distribute(), func(c *gin.Context) { Relay(c, types.RelayFormatOpenAI) })
	router.POST("/v1/responses", identity, middleware.Distribute(), func(c *gin.Context) { Relay(c, types.RelayFormatOpenAIResponses) })
	for _, tc := range []struct {
		name         string
		mode         int32
		wantA, wantB int32
		content      string
	}{
		{"headers_wait", 0, 1, 1, "hello"},
		{"body_wait", 1, 1, 1, "hello"},
		{"audit_is_not_discarded", 2, 1, 0, "prompt_filter_results"},
		{"visible_output_is_not_replayed", 3, 1, 0, "partial"},
		{"responses_body_wait", 4, 1, 1, "hello"},
		{"disabled_does_not_retry", 5, 1, 0, ""},
		{"retry_exhaustion", 6, 2, 0, "upstream_first_byte_timeout"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			scenario.Store(tc.mode)
			callsA.Store(0)
			callsB.Store(0)
			constant.FirstByteTimeout = 1
			if tc.mode == 5 {
				constant.FirstByteTimeout = 0
			}
			deadline := 5 * time.Second
			if tc.mode == 5 {
				deadline = 200 * time.Millisecond
			}
			ctx, cancel := context.WithTimeout(context.Background(), deadline)
			defer cancel()
			path, payload := "/v1/chat/completions", `{"model":"first-byte-model","stream":true,"messages":[{"role":"user","content":"hi"}]}`
			if tc.mode == 4 {
				path, payload = "/v1/responses", `{"model":"first-byte-model","stream":true,"input":"hi"}`
			}
			req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(payload)).WithContext(ctx)
			req.Header.Set("Content-Type", "application/json")
			out := httptest.NewRecorder()
			router.ServeHTTP(out, req)
			if tc.mode == 5 {
				assert.Equal(t, 499, out.Code)
			} else if tc.mode == 6 {
				assert.Equal(t, 504, out.Code)
				assert.Contains(t, out.Body.String(), tc.content)
			} else {
				assert.Equal(t, http.StatusOK, out.Code, out.Body.String())
				assert.Contains(t, out.Body.String(), tc.content)
			}
			if tc.mode == 2 {
				assert.Contains(t, out.Body.String(), "audited")
			}
			if tc.mode == 4 {
				assert.Contains(t, out.Body.String(), "response.output_text.delta")
			}
			assert.Equal(t, tc.wantA, callsA.Load())
			assert.Equal(t, tc.wantB, callsB.Load())
			if tc.wantB == 1 {
				var log model.Log
				require.NoError(t, db.Where("channel_id = ?", 894).Order("id desc").First(&log).Error)
				assert.Contains(t, log.Other, "upstream_first_byte_ms")
			}
		})
	}
}
