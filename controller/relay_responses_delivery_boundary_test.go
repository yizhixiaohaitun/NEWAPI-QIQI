package controller

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
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
	"gorm.io/gorm"
)

const (
	responsesBoundaryGroup   = "responses-delivery-boundary"
	responsesBoundaryBalance = 10000
	responsesBoundaryFee     = 500
	responsesBoundaryUser    = 701
	responsesBoundaryToken   = 702
	responsesBoundaryA       = 703
	responsesBoundaryB       = 704
	responsesBoundaryInput   = `{"model":"gpt-6.1-terra","input":"one local fixture","store":false,"stream":true}`
	responsesBoundaryDelta   = `data: {"type":"response.output_text.delta","delta":"delivered text"}` + "\n\n"
	responsesBoundaryFinish  = `data: {"type":"response.completed","response":{"status":"completed","usage":{"input_tokens":5,"output_tokens":1,"total_tokens":6}}}` + "\n\n"
)

type responsesBoundaryFixture struct {
	a, b           string
	holdA          bool
	holdB          bool
	paid           bool
	retryDisabled  bool
	streamRetries  int
	failWriteBytes *int // nil: normal writer; non-nil: cap first actual write and return an error
}

type responsesBoundaryResult struct {
	status  int
	headers http.Header
	body    string
	calls   []string
	db      *gorm.DB
	writer  *responsesBoundaryShortWriter
}

var errResponsesBoundaryWrite = errors.New("test downstream partial write")

type responsesBoundaryShortWriter struct {
	gin.ResponseWriter
	request       context.Context
	accepted      int
	attempts      int
	firstSize     int
	firstPrefix   string
	liveAtFailure bool
}

func (w *responsesBoundaryShortWriter) Write(p []byte) (int, error) {
	w.attempts++
	if w.attempts != 1 {
		// Allow a second write to expose any incorrect replay, append, or fallback.
		return w.ResponseWriter.Write(p)
	}
	w.liveAtFailure = w.request.Err() == nil
	w.firstSize = len(p)
	w.firstPrefix = string(p[:min(len(p), 100)])
	if w.accepted == 0 {
		return 0, errResponsesBoundaryWrite
	}
	n, _ := w.ResponseWriter.Write(p[:min(w.accepted, len(p))])
	return n, errResponsesBoundaryWrite
}

func (w *responsesBoundaryShortWriter) WriteString(s string) (int, error) {
	return w.Write([]byte(s))
}

