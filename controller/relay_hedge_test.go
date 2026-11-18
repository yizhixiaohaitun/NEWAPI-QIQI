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
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/model_setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestStreamRaceFirstEffectiveOutputWins(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	primaryCtx, primaryCancel := context.WithCancelCause(ctx)
	backupCtx, backupCancel := context.WithCancelCause(ctx)
	r := &streamRace{ctx: ctx, cancel: [2]context.CancelCauseFunc{primaryCancel, backupCancel}, lanes: [2]context.Context{primaryCtx, backupCtx}}
	require.True(t, r.startBackup())
	require.True(t, r.claim(2))
	require.ErrorIs(t, context.Cause(primaryCtx), relaycommon.ErrStreamRaceLost)
	require.Nil(t, context.Cause(backupCtx))
	require.False(t, r.claim(1))
}

func TestStreamRaceBackupCanWinAfterPrimaryFinished(t *testing.T) {
	primaryCtx, primaryCancel := context.WithCancelCause(context.Background())
	backupCtx, backupCancel := context.WithCancelCause(context.Background())
	defer primaryCancel(nil)
	defer backupCancel(nil)
	r := &streamRace{ctx: context.Background(), cancel: [2]context.CancelCauseFunc{primaryCancel, backupCancel}, lanes: [2]context.Context{primaryCtx, backupCtx}}
	require.True(t, r.startBackup())
	r.primaryDone = true
	require.True(t, r.claim(2))
	require.ErrorIs(t, context.Cause(primaryCtx), relaycommon.ErrStreamRaceLost)
}

func TestStreamRaceDoesNotStartAfterPrimaryFinished(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	primaryCtx, primaryCancel := context.WithCancelCause(ctx)
	backupCtx, backupCancel := context.WithCancelCause(ctx)
	r := &streamRace{ctx: ctx, cancel: [2]context.CancelCauseFunc{primaryCancel, backupCancel}, lanes: [2]context.Context{primaryCtx, backupCtx}}
	r.primaryDone = true
	require.False(t, r.startBackup())
	require.False(t, r.claim(2))
	require.Nil(t, context.Cause(primaryCtx))
	require.Nil(t, context.Cause(backupCtx))
}

