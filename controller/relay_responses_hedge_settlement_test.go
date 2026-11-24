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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Same-channel native Responses race: the backup delivers output, the client
// leaves, and only the winner settles against the wallet and the token.
func TestResponsesHedgeSettlementAfterClientDisconnect(t *testing.T) {
	oldDB, oldLogDB := model.DB, model.LOG_DB
	oldMain, oldLogType := common.MainDatabaseType(), common.LogDatabaseType()
	oldRedis, oldCache, oldBatch := common.RedisEnabled, common.MemoryCacheEnabled, common.BatchUpdateEnabled
	oldLog, oldRetry, oldCount, oldTimeout := common.LogConsumeEnabled, common.RetryTimes, constant.CountToken, constant.StreamingTimeout
	oldQiqi, oldGeneral := *operation_setting.GetQiqiSetting(), *operation_setting.GetGeneralSetting()
	oldGroup, oldPrices, oldRatios := ratio_setting.GroupRatio2JSONString(), ratio_setting.ModelPrice2JSONString(), ratio_setting.ModelRatio2JSONString()
	t.Cleanup(func() {
		model.DB, model.LOG_DB = oldDB, oldLogDB
		common.SetDatabaseTypes(oldMain, oldLogType)
		common.RedisEnabled, common.MemoryCacheEnabled, common.BatchUpdateEnabled = oldRedis, oldCache, oldBatch
		common.LogConsumeEnabled, common.RetryTimes, constant.CountToken, constant.StreamingTimeout = oldLog, oldRetry, oldCount, oldTimeout
		*operation_setting.GetQiqiSetting(), *operation_setting.GetGeneralSetting() = oldQiqi, oldGeneral
		require.NoError(t, operation_setting.SetChannelRetryPolicy(operation_setting.DefaultChannelRetryPolicy))
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(oldGroup))
		require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(oldPrices))
		require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(oldRatios))
	})
	service.InitTokenEncoders()

	const modelName, path, group = "gpt-race-settlement", "/v1/responses", "responses-settlement"
	const text, args = "the backup delivered this complete text", `{"city":"local fixture"}`
	for _, tc := range []struct {
		name, frames, readUntil, estimate string
		reportedIn, reportedOut           int
	}{
		{name: "text_done_without_delta", frames: `{"type":"response.output_text.done","item_id":"m","output_index":0,"content_index":0,"text":"` + text + `"}`, readUntil: text, estimate: text},
		{name: "tool_arguments_delta_then_done", frames: `{"type":"response.function_call_arguments.delta","item_id":"t","output_index":0,"delta":"{\"city\":"}` + "\n\ndata: " + `{"type":"response.function_call_arguments.done","item_id":"t","output_index":0,"arguments":"{\"city\":\"local fixture\"}"}`, readUntil: "function_call_arguments.done", estimate: args},
		{name: "reported_usage_wins", frames: `{"type":"response.output_text.delta","delta":"` + text + `"}` + "\n\ndata: " + `{"type":"response.completed","response":{"status":"completed","usage":{"input_tokens":7,"output_tokens":3,"total_tokens":10}}}`, readUntil: "response.completed", reportedIn: 7, reportedOut: 3},
	} {
		t.Run(tc.name, func(t *testing.T) {
			common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
			db := openRequestCompatibilitySQLite(t, ":memory:")
			require.NoError(t, db.AutoMigrate(&model.Ability{}, &model.Channel{}, &model.User{}, &model.Token{}, &model.Log{}, &model.ChannelPurityGroup{}, &model.ChannelPurityMember{}))
			model.DB, model.LOG_DB = db, db
			common.RedisEnabled, common.MemoryCacheEnabled, common.BatchUpdateEnabled = false, false, false
			common.LogConsumeEnabled, common.RetryTimes, constant.CountToken, constant.StreamingTimeout = true, 1, true, 10
			operation_setting.GetQiqiSetting().ZeroReplyAutoRefundEnabled = true
			operation_setting.GetGeneralSetting().PingIntervalEnabled = false
			require.NoError(t, operation_setting.SetChannelRetryPolicy(`[{"stream_hedge":{"delay_ms":40,"retry_same_channel_first":true,"parallel_backup_scope":"all"}}]`))
			require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"responses-settlement":1}`))
			require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(`{}`))
			require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(`{"gpt-race-settlement":1}`))

			const userID, tokenID, channelID, credits = 995, 996, 997, 1000000
			requestID := "local-responses-" + tc.name
			require.NoError(t, db.Create(&model.User{Id: userID, Username: group, Quota: credits, Status: common.UserStatusEnabled}).Error)
			require.NoError(t, db.Create(&model.Token{Id: tokenID, UserId: userID, Key: group, RemainQuota: credits, Status: common.TokenStatusEnabled}).Error)

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			var started atomic.Int32
			primaryCanceled := make(chan struct{})
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				lane := started.Add(1)
				_, _ = io.Copy(io.Discard, r.Body)
				w.Header().Set("Content-Type", "text/event-stream")
				send := func(data string) { _, _ = io.WriteString(w, "data: "+data+"\n\n"); w.(http.Flusher).Flush() }
				if lane == 1 {
					// The losing lane reports a huge usage that must never be billed.
					send(`{"type":"response.created","response":{"id":"losing-lane","status":"in_progress","usage":{"input_tokens":12345,"output_tokens":678}}}`)
				} else {
					send(`{"type":"response.created","response":{"id":"winning-lane","status":"in_progress"}}`)
					send(tc.frames)
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
			require.NoError(t, db.Create(&model.Channel{Id: channelID, Type: constant.ChannelTypeOpenAI, Name: group, Key: group, BaseURL: &upstream.URL, Group: group, Models: modelName, Status: common.ChannelStatusEnabled}).Error)
			require.NoError(t, db.Create(&model.Ability{Group: group, Model: modelName, ChannelId: channelID, Enabled: true, Weight: 1}).Error)
			for _, q := range []string{
				`CREATE TABLE quota_moves (account TEXT, delta INTEGER)`,
				`CREATE TRIGGER wallet_move AFTER UPDATE OF quota ON users WHEN OLD.quota != NEW.quota BEGIN INSERT INTO quota_moves VALUES ('wallet', NEW.quota - OLD.quota); END`,
				`CREATE TRIGGER token_move AFTER UPDATE OF remain_quota ON tokens WHEN OLD.remain_quota != NEW.remain_quota BEGIN INSERT INTO quota_moves VALUES ('token', NEW.remain_quota - OLD.remain_quota); END`,
			} {
				require.NoError(t, db.Exec(q).Error)
			}
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
			}, middleware.Distribute(), func(c *gin.Context) { Relay(c, types.RelayFormatOpenAIResponses) })
			relayDone := make(chan struct{})
			gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				defer close(relayDone)
				handler.ServeHTTP(w, r)
			}))
			defer gateway.Close()

			req, err := http.NewRequestWithContext(ctx, http.MethodPost, gateway.URL+path, strings.NewReader(`{"model":"gpt-race-settlement","stream":true,"store":false,"input":"hello from the local fixture"}`))
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")
			resp, err := gateway.Client().Do(req)
			require.NoError(t, err)
			defer resp.Body.Close()
			require.Equal(t, http.StatusOK, resp.StatusCode)
			read := bufio.NewScanner(resp.Body)
			var delivered strings.Builder
			for read.Scan() {
				delivered.WriteString(read.Text() + "\n")
				if strings.Contains(read.Text(), tc.readUntil) {
					break
				}
			}
			require.NoError(t, read.Err())
			require.Contains(t, delivered.String(), tc.readUntil)
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
			consume := logs[0]
			require.Equal(t, model.LogTypeConsume, consume.Type)
			wantIn, wantOut := tc.reportedIn, tc.reportedOut
			if tc.estimate != "" {
				wantOut = service.CountTextToken(tc.estimate, modelName)
				require.Positive(t, wantOut)
				require.Positive(t, consume.PromptTokens, "estimated prompt is billed with delivered output")
				wantIn = consume.PromptTokens
				assert.Contains(t, consume.Other, "billing-usage-openai-estimated")
			}
			require.Equal(t, wantIn, consume.PromptTokens)
			require.Equal(t, wantOut, consume.CompletionTokens)
			want := common.QuotaRound(float64(wantIn) + float64(wantOut)*ratio_setting.GetCompletionRatio(modelName))
			require.Positive(t, want)
			require.Equal(t, want, consume.Quota)
			var user model.User
			var token model.Token
			require.NoError(t, db.First(&user, userID).Error)
			require.NoError(t, db.First(&token, tokenID).Error)
			require.Equal(t, want, credits-user.Quota, "wallet net charge")
			require.Equal(t, want, credits-token.RemainQuota, "token net charge")
			for _, account := range []string{"wallet", "token"} {
				var moves []int
				require.NoError(t, db.Table("quota_moves").Where("account = ?", account).Order("rowid").Pluck("delta", &moves).Error)
				require.Len(t, moves, 2, "one precharge and one settlement adjustment: %v", moves)
				sum := 0
				for _, m := range moves {
					sum += m
				}
				require.Equal(t, -want, sum, account)
			}
			t.Logf("prompt=%d output=%d net_wallet=%d net_token=%d", wantIn, wantOut, credits-user.Quota, credits-token.RemainQuota)
		})
	}
}
