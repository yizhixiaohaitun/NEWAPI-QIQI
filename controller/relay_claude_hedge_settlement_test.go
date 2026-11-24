package controller

import (
	"bufio"
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
	"github.com/stretchr/testify/require"
)

func TestClaudeHedgeSettlementAfterClientDisconnect(t *testing.T) {
	oldDB, oldLogDB := model.DB, model.LOG_DB
	oldMain, oldLogType := common.MainDatabaseType(), common.LogDatabaseType()
	oldRedis, oldCache, oldBatch := common.RedisEnabled, common.MemoryCacheEnabled, common.BatchUpdateEnabled
	oldLog, oldRetry, oldCount, oldTimeout := common.LogConsumeEnabled, common.RetryTimes, constant.CountToken, constant.StreamingTimeout
	oldQiqi, oldGeneral := *operation_setting.GetQiqiSetting(), *operation_setting.GetGeneralSetting()
	oldGroup, oldPrices := ratio_setting.GroupRatio2JSONString(), ratio_setting.ModelPrice2JSONString()
	t.Cleanup(func() {
		model.DB, model.LOG_DB = oldDB, oldLogDB
		common.SetDatabaseTypes(oldMain, oldLogType)
		common.RedisEnabled, common.MemoryCacheEnabled, common.BatchUpdateEnabled = oldRedis, oldCache, oldBatch
		common.LogConsumeEnabled, common.RetryTimes, constant.CountToken, constant.StreamingTimeout = oldLog, oldRetry, oldCount, oldTimeout
		*operation_setting.GetQiqiSetting(), *operation_setting.GetGeneralSetting() = oldQiqi, oldGeneral
		require.NoError(t, operation_setting.SetChannelRetryPolicy(operation_setting.DefaultChannelRetryPolicy))
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(oldGroup))
		require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(oldPrices))
	})

	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	db := openRequestCompatibilitySQLite(t, ":memory:")
	require.NoError(t, db.AutoMigrate(&model.Ability{}, &model.Channel{}, &model.User{}, &model.Token{}, &model.Log{}, &model.ChannelPurityGroup{}, &model.ChannelPurityMember{}))
	model.DB, model.LOG_DB = db, db
	common.RedisEnabled, common.MemoryCacheEnabled, common.BatchUpdateEnabled = false, false, false
	common.LogConsumeEnabled, common.RetryTimes, constant.CountToken, constant.StreamingTimeout = true, 1, false, 10
	operation_setting.GetQiqiSetting().ZeroReplyAutoRefundEnabled = true
	operation_setting.GetGeneralSetting().PingIntervalEnabled = false
	require.NoError(t, operation_setting.SetChannelRetryPolicy(`[{"stream_hedge":{"delay_ms":40,"retry_same_channel_first":true}}]`))
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"settlement-test":1}`))
	require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(`{"claude-opus-5-5":0.02}`))

	const userID, tokenID, channelID, credits = 991, 992, 993, 1000000
	const group, modelName, path = "settlement-test", "claude-opus-5-5", "/v1/messages"
	const requestID, visible = "local-claude-disconnect", "the backup delivered this visible reply"
	require.NoError(t, db.Create(&model.User{Id: userID, Username: group, Quota: credits, Status: common.UserStatusEnabled}).Error)
	require.NoError(t, db.Create(&model.Token{Id: tokenID, UserId: userID, Key: group, RemainQuota: credits, Status: common.TokenStatusEnabled}).Error)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var started atomic.Int32
	primaryCanceled := make(chan struct{})
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		lane := started.Add(1)
		_, _ = io.Copy(io.Discard, r.Body)
		send := func(data string) {
			_, _ = io.WriteString(w, "data: "+data+"\n\n")
			w.(http.Flusher).Flush()
		}
		w.Header().Set("Content-Type", "text/event-stream")
		if lane == 1 {
			send(`{"type":"message_start","message":{"id":"losing-lane","model":"claude-opus-5-5","usage":{"input_tokens":12345,"output_tokens":4}}}`)
		} else {
			send(`{"type":"message_start","message":{"id":"winning-lane","model":"claude-opus-5-5","usage":{"input_tokens":17,"output_tokens":0,"cache_read_input_tokens":23,"cache_creation_input_tokens":9,"cache_creation":{"ephemeral_5m_input_tokens":4,"ephemeral_1h_input_tokens":5}}}}`)
			send(`{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`)
			send(`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"` + visible + `"}}`)
			// Do not send any message_delta or terminal usage before the reader disconnects.
		}
		select {
		case <-r.Context().Done():
			if lane == 1 {
				close(primaryCanceled)
			}
		case <-ctx.Done():
		}
	}))
	defer upstream.Close()
	require.NoError(t, db.Create(&model.Channel{Id: channelID, Type: constant.ChannelTypeAnthropic, Name: group, Key: group, BaseURL: &upstream.URL, Group: group, Models: modelName, Status: common.ChannelStatusEnabled}).Error)
	require.NoError(t, db.Create(&model.Ability{Group: group, Model: modelName, ChannelId: channelID, Enabled: true, Weight: 1}).Error)
	service.InitHttpClient()
	handler := gin.New()
	handler.POST(path, func(c *gin.Context) {
		defer common.CleanupBodyStorage(c)
		c.Set(common.RequestIdKey, requestID)
		common.SetContextKey(c, constant.ContextKeyUsingGroup, group)
		common.SetContextKey(c, constant.ContextKeyTokenGroup, group)
		common.SetContextKey(c, constant.ContextKeyUserSetting, dto.UserSetting{BillingPreference: "wallet_only", AcceptUnsetRatioModel: true})
		common.SetContextKey(c, constant.ContextKeyUserId, userID)
		common.SetContextKey(c, constant.ContextKeyUserQuota, credits)
		common.SetContextKey(c, constant.ContextKeyTokenId, tokenID)
		common.SetContextKey(c, constant.ContextKeyTokenKey, group)
		c.Next()
	}, middleware.Distribute(), func(c *gin.Context) { Relay(c, types.RelayFormatClaude) })
	relayDone := make(chan struct{})
	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer close(relayDone)
		handler.ServeHTTP(w, r)
	}))
	defer gateway.Close()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, gateway.URL+path, strings.NewReader(`{"model":"claude-opus-5-5","stream":true,"max_tokens":16,"messages":[{"role":"user","content":"hello"}]}`))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	resp, err := gateway.Client().Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	read := bufio.NewScanner(resp.Body)
	var delivered strings.Builder
	for read.Scan() {
		delivered.WriteString(read.Text())
		if strings.Contains(read.Text(), visible) {
			break
		}
	}
	require.NoError(t, read.Err())
	require.Contains(t, delivered.String(), visible)
	require.NotContains(t, delivered.String(), "losing-lane")
	require.NoError(t, resp.Body.Close())
	for _, done := range []<-chan struct{}{primaryCanceled, relayDone} {
		select {
		case <-done:
		case <-ctx.Done():
			t.Fatal("upstream cancellation and final settlement must finish after the client disconnects")
		}
	}
	require.EqualValues(t, 2, started.Load())
	var logs []model.Log
	require.NoError(t, db.Where("request_id = ? AND type IN ?", requestID, []int{model.LogTypeConsume, model.LogTypeRefund}).Find(&logs).Error)
	require.Len(t, logs, 1, "one settlement for the winner, no zero-reply refund: %+v", logs)
	consumption := logs[0]
	require.Equal(t, model.LogTypeConsume, consumption.Type)
	require.Equal(t, 17, consumption.PromptTokens)
	require.Positive(t, consumption.CompletionTokens)
	require.Equal(t, 10000, consumption.Quota) // Fixed price, charged once.
	require.Contains(t, consumption.Other, `"end_reason":"client_gone"`)
	require.Contains(t, consumption.Other, `"cache_tokens":23`)
	require.Contains(t, consumption.Other, `"cache_creation_tokens_5m":4`)
	require.Contains(t, consumption.Other, `"cache_creation_tokens_1h":5`)
	var user model.User
	var token model.Token
	require.NoError(t, db.First(&user, userID).Error)
	require.NoError(t, db.First(&token, tokenID).Error)
	require.Equal(t, consumption.Quota, credits-user.Quota)
	require.Equal(t, consumption.Quota, credits-token.RemainQuota)
}