func runResponsesBoundaryRelay(t *testing.T, f responsesBoundaryFixture) responsesBoundaryResult {
	t.Helper()
	oldMode := gin.Mode()
	oldDB, oldLogDB := model.DB, model.LOG_DB
	oldMainType, oldLogType := common.MainDatabaseType(), common.LogDatabaseType()
	oldCache, oldRedis, oldBatch := common.MemoryCacheEnabled, common.RedisEnabled, common.BatchUpdateEnabled
	oldRetry, oldCount, oldLogs, oldExport := common.RetryTimes, constant.CountToken, common.LogConsumeEnabled, common.DataExportEnabled
	oldTimeout, oldErrorLog, oldQuota := constant.StreamingTimeout, constant.ErrorLogEnabled, common.QuotaPerUnit
	oldRatios, oldPrices := ratio_setting.GroupRatio2JSONString(), ratio_setting.ModelPrice2JSONString()
	oldFree, oldPing := operation_setting.GetQuotaSetting().EnableFreeModelPreConsume, operation_setting.GetGeneralSetting().PingIntervalEnabled
	oldRanges, oldKeywords := operation_setting.AutomaticRetryStatusCodeRanges, operation_setting.AutomaticRetryKeywordsToString()
	qiqi := operation_setting.GetQiqiSetting()
	oldQiqi := *qiqi
	t.Cleanup(func() {
		gin.SetMode(oldMode)
		model.DB, model.LOG_DB = oldDB, oldLogDB
		common.SetDatabaseTypes(oldMainType, oldLogType)
		common.MemoryCacheEnabled, common.RedisEnabled, common.BatchUpdateEnabled = oldCache, oldRedis, oldBatch
		common.RetryTimes, constant.CountToken, common.LogConsumeEnabled, common.DataExportEnabled = oldRetry, oldCount, oldLogs, oldExport
		constant.StreamingTimeout, constant.ErrorLogEnabled, common.QuotaPerUnit = oldTimeout, oldErrorLog, oldQuota
		assert.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(oldRatios))
		assert.NoError(t, ratio_setting.UpdateModelPriceByJSONString(oldPrices))
		operation_setting.GetQuotaSetting().EnableFreeModelPreConsume = oldFree
		operation_setting.GetGeneralSetting().PingIntervalEnabled = oldPing
		operation_setting.AutomaticRetryStatusCodeRanges = oldRanges
		operation_setting.AutomaticRetryKeywordsFromString(oldKeywords)
		*qiqi = oldQiqi
	})

	gin.SetMode(gin.TestMode)
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	db := openRequestCompatibilitySQLite(t, ":memory:")
	require.NoError(t, db.AutoMigrate(&model.Ability{}, &model.Channel{}, &model.User{}, &model.Token{}, &model.Log{}, &model.ChannelPurityGroup{}, &model.ChannelPurityMember{}))
	model.DB, model.LOG_DB = db, db
	common.MemoryCacheEnabled, common.RedisEnabled, common.BatchUpdateEnabled = false, false, false
	common.RetryTimes, constant.CountToken, common.LogConsumeEnabled, common.DataExportEnabled = 0, false, true, false
	constant.StreamingTimeout, constant.ErrorLogEnabled, common.QuotaPerUnit = 30, false, 500000
	operation_setting.GetQuotaSetting().EnableFreeModelPreConsume = false
	operation_setting.GetGeneralSetting().PingIntervalEnabled = false
	operation_setting.AutomaticRetryStatusCodeRanges = []operation_setting.StatusCodeRange{{Start: 429, End: 429}}
	operation_setting.AutomaticRetryKeywordsFromString("")
	qiqi.ResponsesStreamErrorRetryEnabled = !f.retryDisabled
	qiqi.ResponsesStreamErrorRetryTimes = f.streamRetries
	price, precharge := 0.0, 0
	if f.paid {
		price, precharge = 0.001, responsesBoundaryFee
		// Credible negative control: a local error would match both configured
		// paths if it were accidentally returned as an upstream 500.
		common.RetryTimes = 1
		operation_setting.AutomaticRetryStatusCodeRanges = []operation_setting.StatusCodeRange{{Start: 500, End: 599}}
		operation_setting.AutomaticRetryKeywordsFromString(errResponsesBoundaryWrite.Error())
		for _, query := range []string{
			`CREATE TABLE response_quota_moves (account TEXT, delta INTEGER)`,
			`CREATE TRIGGER response_wallet_move AFTER UPDATE OF quota ON users WHEN OLD.quota != NEW.quota BEGIN INSERT INTO response_quota_moves VALUES ('wallet', NEW.quota - OLD.quota); END`,
			`CREATE TRIGGER response_token_move AFTER UPDATE OF remain_quota ON tokens WHEN OLD.remain_quota != NEW.remain_quota BEGIN INSERT INTO response_quota_moves VALUES ('token', NEW.remain_quota - OLD.remain_quota); END`,
		} {
			require.NoError(t, db.Exec(query).Error)
		}
	}
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"responses-delivery-boundary":1}`))
	require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(fmt.Sprintf(`{"gpt-6.1-terra":%f}`, price)))
	require.NoError(t, db.Create(&model.User{Id: responsesBoundaryUser, Username: "responses-boundary-test", Quota: responsesBoundaryBalance, Status: common.UserStatusEnabled}).Error)
	require.NoError(t, db.Create(&model.Token{Id: responsesBoundaryToken, UserId: responsesBoundaryUser, Key: "responses-boundary-loopback", RemainQuota: responsesBoundaryBalance, Status: common.TokenStatusEnabled}).Error)
	service.InitHttpClient()

	var mu sync.Mutex
	var calls []string
	var waited []<-chan struct{}
	upstream := func(name, frames string, hold bool) *httptest.Server {
		stopped := make(chan struct{})
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			mu.Lock()
			calls = append(calls, name)
			if hold {
				waited = append(waited, stopped)
			}
			mu.Unlock()
			assert.Equal(t, "/v1/responses", r.URL.Path)
			body, readErr := io.ReadAll(r.Body)
			assert.NoError(t, readErr)
			assert.JSONEq(t, responsesBoundaryInput, string(body))
			if f.paid {
				var user model.User
				var token model.Token
				assert.NoError(t, db.First(&user, responsesBoundaryUser).Error)
				assert.NoError(t, db.First(&token, responsesBoundaryToken).Error)
				assert.Equal(t, responsesBoundaryBalance-precharge, user.Quota, "wallet precharged before upstream call")
				assert.Equal(t, responsesBoundaryBalance-precharge, token.RemainQuota, "token precharged before upstream call")
				assert.Equal(t, precharge, token.UsedQuota)
			}
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = io.WriteString(w, `data: {"type":"response.created","response":{"id":"attempt-`+name+`","status":"in_progress"}}`+"\n\n"+frames)
			w.(http.Flusher).Flush()
			if hold {
				<-r.Context().Done() // no upstream EOF unless the relay cancels
				close(stopped)
			}
		}))
		t.Cleanup(server.Close)
		return server
	}
	if f.b == "" {
		f.b = responsesBoundaryDelta + responsesBoundaryFinish
	}
	a, b := upstream("A", f.a, f.holdA), upstream("B", f.b, f.holdB)
	high, low, weight := int64(10), int64(0), uint(1)
	require.NoError(t, db.Create(&[]model.Channel{
		{Id: responsesBoundaryA, Type: constant.ChannelTypeOpenAI, Name: "local-A", Key: "loopback-A", BaseURL: &a.URL, Group: responsesBoundaryGroup, Models: "gpt-6.1-terra", Status: common.ChannelStatusEnabled, Priority: &high, Weight: &weight},
		{Id: responsesBoundaryB, Type: constant.ChannelTypeOpenAI, Name: "local-B", Key: "loopback-B", BaseURL: &b.URL, Group: responsesBoundaryGroup, Models: "gpt-6.1-terra", Status: common.ChannelStatusEnabled, Priority: &low, Weight: &weight},
	}).Error)
	require.NoError(t, db.Create(&[]model.Ability{
		{Group: responsesBoundaryGroup, Model: "gpt-6.1-terra", ChannelId: responsesBoundaryA, Enabled: true, Priority: &high, Weight: 1},
		{Group: responsesBoundaryGroup, Model: "gpt-6.1-terra", ChannelId: responsesBoundaryB, Enabled: true, Priority: &low, Weight: 1},
	}).Error)

	var shortWriter *responsesBoundaryShortWriter
	router := gin.New()
	router.POST("/v1/responses", func(c *gin.Context) {
		common.SetContextKey(c, constant.ContextKeyUsingGroup, responsesBoundaryGroup)
		common.SetContextKey(c, constant.ContextKeyTokenGroup, responsesBoundaryGroup)
		common.SetContextKey(c, constant.ContextKeyUserSetting, dto.UserSetting{BillingPreference: "wallet_only", AcceptUnsetRatioModel: true})
		common.SetContextKey(c, constant.ContextKeyUserId, responsesBoundaryUser)
		common.SetContextKey(c, constant.ContextKeyUserQuota, responsesBoundaryBalance)
		common.SetContextKey(c, constant.ContextKeyTokenId, responsesBoundaryToken)
		common.SetContextKey(c, constant.ContextKeyTokenKey, "responses-boundary-loopback")
		c.Set("token_quota", 0)
		c.Set(common.RequestIdKey, "responses-boundary-local")
		if f.failWriteBytes != nil {
			shortWriter = &responsesBoundaryShortWriter{ResponseWriter: c.Writer, request: c.Request.Context(), accepted: *f.failWriteBytes}
			c.Writer = shortWriter
		}
		c.Next()
	}, middleware.Distribute(), func(c *gin.Context) { Relay(c, types.RelayFormatOpenAIResponses) })

	// A bounded live client makes a missed stop visible without hanging the suite.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(responsesBoundaryInput)).WithContext(ctx)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	require.NoError(t, ctx.Err(), "relay must finish before the client timeout")
	mu.Lock()
	actualCalls := append([]string(nil), calls...)
	waitForCancel := append([]<-chan struct{}(nil), waited...)
	mu.Unlock()
	for _, stopped := range waitForCancel {
		select {
		case <-stopped:
		case <-ctx.Done():
			t.Fatal("held-open upstream did not observe relay cancellation")
		}
	}
	return responsesBoundaryResult{status: rec.Code, headers: rec.Result().Header, body: rec.Body.String(), calls: actualCalls, db: db, writer: shortWriter}
}

func TestRelayHTTPResponsesTerminalHeaderFallback(t *testing.T) {
	t.Run("code_only_rate_limit_retries_without_leaking_prelude", func(t *testing.T) {
		r := runResponsesBoundaryRelay(t, responsesBoundaryFixture{a: "event: error\n: upstream comment\ndata: {\"code\":429}\n\n", holdA: true, streamRetries: 1})
		assert.Equal(t, []string{"A", "B"}, r.calls)
		assert.Equal(t, http.StatusOK, r.status)
		assert.Contains(t, r.body, "attempt-B")
		assert.NotContains(t, r.body, "attempt-A")
		assert.NotContains(t, r.body, `"code":429`)
		assert.Contains(t, r.body, "response.completed")
	})
	for _, tc := range []struct{ name, data, want string }{
		{"typed_notification", "event: error\ndata: {\"type\":\"rate_limits.updated\",\"code\":429,\"message\":\"rate window updated\"}\n\n", "rate window updated"},
		{"typed_output", "event: response.failed\ndata: {\"type\":\"response.output_text.delta\",\"delta\":\"error response.failed forbidden to access the private group\"}\n\n", "error response.failed forbidden to access the private group"},
		{"explicit_empty_type", "event: error\ndata: {\"type\":\"\",\"code\":429,\"message\":\"explicitly empty\"}\n", "explicitly empty"},
		{"explicit_null_type", "event: response.error\ndata: {\"type\":null,\"code\":429,\"message\":\"explicitly null\"}\n", "explicitly null"},
		{"blank_resets_name", "event: error\n\ndata: {\"code\":429,\"message\":\"different event\"}\n", "different event"},
		{"consumption_resets_name_without_blank", "event: error\ndata: {\"type\":\"response.in_progress\"}\ndata: {\"code\":429,\"message\":\"next data line\"}\n", "next data line"},
		{"different_case_header", "event: ERROR\ndata: {\"code\":429,\"message\":\"distinct event name\"}\n", "distinct event name"},
		{"nonterminal_header", "event: response.not_error\ndata: {\"code\":429,\"message\":\"not terminal\"}\n", "not terminal"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Also explicitly verify that untyped/unknown payloads keep the existing
			// conservative commit behavior; the later real output must survive.
			r := runResponsesBoundaryRelay(t, responsesBoundaryFixture{a: tc.data + responsesBoundaryDelta + responsesBoundaryFinish, streamRetries: 1})
			assert.Equal(t, []string{"A"}, r.calls)
			assert.Equal(t, http.StatusOK, r.status)
			assert.Contains(t, r.body, tc.want)
			assert.Contains(t, r.body, "delivered text")
			assert.Contains(t, r.body, "response.completed")
		})
	}
	t.Run("audit_and_error_once_without_waiting_for_EOF", func(t *testing.T) {
		failure := "event: response.error\ndata: {\"safeguard_results\":[],\"code\":429,\"message\":\"audit terminal\"}\n\n"
		r := runResponsesBoundaryRelay(t, responsesBoundaryFixture{a: failure + failure + responsesBoundaryDelta, holdA: true, streamRetries: 1})
		assert.Equal(t, []string{"A"}, r.calls)
		assert.Equal(t, http.StatusOK, r.status)
		assert.Equal(t, 1, strings.Count(r.body, `"safeguard_results":[]`))
		assert.Equal(t, 1, strings.Count(r.body, "event: response.error\n"))
		assert.NotContains(t, r.body, "delivered text")
	})
}

func TestRelayHTTPResponsesPublicErrorCodes(t *testing.T) {
	service.InitTokenEncoders()
	type publicError struct {
		Code    json.RawMessage `json:"code"`
		Message string          `json:"message"`
	}
	for _, tc := range []struct {
		name, event, data, wantCode string
		group                       bool
		status                      int
	}{
		{"nested_group_number", "response.failed", `{"type":"response.failed","response":{"error":{"code":403,"message":"无权访问请求的上游分组 private-group","param":"private-param"},"id":"private-id"}}`, `403`, true, http.StatusBadGateway},
		{"nested_group_large_number", "response.failed", `{"response":{"error":{"code":9007199254740993,"message":"forbidden to access the private group","param":"private-param"},"id":"private-id"}}`, `9007199254740993`, true, http.StatusBadGateway},
		{"flat_group_string", "response.error", `{"code":"Permission_Denied","message":"forbidden to access the private group","param":"private-param"}`, `"Permission_Denied"`, true, http.StatusBadGateway},
		{"group_unknown_string_stays_redacted", "response.error", `{"code":"private-group-code","message":"forbidden to access the private group"}`, ``, true, http.StatusBadGateway},
		{"flat_code_only_header", "error", `{"code":429}`, `429`, false, http.StatusTooManyRequests},
		{"nested_code_only", "response.failed", `{"response":{"error":{"code":9007199254740993}}}`, `9007199254740993`, false, http.StatusBadGateway},
		{"nested_code_only_without_terminal_header", "", `{"error":{"code":429}}`, `429`, false, http.StatusTooManyRequests},
		{"flat_numeric_string", "response.error", `{"code":"429"}`, `"429"`, false, http.StatusTooManyRequests},
	} {
		t.Run(tc.name, func(t *testing.T) {
			frame := "data: " + tc.data + "\n\n"
			if tc.event != "" {
				frame = "event: " + tc.event + "\n" + frame
			}
			for _, mode := range []string{"http_after_budget", "committed_sse", "retry_disabled_sse"} {
				t.Run(mode, func(t *testing.T) {
					var got publicError
					var r responsesBoundaryResult
					if mode == "http_after_budget" {
						r = runResponsesBoundaryRelay(t, responsesBoundaryFixture{a: frame, b: frame, holdA: true, holdB: true, streamRetries: 1})
						require.Equal(t, []string{"A", "B"}, r.calls, r.body)
						require.Equal(t, tc.status, r.status, r.body)
						assert.Contains(t, r.headers.Get("Content-Type"), "application/json")
						assert.NotContains(t, r.body, "attempt-")
						var envelope struct{ Error publicError }
						require.NoError(t, common.UnmarshalJsonStr(r.body, &envelope))
						got = envelope.Error
					} else {
						prefix := responsesBoundaryDelta
						if mode == "retry_disabled_sse" {
							prefix = ""
						}
						r = runResponsesBoundaryRelay(t, responsesBoundaryFixture{a: prefix + frame + frame + responsesBoundaryFinish, holdA: true, retryDisabled: mode == "retry_disabled_sse", streamRetries: 1})
						require.Equal(t, []string{"A"}, r.calls)
						require.Equal(t, http.StatusOK, r.status, r.body)
						assert.Contains(t, r.headers.Get("Content-Type"), "text/event-stream")
						assert.NotContains(t, r.body, "response.completed")
						if prefix != "" {
							assert.Contains(t, r.body, "delivered text")
						}
						if tc.event != "" {
							assert.Equal(t, 1, strings.Count(r.body, "event: "+tc.event+"\n"))
						}
						var count int
						for _, line := range strings.Split(r.body, "\n") {
							data, ok := strings.CutPrefix(line, "data: ")
							if !ok {
								continue
							}
							var payload struct {
								publicError
								Error    *publicError
								Response struct{ Error *publicError }
							}
							require.NoError(t, common.UnmarshalJsonStr(data, &payload))
							switch {
							case payload.Response.Error != nil:
								got = *payload.Response.Error
							case payload.Error != nil:
								got = *payload.Error
							case len(payload.Code) > 0 || payload.Message != "":
								got = payload.publicError
							default:
								continue
							}
							count++
						}
						require.Equal(t, 1, count, r.body)
					}
					if tc.wantCode == "" {
						assert.Contains(t, []string{"", "null"}, string(got.Code))
					} else {
						assert.Equal(t, tc.wantCode, string(got.Code), "public JSON must retain the unrounded, uncoerced code")
					}
					if tc.group {
						assert.Contains(t, got.Message, "无权访问请求的上游分组")
						for _, secret := range []string{"private group", "private-group", "private-param", "private-id"} {
							assert.NotContains(t, r.body, secret)
						}
					}
				})
			}
		})
	}
}

func TestRelayHTTPResponsesPaidDeliveryBoundary(t *testing.T) {
	service.InitTokenEncoders()
	tail := responsesBoundaryDelta + responsesBoundaryFinish
	for _, tc := range []struct {
		name string
		auto bool
		zero bool
	}{
		{name: "explicit_commit_partial_write"},
		{name: "automatic_4MiB_commit_partial_write", auto: true},
		{name: "no_retry_zero_byte_write", zero: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			frames, accepted := tail, 17
			if tc.auto {
				progress := `data: {"type":"response.in_progress","padding":"` + strings.Repeat("x", 3<<19) + "\"}\n\n"
				frames = strings.Repeat(progress, 3) + frames
			}
			if tc.zero {
				accepted = 0
			}
			r := runResponsesBoundaryRelay(t, responsesBoundaryFixture{a: frames, holdA: true, paid: true, retryDisabled: tc.zero, streamRetries: 1, failWriteBytes: &accepted})
			require.Equal(t, []string{"A"}, r.calls, "never replay the original request")
			require.True(t, r.writer.liveAtFailure, "not the context-canceled path")
			require.Equal(t, 1, r.writer.attempts, "no later stream write or HTTP error append")
			require.Greater(t, r.writer.firstSize, accepted)
			if tc.auto {
				assert.Greater(t, r.writer.firstSize, 3<<20, "first write must be the automatically flushed prelude")
				assert.LessOrEqual(t, r.writer.firstSize, 4<<20)
			} else {
				assert.Less(t, r.writer.firstSize, 512, "first write is the small prelude")
			}
			// The unbuffered renderer writes the newline separately. The buffered
			// paths must start with the complete event line in the first write.
			firstEvent := "event: response.created"
			if !tc.zero {
				firstEvent += "\n"
			}
			assert.True(t, strings.HasPrefix(r.writer.firstPrefix, firstEvent), "first write: %q", r.writer.firstPrefix)
			assert.Equal(t, r.writer.firstPrefix[:accepted], r.body)
			assertResponsesBoundaryBilling(t, r, true, false)
		})
	}
	t.Run("normal_completion_settles_once", func(t *testing.T) {
		r := runResponsesBoundaryRelay(t, responsesBoundaryFixture{a: tail, paid: true})
		assert.Equal(t, []string{"A"}, r.calls)
		assert.Contains(t, r.body, "delivered text")
		assert.Contains(t, r.body, "response.completed")
		assertResponsesBoundaryBilling(t, r, false, true)
	})
	t.Run("delivered_upstream_terminal_settles_partial_usage", func(t *testing.T) {
		err := "event: response.failed\ndata: {\"type\":\"response.failed\",\"response\":{\"error\":{\"code\":\"context_length_exceeded\",\"message\":\"upstream ended after delivery\"}}}\n\n"
		r := runResponsesBoundaryRelay(t, responsesBoundaryFixture{a: responsesBoundaryDelta + err + err + responsesBoundaryFinish, holdA: true, paid: true, streamRetries: 1})
		assert.Equal(t, []string{"A"}, r.calls)
		assert.Contains(t, r.body, "delivered text")
		assert.Equal(t, 1, strings.Count(r.body, "event: response.failed\n"))
		assert.NotContains(t, r.body, "response.completed")
		assertResponsesBoundaryBilling(t, r, false, false)
	})
}