// Exercise the real controller, routing, upstream HTTP cancellation and billing.
func TestClaudeOpus55HedgeHTTP(t *testing.T) {
	oldDB, oldLogDB := model.DB, model.LOG_DB
	oldMain, oldLogType := common.MainDatabaseType(), common.LogDatabaseType()
	oldCache, oldRedis, oldBatch := common.MemoryCacheEnabled, common.RedisEnabled, common.BatchUpdateEnabled
	oldRetry, oldCount, oldLog, oldError := common.RetryTimes, constant.CountToken, common.LogConsumeEnabled, constant.ErrorLogEnabled
	oldHedge, oldIdle := constant.StreamHedgeDelayMS, constant.StreamingTimeout
	oldRatios, oldPrices := ratio_setting.GroupRatio2JSONString(), ratio_setting.ModelPrice2JSONString()
	oldPing := *operation_setting.GetGeneralSetting()
	oldGlobal := *model_setting.GetGlobalSettings()
	t.Cleanup(func() {
		model.DB, model.LOG_DB = oldDB, oldLogDB
		common.SetDatabaseTypes(oldMain, oldLogType)
		common.MemoryCacheEnabled, common.RedisEnabled, common.BatchUpdateEnabled = oldCache, oldRedis, oldBatch
		common.RetryTimes, constant.CountToken, common.LogConsumeEnabled, constant.ErrorLogEnabled = oldRetry, oldCount, oldLog, oldError
		constant.StreamHedgeDelayMS, constant.StreamingTimeout = oldHedge, oldIdle
		*operation_setting.GetGeneralSetting() = oldPing
		*model_setting.GetGlobalSettings() = oldGlobal
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(oldRatios))
		require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(oldPrices))
	})
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	db := openRequestCompatibilitySQLite(t, ":memory:")
	require.NoError(t, db.AutoMigrate(&model.Ability{}, &model.Channel{}, &model.User{}, &model.Token{}, &model.Log{}, &model.ChannelPurityGroup{}, &model.ChannelPurityMember{}))
	model.DB, model.LOG_DB = db, db
	common.MemoryCacheEnabled, common.RedisEnabled, common.BatchUpdateEnabled = false, false, false
	common.RetryTimes, constant.CountToken, common.LogConsumeEnabled, constant.ErrorLogEnabled = 5, false, true, false
	constant.StreamingTimeout = 5
	operation_setting.GetGeneralSetting().PingIntervalEnabled = true
	operation_setting.GetGeneralSetting().PingIntervalSeconds = 1
	model_setting.GetGlobalSettings().AutoChatToResponsesEnabled = false
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"hedge-test":1}`))
	require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(`{"claude-opus-5-5":0}`))
	require.NoError(t, db.Create(&model.User{Id: 981, Username: "hedge-test", Quota: 1000000, Status: common.UserStatusEnabled}).Error)
	require.NoError(t, db.Create(&model.Token{Id: 982, UserId: 981, Key: "hedge-test", RemainQuota: 1000000, Status: common.TokenStatusEnabled}).Error)

	const payload = `{"model":"claude-opus-5-5","stream":true,"max_tokens":64,"messages":[{"role":"user","content":"replayed body"}]}`
	for _, tc := range []struct {
		name   string
		winner int
		backup bool
		status int
	}{
		{"backup_wins", 2, true, 200}, {"primary_wins_after_backup_error", 1, true, 200},
		{"backup_wins_after_primary_empty", 2, true, 200}, {"both_empty", 0, true, 502},
		{"effective_before_threshold", 1, false, 200}, {"disabled", 1, false, 200},
		{"pinned", 1, false, 200}, {"client_cancels", 0, true, 499},
		{"primary_empty_before_threshold", 0, false, 502},
	} {
		t.Run(tc.name, func(t *testing.T) {
			const primaryID, backupID = 983, 984
			var calls [2]atomic.Int32
			var bodies [2]atomic.Pointer[string]
			started := [2]chan struct{}{make(chan struct{}), make(chan struct{})}
			finished := [2]chan struct{}{make(chan struct{}), make(chan struct{})}
			canceled := [2]chan struct{}{make(chan struct{}), make(chan struct{})}
			ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
			defer cancel()
			makeUpstream := func(lane int) *httptest.Server {
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if calls[lane].Add(1) != 1 {
						w.WriteHeader(500)
						return
					}
					defer close(finished[lane])
					body, _ := io.ReadAll(r.Body)
					text := string(body)
					bodies[lane].Store(&text)
					label := []string{"primary", "backup"}[lane]
					w.Header().Set("Content-Type", "text/event-stream")
					w.Header().Set(common.RequestIdKey, label+"-request-id")
					send := func(data string) { _, _ = io.WriteString(w, "data: "+data+"\r\n\r\n"); w.(http.Flusher).Flush() }
					send(`{"type":"message_start","message":{"id":"` + label + `-preamble","model":"claude-opus-5-5","usage":{"input_tokens":10,"output_tokens":1}}}`)
					send(`{"type":"ping"}`)
					send(`{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`)
					close(started[lane])
					wait := func(ch <-chan struct{}) {
						select {
						case <-ch:
						case <-ctx.Done():
						case <-r.Context().Done():
						}
					}
					switch tc.name {
					case "primary_empty_before_threshold":
						send(`{"type":"message_stop"}`)
						return
					case "client_cancels":
						wait(r.Context().Done())
						<-r.Context().Done()
						close(canceled[lane])
						return
					case "backup_wins":
						if lane == 0 {
							wait(r.Context().Done())
							<-r.Context().Done()
							close(canceled[lane])
							return
						}
					case "primary_wins_after_backup_error":
						if lane == 1 {
							send(`{"type":"error","error":{"type":"overloaded_error","message":"local test"}}`)
							return
						}
						wait(finished[1])
					case "backup_wins_after_primary_empty", "both_empty":
						if lane == 0 {
							wait(started[1])
							send(`{"type":"message_stop"}`)
							return
						}
						wait(finished[0])
						if tc.winner == 0 {
							send(`{"type":"message_stop"}`)
							return
						}
					}
					send(`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"` + label + `-visible"}}`)
					send(`{"type":"content_block_stop","index":0}`)
					send(`{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":3}}`)
					send(`{"type":"message_stop"}`)
				}))
			}
			a, b := makeUpstream(0), makeUpstream(1)
			defer a.Close()
			defer b.Close()
			constant.StreamHedgeDelayMS = 70
			switch tc.name {
			case "disabled":
				constant.StreamHedgeDelayMS = 0
			case "effective_before_threshold", "primary_empty_before_threshold":
				constant.StreamHedgeDelayMS = 2000
			}
			require.NoError(t, db.Exec("DELETE FROM channels").Error)
			require.NoError(t, db.Exec("DELETE FROM abilities").Error)
			require.NoError(t, db.Exec("DELETE FROM logs").Error)
			high, low, weight := int64(10), int64(0), uint(1)
			require.NoError(t, db.Create(&[]model.Channel{{Id: primaryID, Type: constant.ChannelTypeAnthropic, Name: "primary", Key: "key-a", BaseURL: &a.URL, Group: "hedge-test", Models: "claude-opus-5-5", Status: common.ChannelStatusEnabled, Priority: &high, Weight: &weight}, {Id: backupID, Type: constant.ChannelTypeAnthropic, Name: "backup", Key: "key-b", BaseURL: &b.URL, Group: "hedge-test", Models: "claude-opus-5-5", Status: common.ChannelStatusEnabled, Priority: &low, Weight: &weight}}).Error)
			require.NoError(t, db.Create(&[]model.Ability{{Group: "hedge-test", Model: "claude-opus-5-5", ChannelId: primaryID, Enabled: true, Priority: &high, Weight: 1}, {Group: "hedge-test", Model: "claude-opus-5-5", ChannelId: backupID, Enabled: true, Priority: &low, Weight: 1}}).Error)
			service.InitHttpClient()
			router := gin.New()
			router.POST("/v1/messages", func(c *gin.Context) {
				defer common.CleanupBodyStorage(c)
				common.SetContextKey(c, constant.ContextKeyUsingGroup, "hedge-test")
				common.SetContextKey(c, constant.ContextKeyTokenGroup, "hedge-test")
				common.SetContextKey(c, constant.ContextKeyUserSetting, dto.UserSetting{BillingPreference: "wallet_only", AcceptUnsetRatioModel: true})
				common.SetContextKey(c, constant.ContextKeyUserId, 981)
				common.SetContextKey(c, constant.ContextKeyUserQuota, 1000000)
				common.SetContextKey(c, constant.ContextKeyTokenId, 982)
				common.SetContextKey(c, constant.ContextKeyTokenKey, "hedge-test")
				if tc.name == "pinned" {
					common.SetContextKey(c, constant.ContextKeyTokenSpecificChannelId, "983")
				}
				c.Next()
			}, middleware.Distribute(), func(c *gin.Context) { Relay(c, types.RelayFormatClaude) })
			req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(payload)).WithContext(ctx)
			req.Header.Set("Content-Type", "application/json")
			if tc.name == "client_cancels" {
				go func() {
					select {
					case <-started[1]:
						cancel()
					case <-ctx.Done():
					}
				}()
			}
			out := httptest.NewRecorder()
			router.ServeHTTP(out, req)
			require.Equal(t, tc.status, out.Code, out.Body.String())
			require.EqualValues(t, 1, calls[0].Load())
			require.Contains(t, *bodies[0].Load(), "replayed body")
			if tc.backup {
				require.EqualValues(t, 1, calls[1].Load())
				require.JSONEq(t, *bodies[0].Load(), *bodies[1].Load())
			} else {
				require.Zero(t, calls[1].Load())
			}
			waitCanceled := func(lane int) {
				t.Helper()
				select {
				case <-canceled[lane]:
				case <-time.After(time.Second):
					t.Fatal("upstream was not canceled")
				}
			}
			if tc.name == "backup_wins" || tc.name == "client_cancels" {
				waitCanceled(0)
			}
			if tc.name == "client_cancels" {
				waitCanceled(1)
			}
			var logs []model.Log
			require.NoError(t, db.Where("type = ?", model.LogTypeConsume).Find(&logs).Error)
			if tc.winner == 0 {
				require.NotContains(t, out.Header().Get("Content-Type"), "text/event-stream")
				require.NotContains(t, out.Body.String(), "-preamble")
				require.Empty(t, logs)
			} else {
				label, loser := []string{"primary", "backup"}[tc.winner-1], []string{"backup", "primary"}[tc.winner-1]
				require.Contains(t, out.Header().Get("Content-Type"), "text/event-stream")
				require.Equal(t, 1, strings.Count(out.Body.String(), label+"-preamble"))
				require.Equal(t, 1, strings.Count(out.Body.String(), label+"-visible"))
				require.NotContains(t, out.Body.String(), loser+"-preamble")
				require.Len(t, logs, 1)
				require.Equal(t, primaryID+tc.winner-1, logs[0].ChannelId)
			}
		})
	}
}
