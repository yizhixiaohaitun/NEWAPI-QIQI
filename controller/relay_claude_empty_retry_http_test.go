package controller

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRelayClaudeEmptyStreamHTTP(t *testing.T) {
	oldDB, oldLogDB := model.DB, model.LOG_DB
	oldMain, oldLogType := common.MainDatabaseType(), common.LogDatabaseType()
	oldCache, oldRedis, oldBatch := common.MemoryCacheEnabled, common.RedisEnabled, common.BatchUpdateEnabled
	oldRetry, oldCount, oldLog, oldError := common.RetryTimes, constant.CountToken, common.LogConsumeEnabled, constant.ErrorLogEnabled
	oldHedge, oldFirst, oldIdle := constant.StreamHedgeDelayMS, constant.FirstByteTimeout, constant.StreamingTimeout
	oldRatios, oldPrices := ratio_setting.GroupRatio2JSONString(), ratio_setting.ModelPrice2JSONString()
	oldModelRatio := ratio_setting.ModelRatio2JSONString()
	oldStatuses := operation_setting.AutomaticRetryStatusCodesToString()
	oldKeywords := operation_setting.AutomaticRetryKeywordsToString()
	oldPing := operation_setting.GetGeneralSetting().PingIntervalEnabled
	oldModelSettings := *model_setting.GetGlobalSettings()
	t.Cleanup(func() {
		model.DB, model.LOG_DB = oldDB, oldLogDB
		common.SetDatabaseTypes(oldMain, oldLogType)
		common.MemoryCacheEnabled, common.RedisEnabled, common.BatchUpdateEnabled = oldCache, oldRedis, oldBatch
		common.RetryTimes, constant.CountToken, common.LogConsumeEnabled, constant.ErrorLogEnabled = oldRetry, oldCount, oldLog, oldError
		constant.StreamHedgeDelayMS, constant.FirstByteTimeout, constant.StreamingTimeout = oldHedge, oldFirst, oldIdle
		operation_setting.GetGeneralSetting().PingIntervalEnabled = oldPing
		operation_setting.AutomaticRetryKeywordsFromString(oldKeywords)
		*model_setting.GetGlobalSettings() = oldModelSettings
		require.NoError(t, operation_setting.AutomaticRetryStatusCodesFromString(oldStatuses))
		require.NoError(t, operation_setting.SetChannelRetryPolicy(operation_setting.DefaultChannelRetryPolicy))
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(oldRatios))
		require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(oldPrices))
		require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(oldModelRatio))
	})
	const (
		modelName     = "claude-opus-5-5"
		group         = "claude-empty-test"
		primary       = 437
		alternate     = 477
		emptyMessage  = "upstream stream ended before effective output"
		noDataMessage = "upstream stream ended before producing any data"
		stopFrame     = `{"type":"message_stop"}`
		quota         = 1000000
		userID        = 981
		tokenID       = 982
		tokenKey      = "claude-empty-local"
	)
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	db := openRequestCompatibilitySQLite(t, ":memory:")
	require.NoError(t, db.AutoMigrate(&model.Ability{}, &model.Channel{}, &model.User{}, &model.Token{}, &model.Log{}, &model.ChannelPurityGroup{}, &model.ChannelPurityMember{}))
	model.DB, model.LOG_DB = db, db
	common.MemoryCacheEnabled, common.RedisEnabled, common.BatchUpdateEnabled = false, false, false
	constant.CountToken, common.LogConsumeEnabled, constant.ErrorLogEnabled = false, true, false
	constant.StreamHedgeDelayMS, constant.FirstByteTimeout, constant.StreamingTimeout = 0, 30, 30
	operation_setting.GetGeneralSetting().PingIntervalEnabled = false
	operation_setting.AutomaticRetryKeywordsFromString("")
	model_setting.GetGlobalSettings().AutoChatToResponsesEnabled = false
	model_setting.GetGlobalSettings().PassThroughRequestEnabled = false
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"claude-empty-test":1}`))
	require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(`{"claude-opus-5-5":1}`))
	require.NoError(t, db.Create(&model.User{Id: userID, Username: "claude-empty-test", Quota: quota, Status: common.UserStatusEnabled}).Error)
	require.NoError(t, db.Create(&model.Token{Id: tokenID, UserId: userID, Key: tokenKey, RemainQuota: quota, Status: common.TokenStatusEnabled}).Error)
	service.InitHttpClient()

	for _, tc := range []struct {
		name                                                 string
		ranges, rule, firstEnd                               string
		hedge, unmap, noRoute, pinned, noTimer, cancelClient bool
		budget, status, settledChannel                       int
		closingText                                          string
		backupEnd, requestExtra, override                    string
		paid, keyword, affinityStop, noAccountQuota          bool
	}{
		{name: "mapped_429_before_hedge", paid: true, ranges: "429", hedge: true, budget: 2, status: 200, settledChannel: alternate},
		{name: "mapped_429_without_hedge", firstEnd: "stop_only", ranges: "429", budget: 2, status: 200, settledChannel: alternate},
		{name: "mapped_429_without_first_output_timer", ranges: "429", hedge: true, noTimer: true, budget: 2, status: 200, settledChannel: alternate},
		{name: "mapped_json_retry", ranges: "503", rule: `{"channel_id":437,"status":429,"error_code":"bad_response_body","action":"retry"}`, hedge: true, budget: 2, status: 200, settledChannel: alternate},
		{name: "mapped_keyword_is_still_supported", keyword: true, ranges: "503", hedge: true, budget: 2, status: 200, settledChannel: alternate},
		{name: "mapped_json_stop", keyword: true, ranges: "429", rule: `{"channel_id":437,"status":429,"action":"stop"}`, hedge: true, budget: 2, status: 429, closingText: emptyMessage},
		{name: "unmapped_502_not_in_ranges", ranges: "429", unmap: true, hedge: true, budget: 2, status: 502, closingText: emptyMessage},
		{name: "mapped_no_eligible_alternate", paid: true, ranges: "429", noRoute: true, hedge: true, budget: 2, status: 429, closingText: emptyMessage},
		{name: "unmapped_no_eligible_alternate", ranges: "502", unmap: true, noRoute: true, hedge: true, budget: 2, status: 502, closingText: emptyMessage},
		{name: "zero_retry_budget", firstEnd: "stop_only", ranges: "429", hedge: true, budget: 0, status: 429, closingText: emptyMessage},
		{name: "pinned_channel", firstEnd: "stop_only", ranges: "429", hedge: true, pinned: true, budget: 2, status: 429, closingText: emptyMessage},
		{name: "affinity_forbids_retry", firstEnd: "stop_only", affinityStop: true, ranges: "429", hedge: true, budget: 2, status: 429, closingText: emptyMessage},
		{name: "no_wallet_quota_stops_before_dispatch", noAccountQuota: true, paid: true, ranges: "429", hedge: true, budget: 2, status: 429},
		{name: "only_preamble_then_eof", firstEnd: "preamble_eof", ranges: "502,429", hedge: true, budget: 2, status: 502, closingText: emptyMessage},
		{name: "zero_frame_eof_keeps_stop", firstEnd: "zero_frame_eof", ranges: "429", hedge: true, budget: 2, status: 429, closingText: noDataMessage},
		{name: "retry_does_not_inherit_previous_frame_count", hedge: true, backupEnd: "zero_frame_eof", ranges: "429", budget: 2, status: 502, closingText: noDataMessage},
		{name: "upstream_same_code_and_message_keeps_stop", firstEnd: "upstream_bad_body", ranges: "429", rule: `{"status":429,"error_code":"bad_response_body","action":"retry"}`, hedge: true, budget: 2, status: 429, closingText: emptyMessage},
		{name: "unbuffered_preamble_already_sent", ranges: "429", budget: 2, status: 200, settledChannel: primary, closingText: "primary-only"},
		{name: "malformed_stream_keeps_stop", firstEnd: "malformed", ranges: "429", hedge: true, budget: 2, status: 429},
		{name: "audit_is_not_discarded", firstEnd: "audit", ranges: "429", hedge: true, budget: 2, status: 200, settledChannel: primary, closingText: "fixture-audit"},
		{name: "audit_without_first_output_timer_is_not_discarded", firstEnd: "audit", noTimer: true, ranges: "429", hedge: true, budget: 2, status: 200, settledChannel: primary, closingText: "fixture-audit"},
		{name: "tool_is_not_replayed", firstEnd: "tool", ranges: "429", budget: 2, status: 200, settledChannel: primary, closingText: "fixture-tool"},
		{name: "refusal_is_not_replayed", firstEnd: "refusal", ranges: "429", hedge: true, budget: 2, status: 200, settledChannel: primary, closingText: "fixture-refusal"},
		{name: "stop_details_without_stop_reason_is_not_replayed", firstEnd: "details", ranges: "429", hedge: true, budget: 2, status: 200, settledChannel: primary, closingText: "fixture-details"},
		{name: "client_tool_definition_is_safe_before_output", requestExtra: `,"tools":[{"name":"lookup","input_schema":{"type":"object"}}]`, ranges: "429", hedge: true, budget: 2, status: 200, settledChannel: alternate},
		{name: "provider_tool_request_is_not_replayed", requestExtra: `,"tools":[{"type":"web_search_20250305","name":"web_search"}]`, ranges: "429", hedge: true, budget: 2, status: 429, closingText: emptyMessage},
		{name: "provider_state_added_by_override_is_not_replayed", override: `{"container":"fixture-container"}`, ranges: "429", hedge: true, budget: 2, status: 429, closingText: emptyMessage},
		{name: "hidden_thinking_progress_is_not_an_empty_stream", firstEnd: "hidden_thinking", ranges: "429", hedge: true, budget: 2, status: 200, settledChannel: primary, closingText: `"estimated_tokens":7`},
		{name: "stop_sequence_is_not_an_empty_failure", firstEnd: "stop_sequence", ranges: "429", budget: 2, status: 200, settledChannel: primary, closingText: `"stop_reason":"stop_sequence"`},
		{name: "text_before_eof_is_not_replayed", firstEnd: "text_eof", ranges: "429", hedge: true, budget: 2, status: 200, settledChannel: primary, closingText: "fixture-partial"},
		{name: "client_cancel_does_not_retry", ranges: "429", hedge: true, budget: 2, cancelClient: true, status: 499},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, table := range []string{"logs", "abilities", "channels"} {
				require.NoError(t, db.Exec("DELETE FROM "+table).Error)
			}
			common.RetryTimes = tc.budget
			price := `{"claude-opus-5-5":0}`
			if tc.paid {
				price = `{}` // use the fixture's per-token price, not a per-call fee
			}
			require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(price))
			walletQuota := quota
			if tc.noAccountQuota {
				walletQuota = 0
			}
			require.NoError(t, db.Model(&model.User{}).Where("id = ?", userID).Update("quota", walletQuota).Error)
			require.NoError(t, db.Model(&model.Token{}).Where("id = ?", tokenID).Update("remain_quota", quota).Error)
			keywords := ""
			if tc.keyword {
				keywords = emptyMessage
			}
			operation_setting.AutomaticRetryKeywordsFromString(keywords)
			require.NoError(t, operation_setting.AutomaticRetryStatusCodesFromString(tc.ranges))
			delay := 0
			if tc.hedge {
				delay = 15000
			}
			policy := fmt.Sprintf(`[{"stream_hedge":{"delay_ms":%d,"retry_same_channel_first":true}}`, delay)
			if tc.rule != "" {
				policy += "," + tc.rule
			}
			require.NoError(t, operation_setting.SetChannelRetryPolicy(policy+"]"))

			// Request lifetime is shorter than the delayed hedge, so an alternate
			// request proves immediate failover, not a fifteen-second backup.
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			type dispatch struct {
				body          string
				wallet, token int
				err           error
			}
			aCalls, bCalls := make(chan dispatch, 8), make(chan dispatch, 8)
			balances := func() (int, int, error) {
				var user model.User
				var token model.Token
				if err := db.First(&user, userID).Error; err != nil {
					return 0, 0, err
				}
				if err := db.First(&token, tokenID).Error; err != nil {
					return 0, 0, err
				}
				return user.Quota, token.RemainQuota, nil
			}
			upstream := func(isPrimary bool, calls chan<- dispatch) *httptest.Server {
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					body, err := io.ReadAll(r.Body)
					if err != nil {
						w.WriteHeader(http.StatusBadRequest)
						return
					}
					seen := dispatch{body: string(body)}
					if tc.paid {
						seen.wallet, seen.token, seen.err = balances()
					}
					calls <- seen
					w.Header().Set("Content-Type", "text/event-stream")
					if (isPrimary && tc.firstEnd == "zero_frame_eof") || (!isPrimary && tc.backupEnd == "zero_frame_eof") {
						_, _ = io.WriteString(w, ": heartbeat\n\n")
						return
					}
					send := func(frame string) { _, _ = fmt.Fprintf(w, "data: %s\n\n", frame); w.(http.Flusher).Flush() }
					if isPrimary && tc.firstEnd == "stop_only" {
						send(stopFrame)
						return
					}
					id := "backup-only"
					if isPrimary {
						id = "primary-only"
					}
					auditField := ""
					if isPrimary && tc.firstEnd == "audit" {
						auditField = `,"safeguard_results":{"provider":"fixture-audit"}`
					}
					send(fmt.Sprintf(`{"type":"message_start","message":{"id":%q,"model":%q,"content":[],"usage":{"input_tokens":7,"output_tokens":1}%s}}`, id, modelName, auditField))
					send(`{"type":"ping"}`)
					if isPrimary {
						if tc.cancelClient {
							cancel()
							return
						}
						switch tc.firstEnd {
						case "upstream_bad_body":
							send(fmt.Sprintf(`{"type":"error","error":{"type":"bad_response_body","message":%q}}`, emptyMessage))
							return
						case "malformed":
							send(`{"type":`)
							return
						case "tool":
							send(`{"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"fixture-tool","name":"lookup","input":{}}}`)
							send(`{"type":"content_block_stop","index":0}`)
							send(`{"type":"message_delta","delta":{"stop_reason":"tool_use"},"usage":{"output_tokens":2}}`)
						case "refusal":
							send(`{"type":"message_delta","delta":{"stop_reason":"refusal","stop_details":{"explanation":"fixture-refusal"}},"usage":{"output_tokens":2}}`)
						case "details":
							send(`{"type":"message_delta","delta":{"stop_details":{"explanation":"fixture-details"}},"usage":{"output_tokens":2}}`)
						case "hidden_thinking":
							send(`{"type":"content_block_start","index":0,"content_block":{"type":"thinking","thinking":""}}`)
							send(`{"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","estimated_tokens":7}}`)
							send(`{"type":"content_block_stop","index":0}`)
							send(`{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":7}}`)
						case "stop_sequence":
							send(`{"type":"message_delta","delta":{"stop_reason":"stop_sequence","stop_sequence":"fixture-stop"},"usage":{"output_tokens":1}}`)
						case "text_eof":
							send(`{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`)
							send(`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"fixture-partial"}}`)
							return
						case "audit":
						default:
							send(`{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`)
							send(`{"type":"content_block_stop","index":0}`)
							if tc.firstEnd == "preamble_eof" {
								return
							}
						}
					} else {
						send(`{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`)
						send(`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"backup-completed"}}`)
						send(`{"type":"content_block_stop","index":0}`)
						send(`{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":3}}`)
					}
					send(stopFrame)
				}))
			}
			a, b := upstream(true, aCalls), upstream(false, bCalls)
			defer a.Close()
			defer b.Close()
			mapping := `{"502":"429"}`
			if tc.unmap {
				mapping = "{}"
			}
			backupGroup := group
			if tc.noRoute {
				backupGroup = "not-the-token-group"
			}
			high, low, weight := int64(10), int64(0), uint(1)
			require.NoError(t, db.Create(&[]model.Channel{
				{Id: primary, Type: constant.ChannelTypeAnthropic, Name: "a", Key: "local-a", BaseURL: &a.URL, Group: group, Models: modelName, Status: common.ChannelStatusEnabled, Priority: &high, Weight: &weight, StatusCodeMapping: &mapping, ParamOverride: common.GetPointer(tc.override)},
				{Id: alternate, Type: constant.ChannelTypeAnthropic, Name: "b", Key: "local-b", BaseURL: &b.URL, Group: backupGroup, Models: modelName, Status: common.ChannelStatusEnabled, Priority: &low, Weight: &weight},
			}).Error)
			require.NoError(t, db.Create(&[]model.Ability{
				{Group: group, Model: modelName, ChannelId: primary, Enabled: true, Priority: &high, Weight: 1},
				{Group: backupGroup, Model: modelName, ChannelId: alternate, Enabled: true, Priority: &low, Weight: 1},
			}).Error)
			router := gin.New()
			router.POST("/v1/messages", func(c *gin.Context) {
				defer common.CleanupBodyStorage(c)
				common.SetContextKey(c, constant.ContextKeyUsingGroup, group)
				common.SetContextKey(c, constant.ContextKeyTokenGroup, group)
				common.SetContextKey(c, constant.ContextKeyUserSetting, dto.UserSetting{BillingPreference: "wallet_only", AcceptUnsetRatioModel: true})
				common.SetContextKey(c, constant.ContextKeyUserId, userID)
				common.SetContextKey(c, constant.ContextKeyUserQuota, walletQuota)
				common.SetContextKey(c, constant.ContextKeyTokenId, tokenID)
				common.SetContextKey(c, constant.ContextKeyTokenKey, tokenKey)
				if tc.noTimer {
					common.SetContextKey(c, constant.ContextKeyTokenFirstByteTimeout, common.GetPointer(0))
				}
				if tc.pinned {
					common.SetContextKey(c, constant.ContextKeyTokenSpecificChannelId, "437")
				}
				c.Next()
			}, middleware.Distribute(), func(c *gin.Context) {
				if tc.affinityStop {
					c.Set("channel_affinity_skip_retry_on_failure", true)
				}
				Relay(c, types.RelayFormatClaude)
			})
			payload := `{"model":"claude-opus-5-5","max_tokens":64,"stream":true,"messages":[{"role":"user","content":"fixture prompt"}]` + tc.requestExtra + "}"
			req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(payload)).WithContext(ctx)
			req.Header.Set("Content-Type", "application/json")
			out := httptest.NewRecorder()
			router.ServeHTTP(out, req)
			body := out.Body.String()
			assert.Equal(t, tc.status, out.Code, body)
			var original dispatch
			if tc.noAccountQuota {
				assert.Empty(t, aCalls)
			} else {
				require.Len(t, aCalls, 1)
				original = <-aCalls
				if tc.override != "" {
					assert.Contains(t, original.body, `"container":"fixture-container"`)
				}
				if tc.paid {
					require.NoError(t, original.err)
					assert.Less(t, original.wallet, quota, "actual precharge before the primary dispatch")
					assert.Equal(t, original.wallet, original.token)
				}
			}
			if tc.settledChannel == alternate || tc.backupEnd != "" {
				require.Len(t, bCalls, 1)
				backup := <-bCalls
				require.JSONEq(t, original.body, backup.body)
				if tc.paid {
					require.NoError(t, backup.err)
					assert.Equal(t, original.wallet, backup.wallet, "retry must not precharge again")
					assert.Equal(t, original.token, backup.token)
				}
			} else {
				assert.Empty(t, bCalls)
			}
			if tc.settledChannel == alternate {
				assert.Contains(t, body, "backup-only")
				assert.Contains(t, body, "backup-completed")
				assert.Contains(t, body, `"type":"message_stop"`)
			}
			if tc.settledChannel != primary {
				assert.NotContains(t, body, "primary-only")
			}
			if tc.firstEnd == "upstream_bad_body" {
				assert.Contains(t, body, `"type":"bad_response_body"`, "keep the upstream error type")
			} else if tc.closingText == emptyMessage && tc.status == 429 {
				assert.Contains(t, body, `"type":"rate_limit_error"`)
			}
			if tc.closingText != "" {
				assert.Contains(t, body, tc.closingText)
			}
			var logs []model.Log
			require.NoError(t, db.Where("type = ?", model.LogTypeConsume).Find(&logs).Error)
			if tc.settledChannel == 0 {
				assert.Contains(t, out.Header().Get("Content-Type"), "application/json")
				assert.NotContains(t, body, "event:")
				assert.Empty(t, logs)
			} else {
				assert.Contains(t, out.Header().Get("Content-Type"), "text/event-stream")
				require.Len(t, logs, 1)
				assert.Equal(t, tc.settledChannel, logs[0].ChannelId)
			}
			if tc.paid && tc.settledChannel == alternate {
				wantQuota := common.QuotaFromFloat(7 + 3*ratio_setting.GetCompletionRatio(modelName))
				assert.Positive(t, logs[0].Quota)
				assert.EqualValues(t, wantQuota, logs[0].Quota)
				wallet, token, err := balances()
				require.NoError(t, err)
				assert.Equal(t, quota-int(wantQuota), wallet)
				assert.Equal(t, wallet, token)
			} else if tc.paid {
				require.EventuallyWithT(t, func(collect *assert.CollectT) {
					wallet, token, err := balances()
					assert.NoError(collect, err)
					assert.Equal(collect, walletQuota, wallet)
					assert.Equal(collect, quota, token)
				}, time.Second, 5*time.Millisecond, "terminal failure must refund both balances")
			}
		})
	}
}