func assertResponsesBoundaryBilling(t *testing.T, r responsesBoundaryResult, refunded, complete bool) {
	t.Helper()
	wantBalance, wantUsed, wantRequests := responsesBoundaryBalance-responsesBoundaryFee, responsesBoundaryFee, 1
	deltas := []int{-responsesBoundaryFee}
	if refunded {
		wantBalance, wantUsed, wantRequests = responsesBoundaryBalance, 0, 0
		deltas = append(deltas, responsesBoundaryFee)
		// Wallet refunds are asynchronous; wait for both actual balance writes.
		require.Eventually(t, func() bool {
			var n int64
			return r.db.Table("response_quota_moves").Where("delta > 0").Count(&n).Error == nil && n >= 2
		}, 2*time.Second, 10*time.Millisecond, "wallet and token must both be refunded")
	}
	for _, account := range []string{"wallet", "token"} {
		var actual []int
		require.NoError(t, r.db.Table("response_quota_moves").Where("account = ?", account).Order("rowid").Pluck("delta", &actual).Error)
		assert.Equal(t, deltas, actual, account+" must only be debited or refunded once")
	}
	var user model.User
	var token model.Token
	var a, b model.Channel
	require.NoError(t, r.db.First(&user, responsesBoundaryUser).Error)
	require.NoError(t, r.db.First(&token, responsesBoundaryToken).Error)
	require.NoError(t, r.db.First(&a, responsesBoundaryA).Error)
	require.NoError(t, r.db.First(&b, responsesBoundaryB).Error)
	assert.Equal(t, wantBalance, user.Quota)
	assert.Equal(t, wantUsed, user.UsedQuota)
	assert.Equal(t, wantRequests, user.RequestCount)
	assert.Equal(t, wantBalance, token.RemainQuota)
	assert.Equal(t, wantUsed, token.UsedQuota)
	assert.EqualValues(t, wantUsed, a.UsedQuota)
	assert.Zero(t, b.UsedQuota)
	var logs []model.Log
	require.NoError(t, r.db.Where("type = ?", model.LogTypeConsume).Find(&logs).Error)
	require.Len(t, logs, wantRequests, "a zero-cost settlement must not masquerade as a refund")
	if !refunded {
		assert.Equal(t, responsesBoundaryA, logs[0].ChannelId)
		assert.Equal(t, responsesBoundaryFee, logs[0].Quota)
		assert.Positive(t, logs[0].CompletionTokens)
		if complete {
			assert.Equal(t, 5, logs[0].PromptTokens)
			assert.Equal(t, 1, logs[0].CompletionTokens)
		}
	}
}
