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
	r := &streamRace{ctx: ctx, started: 1, cancel: [maxStreamRaceLanes]context.CancelCauseFunc{primaryCancel, backupCancel}, lanes: [maxStreamRaceLanes]context.Context{primaryCtx, backupCtx}}
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
	r := &streamRace{ctx: context.Background(), started: 1, cancel: [maxStreamRaceLanes]context.CancelCauseFunc{primaryCancel, backupCancel}, lanes: [maxStreamRaceLanes]context.Context{primaryCtx, backupCtx}}
	require.True(t, r.startBackup())
	r.done[0] = true
	require.True(t, r.claim(2))
	require.ErrorIs(t, context.Cause(primaryCtx), relaycommon.ErrStreamRaceLost)
}

func TestStreamRaceDoesNotStartAfterPrimaryFinished(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	primaryCtx, primaryCancel := context.WithCancelCause(ctx)
	backupCtx, backupCancel := context.WithCancelCause(ctx)
	r := &streamRace{ctx: ctx, started: 1, cancel: [maxStreamRaceLanes]context.CancelCauseFunc{primaryCancel, backupCancel}, lanes: [maxStreamRaceLanes]context.Context{primaryCtx, backupCtx}}
	r.done[0] = true
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
	oldRetryStatuses := operation_setting.AutomaticRetryStatusCodesToString()
	oldGlobal := *model_setting.GetGlobalSettings()
	common.OptionMapRWMutex.Lock()
	oldOptions := common.OptionMap
	common.OptionMap = make(map[string]string)
	common.OptionMapRWMutex.Unlock()
	t.Cleanup(func() {
		model.DB, model.LOG_DB = oldDB, oldLogDB
		common.OptionMapRWMutex.Lock()
		common.OptionMap = oldOptions
		common.OptionMapRWMutex.Unlock()
		require.NoError(t, operation_setting.SetChannelRetryPolicy(operation_setting.DefaultChannelRetryPolicy))
		common.SetDatabaseTypes(oldMain, oldLogType)
		common.MemoryCacheEnabled, common.RedisEnabled, common.BatchUpdateEnabled = oldCache, oldRedis, oldBatch
		common.RetryTimes, constant.CountToken, common.LogConsumeEnabled, constant.ErrorLogEnabled = oldRetry, oldCount, oldLog, oldError
		constant.StreamHedgeDelayMS, constant.StreamingTimeout = oldHedge, oldIdle
		*operation_setting.GetGeneralSetting() = oldPing
		require.NoError(t, operation_setting.AutomaticRetryStatusCodesFromString(oldRetryStatuses))
		*model_setting.GetGlobalSettings() = oldGlobal
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(oldRatios))
		require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(oldPrices))
	})
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	db := openRequestCompatibilitySQLite(t, ":memory:")
	require.NoError(t, db.AutoMigrate(&model.Option{}, &model.Ability{}, &model.Channel{}, &model.User{}, &model.Token{}, &model.Log{}, &model.ChannelPurityGroup{}, &model.ChannelPurityMember{}))
	model.DB, model.LOG_DB = db, db
	common.MemoryCacheEnabled, common.RedisEnabled, common.BatchUpdateEnabled = false, false, false
	common.RetryTimes, constant.CountToken, common.LogConsumeEnabled, constant.ErrorLogEnabled = 5, false, true, false
	constant.StreamingTimeout = 5
	require.NoError(t, operation_setting.AutomaticRetryStatusCodesFromString("429,502"))
	operation_setting.GetGeneralSetting().PingIntervalEnabled = true
	operation_setting.GetGeneralSetting().PingIntervalSeconds = 1
	model_setting.GetGlobalSettings().AutoChatToResponsesEnabled = false
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"hedge-test":1}`))
	require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(`{"claude-opus-5-5":0}`))
	require.NoError(t, db.Create(&model.User{Id: 981, Username: "hedge-test", Quota: 1000000, Status: common.UserStatusEnabled}).Error)
	require.NoError(t, db.Create(&model.Token{Id: 982, UserId: 981, Key: "hedge-test", RemainQuota: 1000000, Status: common.TokenStatusEnabled}).Error)

	const payload = `{"model":"claude-opus-5-5","stream":true,"max_tokens":64,"messages":[{"role":"user","content":"replayed body"}]}`
	for _, tc := range []struct {
		name     string
		winner   int
		backup   bool
		status   int
		fallback bool
	}{
		{"backup_wins", 2, true, 200, false}, {"primary_wins_after_backup_error", 1, true, 200, false},
		{"backup_wins_after_primary_empty", 2, true, 200, false}, {"both_empty", 0, true, 502, false},
		{"effective_before_threshold", 1, false, 200, false}, {"disabled", 1, false, 200, false},
		{"pinned", 1, false, 200, false}, {"client_cancels", 0, true, 499, false},
		{"primary_empty_before_threshold", 0, false, 502, false},
		{"primary_rate_limit_before_threshold", 2, true, 200, false},
		{"both_rate_limited", 3, true, 200, true}, {"retry_budget_spent_by_backup", 0, true, 429, false},
		{"primary_output_timeout_backup_rate_limit", 3, true, 200, true},
		{"no_retry_budget", 0, false, 429, false}, {"committed_error", 1, false, 200, false},
		{"request_deadline_cancels_both", 0, true, 500, false},
		{"upstream_gateway_timeout", 0, false, 504, false},
		{"json_same_channel_wins", 2, true, 200, false},
		{"json_next_channel_wins", 3, true, 200, true},
		{"json_budget_only_allows_same_channel", 1, true, 200, false},
		{"json_zero_disables_startup_setting", 1, false, 200, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			const primaryID, backupID, fallbackID = 983, 984, 985
			sameChannel := strings.HasPrefix(tc.name, "json_") && tc.backup
			var primaryRequests atomic.Int32
			var calls [3]atomic.Int32
			var bodies [3]atomic.Pointer[string]
			started := [3]chan struct{}{make(chan struct{}), make(chan struct{}), make(chan struct{})}
			finished := [3]chan struct{}{make(chan struct{}), make(chan struct{}), make(chan struct{})}
			canceled := [3]chan struct{}{make(chan struct{}), make(chan struct{}), make(chan struct{})}
			ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
			defer cancel()
			makeUpstream := func(endpoint int) *httptest.Server {
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					lane := endpoint
					if sameChannel && endpoint == 0 && primaryRequests.Add(1) > 1 {
						lane = 1
					}
					if sameChannel && endpoint == 1 {
						lane = 2
					}
					if calls[lane].Add(1) != 1 {
						w.WriteHeader(500)
						return
					}
					defer close(finished[lane])
					body, _ := io.ReadAll(r.Body)
					text := string(body)
					bodies[lane].Store(&text)
					label := []string{"primary", "backup", "fallback"}[lane]
					if tc.name == "upstream_gateway_timeout" {
						w.Header().Set("Content-Type", "application/json")
						w.WriteHeader(http.StatusGatewayTimeout)
						_, _ = io.WriteString(w, `{"type":"error","error":{"type":"api_error","message":"local gateway timeout"}}`)
						return
					}
					if (tc.name == "primary_rate_limit_before_threshold" || tc.name == "no_retry_budget") && lane == 0 {
						w.Header().Set("Content-Type", "application/json")
						w.WriteHeader(http.StatusTooManyRequests)
						_, _ = io.WriteString(w, `{"type":"error","error":{"type":"rate_limit_error","message":"local rate limit"}}`)
						return
					}
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
					case "json_same_channel_wins":
						if lane == 0 {
							<-r.Context().Done()
							close(canceled[lane])
							return
						}
					case "json_next_channel_wins":
						if lane < 2 {
							<-r.Context().Done()
							close(canceled[lane])
							return
						}
						wait(started[0])
						wait(started[1])
					case "json_budget_only_allows_same_channel":
						if lane == 0 {
							wait(started[1])
							select {
							case <-time.After(180 * time.Millisecond):
							case <-r.Context().Done():
								return
							}
						} else {
							<-r.Context().Done()
							close(canceled[lane])
							return
						}
					case "json_zero_disables_startup_setting":
						select {
						case <-time.After(140 * time.Millisecond):
						case <-r.Context().Done():
							return
						}
					case "primary_output_timeout_backup_rate_limit":
						if lane == 0 {
							<-r.Context().Done()
							close(canceled[lane])
							return
						}
						if lane == 1 {
							send(`{"type":"error","error":{"type":"rate_limit_error","message":"local rate limit"}}`)
							return
						}
					case "both_rate_limited", "retry_budget_spent_by_backup":
						if lane < 2 {
							wait(started[1])
							send(`{"type":"error","error":{"type":"rate_limit_error","message":"local rate limit"}}`)
							return
						}
					case "primary_empty_before_threshold":
						send(`{"type":"message_stop"}`)
						return
					case "client_cancels", "request_deadline_cancels_both":
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
					if tc.name == "committed_error" {
						send(`{"type":"error","error":{"type":"rate_limit_error","message":"local rate limit"}}`)
						return
					}
					send(`{"type":"content_block_stop","index":0}`)
					send(`{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":3}}`)
					send(`{"type":"message_stop"}`)
				}))
			}
			a, b, third := makeUpstream(0), makeUpstream(1), makeUpstream(2)
			defer a.Close()
			defer b.Close()
			defer third.Close()
			constant.StreamHedgeDelayMS = 70
			constant.StreamingTimeout = 5
			common.RetryTimes = 5
			switch tc.name {
			case "primary_output_timeout_backup_rate_limit":
				constant.StreamingTimeout = 1
			case "disabled":
				constant.StreamHedgeDelayMS = 0
			case "effective_before_threshold", "primary_empty_before_threshold", "primary_rate_limit_before_threshold", "committed_error", "upstream_gateway_timeout":
				constant.StreamHedgeDelayMS = 2000
			case "retry_budget_spent_by_backup":
				common.RetryTimes = 1
			case "no_retry_budget":
				common.RetryTimes = 0
			}
			policy := operation_setting.DefaultChannelRetryPolicy
			if sameChannel {
				constant.StreamHedgeDelayMS = 0
				policy = `[{"stream_hedge":{"delay_ms":70,"retry_same_channel_first":true}}]`
			}
			if tc.name == "json_budget_only_allows_same_channel" {
				common.RetryTimes = 1
			}
			if tc.name == "json_zero_disables_startup_setting" {
				policy = `[{"stream_hedge":{"delay_ms":0}}]`
			}
			// The controller uses the freshly saved snapshot in the same process.
			require.NoError(t, model.UpdateOption(operation_setting.ChannelRetryPolicyOption, policy))
			require.NoError(t, db.Exec("DELETE FROM channels").Error)
			require.NoError(t, db.Exec("DELETE FROM abilities").Error)
			require.NoError(t, db.Exec("DELETE FROM logs").Error)
			high, low, weight := int64(10), int64(0), uint(1)
			require.NoError(t, db.Create(&[]model.Channel{{Id: primaryID, Type: constant.ChannelTypeAnthropic, Name: "primary", Key: "key-a", BaseURL: &a.URL, Group: "hedge-test", Models: "claude-opus-5-5", Status: common.ChannelStatusEnabled, Priority: &high, Weight: &weight}, {Id: backupID, Type: constant.ChannelTypeAnthropic, Name: "backup", Key: "key-b", BaseURL: &b.URL, Group: "hedge-test", Models: "claude-opus-5-5", Status: common.ChannelStatusEnabled, Priority: &low, Weight: &weight}}).Error)
			require.NoError(t, db.Create(&[]model.Ability{{Group: "hedge-test", Model: "claude-opus-5-5", ChannelId: primaryID, Enabled: true, Priority: &high, Weight: 1}, {Group: "hedge-test", Model: "claude-opus-5-5", ChannelId: backupID, Enabled: true, Priority: &low, Weight: 1}}).Error)
			if tc.fallback || tc.name == "retry_budget_spent_by_backup" || tc.name == "committed_error" || tc.name == "request_deadline_cancels_both" {
				last := int64(-1)
				require.NoError(t, db.Create(&model.Channel{Id: fallbackID, Type: constant.ChannelTypeAnthropic, Name: "fallback", Key: "key-c", BaseURL: &third.URL, Group: "hedge-test", Models: "claude-opus-5-5", Status: common.ChannelStatusEnabled, Priority: &last, Weight: &weight}).Error)
				require.NoError(t, db.Create(&model.Ability{Group: "hedge-test", Model: "claude-opus-5-5", ChannelId: fallbackID, Enabled: true, Priority: &last, Weight: 1}).Error)
			}
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
				if tc.name == "request_deadline_cancels_both" {
					common.SetContextKey(c, constant.ContextKeyTokenUpstreamTimeout, 1)
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
			if tc.fallback {
				require.EqualValues(t, 1, calls[2].Load())
				require.JSONEq(t, *bodies[0].Load(), *bodies[2].Load())
			} else {
				require.Zero(t, calls[2].Load())
			}
			waitCanceled := func(lane int) {
				t.Helper()
				select {
				case <-canceled[lane]:
				case <-time.After(time.Second):
					t.Fatal("upstream was not canceled")
				}
			}
			switch tc.name {
			case "client_cancels", "request_deadline_cancels_both":
				waitCanceled(0)
				waitCanceled(1)
			case "backup_wins", "primary_output_timeout_backup_rate_limit", "json_same_channel_wins":
				waitCanceled(0)
			case "json_next_channel_wins":
				waitCanceled(0)
				waitCanceled(1)
			case "json_budget_only_allows_same_channel":
				waitCanceled(1)
			}
			var logs []model.Log
			require.NoError(t, db.Where("type = ?", model.LogTypeConsume).Find(&logs).Error)
			if tc.winner == 0 {
				if tc.name != "no_retry_budget" { // The ordinary path does not use a private stream writer.
					require.NotContains(t, out.Header().Get("Content-Type"), "text/event-stream")
				}
				require.NotContains(t, out.Body.String(), "-preamble")
				require.Empty(t, logs)
			} else {
				label := []string{"primary", "backup", "fallback"}[tc.winner-1]
				require.Contains(t, out.Header().Get("Content-Type"), "text/event-stream")
				require.Equal(t, 1, strings.Count(out.Body.String(), label+"-preamble"))
				require.Equal(t, 1, strings.Count(out.Body.String(), label+"-visible"))
				for _, other := range []string{"primary", "backup", "fallback"} {
					if other != label {
						require.NotContains(t, out.Body.String(), other+"-preamble")
					}
				}
				if tc.name == "committed_error" {
					require.Empty(t, logs)
				} else {
					require.Len(t, logs, 1)
					wantChannelID := primaryID + tc.winner - 1
					if sameChannel && tc.winner > 1 {
						wantChannelID--
					}
					require.Equal(t, wantChannelID, logs[0].ChannelId)
				}
			}
		})
	}
}
