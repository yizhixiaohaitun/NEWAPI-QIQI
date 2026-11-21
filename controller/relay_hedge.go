package controller

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"maps"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
)

// Only the winner may touch the real response writer. Both the timer and
// protocol callbacks arbitrate under the same lock, independently of I/O.
const maxStreamRaceLanes = 3

type streamRace struct {
	mu      sync.Mutex
	ctx     context.Context
	cancel  [maxStreamRaceLanes]context.CancelCauseFunc
	lanes   [maxStreamRaceLanes]context.Context
	winner  int // 0 = unclaimed; lanes are one-based
	started int // starts at 1 for the original request
	done    [maxStreamRaceLanes]bool
}

func (r *streamRace) startBackups(count int) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if count < 1 || r.started+count > maxStreamRaceLanes || r.winner != 0 || r.ctx.Err() != nil {
		return false
	}
	for lane := 0; lane < r.started; lane++ {
		if !r.done[lane] && r.lanes[lane].Err() == nil {
			r.started += count
			return true
		}
	}
	return false
}

func (r *streamRace) claim(lane int) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.winner != 0 {
		return r.winner == lane
	}
	if lane > r.started || r.ctx.Err() != nil || r.lanes[lane-1].Err() != nil {
		return false
	}
	r.winner = lane
	for i, cancel := range r.cancel {
		if i != lane-1 && cancel != nil {
			cancel(relaycommon.ErrStreamRaceLost)
		}
	}
	return true
}

// Pre-output headers, SSE preambles and flushes are private to an attempt.
// After selection, only that attempt can forward writes and deadlines.
type raceStreamWriter struct {
	gin.ResponseWriter
	header   http.Header
	body     bytes.Buffer
	status   int
	selected bool
	overflow bool
	cancel   context.CancelCauseFunc
}

var (
	errRacePreambleTooLarge = errors.New("stream preamble exceeded buffer limit before effective output")
	errRaceNoOutputTimeout  = errors.New("upstream stream timed out before effective output")
)

func (w *raceStreamWriter) Header() http.Header { return w.header }
func (w *raceStreamWriter) WriteHeader(code int) {
	if w.selected {
		w.ResponseWriter.WriteHeader(code)
	} else if code > 0 {
		w.status = code
	}
}
func (w *raceStreamWriter) WriteHeaderNow() {
	if w.selected {
		w.ResponseWriter.WriteHeaderNow()
	}
}
func (w *raceStreamWriter) Status() int { return w.status }
func (w *raceStreamWriter) Size() int {
	if w.selected {
		return w.ResponseWriter.Size()
	}
	return -1
}
func (w *raceStreamWriter) Written() bool { return w.selected }
func (w *raceStreamWriter) Flush() {
	if w.selected {
		w.ResponseWriter.Flush()
	}
}
func (w *raceStreamWriter) Write(p []byte) (int, error) {
	if w.selected {
		return w.ResponseWriter.Write(p)
	}
	if bytes.Equal(p, []byte(": PING\n\n")) {
		return len(p), nil
	}
	if w.overflow || len(p) > (4<<20)-w.body.Len() {
		w.overflow = true
		w.cancel(errRacePreambleTooLarge)
		return 0, errRacePreambleTooLarge
	}
	return w.body.Write(p)
}
func (w *raceStreamWriter) WriteString(s string) (int, error) { return w.Write([]byte(s)) }
func (w *raceStreamWriter) SetWriteDeadline(t time.Time) error {
	if !w.selected {
		return nil
	}
	return http.NewResponseController(w.ResponseWriter).SetWriteDeadline(t)
}
func (w *raceStreamWriter) publish(c *gin.Context) error {
	w.selected = true
	helper.ExtendWriteDeadline(c)
	h := w.ResponseWriter.Header()
	clear(h)
	for k, v := range w.header {
		h[k] = v
	}
	w.header = h
	w.ResponseWriter.WriteHeader(w.status)
	if w.body.Len() != 0 {
		_, err := w.ResponseWriter.Write(w.body.Bytes())
		w.body = bytes.Buffer{}
		return err
	}
	return nil
}

// A race is limited to the first attempt and native HTTP SSE adapters. Never
// race a pinned resource or bypass an affinity rule that prevents retries.
func canRaceStream(c *gin.Context, info *relaycommon.RelayInfo, channel *model.Channel, allProtocols bool) bool {
	if info.PurityDetectionRequest || !info.IsStream || c.Writer.Written() || service.ShouldSkipRetryAfterChannelAffinityFailure(c) || service.ShouldProtectResponsesStateAffinity(c) {
		return false
	}
	if _, pinned := common.GetContextKey(c, constant.ContextKeyTokenSpecificChannelId); pinned {
		return false
	}
	return nativeRaceStream(c, info, channel, allProtocols)
}

func nativeRaceStream(c *gin.Context, info *relaycommon.RelayInfo, channel *model.Channel, allProtocols bool) bool {
	if service.ShouldChatCompletionsUseResponsesGlobal(channel.Id, channel.Type, info.OriginModelName) {
		return false
	}
	if info.RelayFormat == types.RelayFormatClaude {
		return channel.Type == constant.ChannelTypeAnthropic
	}
	if !allProtocols {
		return false
	}
	// Use the effective overrides after distributor/affinity merged its template.
	setting, _ := common.GetContextKeyType[dto.ChannelSettings](c, constant.ContextKeyChannelSetting)
	other, _ := common.GetContextKeyType[dto.ChannelOtherSettings](c, constant.ContextKeyChannelOtherSetting)
	overrides := common.GetContextKeyStringMap(c, constant.ContextKeyChannelParamOverride)
	switch info.RelayFormat {
	case types.RelayFormatOpenAI:
		return info.RelayMode == relayconstant.RelayModeChatCompletions && channel.Type == constant.ChannelTypeOpenAI && relay.NativeChatStreamForHedge(c, info, setting, overrides)
	case types.RelayFormatOpenAIResponses:
		return info.RelayMode == relayconstant.RelayModeResponses && channel.Type == constant.ChannelTypeOpenAI && !other.DisableStore && relay.StreamHedgeHeaderOnlyOverrides(overrides)
	case types.RelayFormatGemini:
		return info.RelayMode == relayconstant.RelayModeGemini && channel.Type == constant.ChannelTypeGemini && strings.HasSuffix(c.Request.URL.Path, ":streamGenerateContent") && relay.StreamHedgeHeaderOnlyOverrides(overrides)
	}
	return false
}

// Check the original wire body, including fields not represented by the DTO.
// Responses defaults to persistent storage, so only explicit store:false is safe.
func statelessStreamRace(format types.RelayFormat, body []byte) bool {
	if format == types.RelayFormatClaude {
		return true
	}
	var req map[string]json.RawMessage
	if json.Unmarshal(body, &req) != nil || req == nil {
		return false
	}
	fields := make(map[string]json.RawMessage, len(req))
	for key, value := range req {
		key = strings.ToLower(key)
		if _, duplicate := fields[key]; duplicate {
			return false
		}
		fields[key] = value
		switch key {
		case "conversation", "previous_response_id", "cachedcontent", "cached_content", "context_management", "prompt", "extra_body":
			if string(value) != "null" {
				return false
			}
		case "store", "background":
			if string(value) != "false" && string(value) != "null" {
				return false
			}
		}
	}
	if format == types.RelayFormatGemini {
		var tools []map[string]json.RawMessage
		if raw := fields["tools"]; len(raw) != 0 && json.Unmarshal(raw, &tools) != nil {
			return false
		}
		for _, tool := range tools {
			for name := range tool {
				if name != "functionDeclarations" && name != "function_declarations" {
					return false // Never duplicate a provider-executed tool.
				}
			}
		}
		return true
	}
	if format == types.RelayFormatOpenAIResponses && string(fields["store"]) != "false" {
		return false
	}
	var tools []struct {
		Type string `json:"type"`
	}
	if raw := fields["tools"]; len(raw) != 0 && json.Unmarshal(raw, &tools) != nil {
		return false
	}
	for _, tool := range tools {
		if tool.Type != "function" {
			return false
		}
	}
	var input any
	if raw := fields["input"]; len(raw) != 0 && json.Unmarshal(raw, &input) != nil {
		return false
	}
	if items, ok := input.([]any); ok {
		for _, item := range items {
			if obj, ok := item.(map[string]any); ok {
				kind, _ := obj["type"].(string)
				if kind == "item_reference" || strings.HasPrefix(kind, "mcp_") || obj["id"] != nil || obj["encrypted_content"] != nil {
					return false
				}
			}
		}
	}
	return true
}

type streamRaceAttempt struct {
	c                 *gin.Context
	info              relaycommon.RelayInfo
	channel           *model.Channel
	allProtocols      bool
	billing           relaycommon.BillingSettler
	firstByteTimedOut bool
	writer            *raceStreamWriter
	settle            func(*relaycommon.RelayInfo)
	err               *types.NewAPIError
	contextErr        error
}

func newStreamRaceAttempt(c *gin.Context, info *relaycommon.RelayInfo, writer gin.ResponseWriter, header http.Header, status int, body []byte, allProtocols bool) (*streamRaceAttempt, error) {
	local := c.Copy()
	local.Request = c.Request.Clone(c.Request.Context())
	local.Request.Body = io.NopCloser(bytes.NewReader(body))
	local.Set(common.KeyBodyStorage, nil)
	local.Set(common.KeyRequestBody, nil)
	w := &raceStreamWriter{ResponseWriter: writer, header: header.Clone(), status: status}
	local.Writer = w
	a := &streamRaceAttempt{c: local, info: *info, writer: w, allProtocols: allProtocols, billing: info.Billing}
	a.info.RequestConversionChain = append([]types.RelayFormat(nil), info.RequestConversionChain...)
	// Legacy Claude already isolates its mutable request via a per-helper copy.
	if !allProtocols {
		if req, ok := info.Request.(*dto.ClaudeRequest); ok {
			copy := *req
			a.info.Request = &copy
		}
		return a, nil
	}
	a.info.Billing = nil // Only restore on the controller, after all lanes have joined.
	a.info.RequestHeaders = maps.Clone(info.RequestHeaders)
	a.info.RuntimeHeadersOverride = maps.Clone(info.RuntimeHeadersOverride)
	a.info.ParamOverrideAudit = append([]string(nil), info.ParamOverrideAudit...)
	if info.ResponsesUsageInfo != nil {
		state := *info.ResponsesUsageInfo
		state.BuiltInTools = maps.Clone(state.BuiltInTools)
		for k, v := range state.BuiltInTools {
			if v != nil {
				tool := *v
				state.BuiltInTools[k] = &tool
			}
		}
		a.info.ResponsesUsageInfo = &state
	}
	var err error
	switch req := info.Request.(type) {
	case *dto.ClaudeRequest:
		a.info.Request, err = common.DeepCopy(req)
	case *dto.GeneralOpenAIRequest:
		a.info.Request, err = common.DeepCopy(req)
	case *dto.OpenAIResponsesRequest:
		a.info.Request, err = common.DeepCopy(req)
	case *dto.GeminiChatRequest:
		a.info.Request, err = common.DeepCopy(req)
	default:
		err = errors.New("unsupported stream race request")
	}
	return a, err
}

func (a *streamRaceAttempt) run(r *streamRace, lane int, ctx context.Context) {
	defer common.CleanupBodyStorage(a.c)
	// An upstream that sends only metadata can keep resetting the idle timer.
	// Bound just the uncommitted phase; the winner keeps normal stream timeouts.
	if constant.StreamingTimeout > 0 {
		timer := time.AfterFunc(time.Duration(constant.StreamingTimeout)*time.Second, func() {
			r.mu.Lock()
			defer r.mu.Unlock()
			if r.winner != lane {
				r.cancel[lane-1](errRaceNoOutputTimeout)
			}
		})
		defer timer.Stop()
	}
	a.info.UpstreamContext = ctx
	// Do not send transport keepalives before upstream headers. Scanner pings
	// retain their normal behavior after selection, but are dropped before it.
	a.info.DeferStreamHeadersUntilResponse = true
	a.c.Set("event_stream_headers_set", false)
	a.c.Set(common.UpstreamRequestIdKey, "")
	var publishErr error
	a.writer.cancel = r.cancel[lane-1]
	a.info.ClaimStreamOutput = func() bool {
		if a.writer.selected {
			return true
		}
		if a.writer.overflow || ctx.Err() != nil || !r.claim(lane) {
			return false
		}
		publishErr = a.writer.publish(a.c)
		if publishErr != nil {
			r.cancel[lane-1](publishErr)
		}
		return publishErr == nil
	}
	a.info.DeferStreamSettlement = func(settle func(*relaycommon.RelayInfo)) { a.settle = settle }
	a.c.Set(string(constant.ContextKeyUpstreamCancel), context.CancelFunc(func() { r.cancel[lane-1](nil) }))
	switch a.info.RelayFormat {
	case types.RelayFormatOpenAI:
		a.err = relay.TextHelper(a.c, &a.info)
	case types.RelayFormatOpenAIResponses:
		a.err = relay.ResponsesHelper(a.c, &a.info)
	case types.RelayFormatGemini:
		a.err = relay.GeminiHelper(a.c, &a.info)
	default:
		a.err = relay.ClaudeHelper(a.c, &a.info)
	}
	if a.allProtocols {
		a.firstByteTimedOut = a.info.FirstByteWatch != nil && a.info.FirstByteWatch.Finish()
	}
	a.contextErr = ctx.Err()
	if publishErr != nil {
		a.err = types.NewError(publishErr, types.ErrorCodeBadResponseBody, types.ErrOptionWithSkipRetry())
	} else if a.firstByteTimedOut && !a.writer.selected && r.ctx.Err() == nil {
		a.c.Set("upstream_first_byte_timeout", true)
		a.info.ResetFirstResponseTime()
		a.err = types.NewErrorWithStatusCode(errors.New("upstream timed out before the first response byte"), types.ErrorCodeFirstByteTimeout, http.StatusGatewayTimeout)
	} else if errors.Is(context.Cause(ctx), errRaceNoOutputTimeout) ||
		(!a.writer.selected && a.info.StreamStatus != nil && a.info.StreamStatus.EndReason == relaycommon.StreamEndReasonTimeout) {
		// The scanner's idle timer may fire just before the lane's output
		// timer. Before commitment either one is a local per-lane timeout.
		a.err = types.NewErrorWithStatusCode(errRaceNoOutputTimeout, types.ErrorCodeStreamOutputTimeout, http.StatusGatewayTimeout)
	} else if a.writer.overflow {
		a.err = types.NewErrorWithStatusCode(errRacePreambleTooLarge, types.ErrorCodeBadResponseBody, http.StatusBadGateway)
	}
	// These callbacks are attempt-local, not state for settlement or retries.
	a.info.ClaimStreamOutput = nil
	a.info.DeferStreamSettlement = nil
}

func (a *streamRaceAttempt) prepareBackupChannel(channel *model.Channel) bool {
	if channel == nil || (!a.allProtocols && !nativeRaceStream(a.c, &a.info, channel, false)) {
		return false
	}
	a.c.Set(string(constant.ContextKeyChannelOrganization), "")
	a.c.Set(string(constant.ContextKeyChannelMultiKeyIndex), 0)
	if err := middleware.SetupContextForSelectedChannel(a.c, channel, a.info.OriginModelName); err != nil {
		return false
	}
	if a.allProtocols && !nativeRaceStream(a.c, &a.info, channel, true) {
		return false
	}
	a.channel = channel
	return true
}

// Select only within the request's effective group. Parallel hedging does not
// opt a token into cross-group routing or introduce another pricing group.
func (a *streamRaceAttempt) prepareParallelAlternate(ctx context.Context, group string, primaryID int) {
	excluded := map[int]bool{primaryID: true}
	for len(excluded) <= 16 && ctx.Err() == nil {
		channel, err := model.GetRandomSatisfiedChannelExcluding(group, a.info.OriginModelName, 0, a.c.Request.URL.Path, excluded)
		if err != nil || channel == nil {
			return
		}
		if channel.Status == common.ChannelStatusEnabled && a.prepareBackupChannel(channel) {
			return
		}
		excluded[channel.Id] = true
	}
}

// A nil attempt means the ordinary relay path should run. The returned
// backups are actual attempts, each counted against the shared retry budget.
func raceRelayStream(c *gin.Context, info *relaycommon.RelayInfo, primary *model.Channel, budget context.Context, storage common.BodyStorage, wait time.Duration, sameChannelFirst, parallelBackups, allProtocols bool, retries int) (*streamRaceAttempt, []*model.Channel) {
	if !canRaceStream(c, info, primary, allProtocols) || storage.IsDisk() || storage.Size() > 1<<20 {
		return nil, nil
	}
	body, err := storage.Bytes()
	if err != nil || (allProtocols && !statelessStreamRace(info.RelayFormat, body)) {
		return nil, nil
	}
	group := info.UsingGroup
	if autoGroup := common.GetContextKeyString(c, constant.ContextKeyAutoGroup); autoGroup != "" {
		group = autoGroup
	}
	if group == "" || group == "auto" {
		return nil, nil
	}

	count := 2
	if sameChannelFirst && retries > 1 {
		count = 3
	}
	ctx, cancel := context.WithCancel(budget)
	r := &streamRace{ctx: ctx, started: 1}
	var attempts [maxStreamRaceLanes]*streamRaceAttempt
	header, status := c.Writer.Header(), c.Writer.Status()
	for i := range count {
		attempts[i], err = newStreamRaceAttempt(c, info, c.Writer, header, status, body, allProtocols)
		if err != nil {
			cancel()
			return nil, nil
		}
		r.lanes[i], r.cancel[i] = context.WithCancelCause(ctx)
	}
	attempts[0].channel = primary
	completed := make(chan struct{}, count)
	var workers sync.WaitGroup
	var runnerPanic any
	var panicOnce sync.Once
	launch := func(i int) {
		workers.Add(1)
		go func() {
			defer workers.Done()
			defer func() {
				if p := recover(); p != nil {
					panicOnce.Do(func() { runnerPanic = p })
					cancel()
				}
				r.mu.Lock()
				r.done[i] = true
				r.mu.Unlock()
				completed <- struct{}{}
			}()
			attempts[i].run(r, i+1, r.lanes[i])
		}()
	}
	defer func() { cancel(); workers.Wait() }()
	timer := time.NewTimer(wait)
	defer timer.Stop()
	launch(0)
	running := 1
	timerC := timer.C
	for running > 0 {
		select {
		case <-ctx.Done():
			running = 0
		case <-completed:
			running--
			r.mu.Lock()
			won := r.winner != 0
			r.mu.Unlock()
			if won {
				timer.Stop()
				timerC = nil
			}
		case <-timerC:
			timerC = nil
			i := r.started
			if r.lanes[i].Err() != nil {
				continue
			}
			last := i + 1
			if parallelBackups && count == 3 {
				last = count
			}
			for j := i; j < last; j++ {
				candidate := attempts[j]
				if sameChannelFirst && j == 1 {
					// Reuse the original selection, including its existing key.
					candidate.channel = primary
				} else if parallelBackups {
					candidate.prepareParallelAlternate(r.lanes[j], group, primary.Id)
				} else {
					channel, err := model.GetRandomSatisfiedChannelExcluding(group, info.OriginModelName, 0, c.Request.URL.Path, map[int]bool{primary.Id: true})
					if err == nil {
						candidate.prepareBackupChannel(channel)
					}
				}
				if candidate.channel == nil {
					last = j
					break
				}
				candidate.info.RetryIndex += j
				service.MarkChannelAffinityUsed(candidate.c, group, candidate.channel.Id)
			}
			// Reserve both slots at the same threshold, before either backup can win.
			if !r.startBackups(last - i) {
				continue
			}
			for j := i; j < last; j++ {
				logger.LogInfo(attempts[j].c, "stream backup started")
				launch(j)
				running++
			}
			if !parallelBackups && last < count {
				timer.Reset(wait)
				timerC = timer.C
			}
		}
	}
	workers.Wait()
	if runnerPanic != nil {
		panic(runnerPanic)
	}

	result := attempts[0]
	if r.winner != 0 {
		result = attempts[r.winner-1]
	}
	if r.winner == 0 && result.err == nil {
		result.err = types.NewErrorWithStatusCode(errors.New("upstream stream ended before effective output"), types.ErrorCodeBadResponseBody, http.StatusBadGateway, types.ErrOptionWithSkipRetry())
	}
	for k, v := range result.c.Keys {
		if k != common.KeyBodyStorage && k != common.KeyRequestBody && k != "event_stream_headers_set" {
			c.Set(k, v)
		}
	}
	var backups []*model.Channel
	for _, attempt := range attempts[1:r.started] {
		backups = append(backups, attempt.channel)
		addUsedChannel(c, attempt.channel.Id)
	}
	if len(backups) != 0 {
		addUsed := c.GetStringSlice("use_channel")
		result.c.Set("use_channel", addUsed)
		if r.winner == 0 {
			logger.LogInfo(c, "stream race finished without effective output")
		} else {
			logger.LogInfo(c, "stream race finished: winner="+result.channel.Name)
		}
	}
	*info = result.info
	if allProtocols {
		info.Billing = result.billing
	}
	if r.winner != 0 && c.Writer.Written() {
		c.Set(string(constant.ContextKeyStreamHedgeWinner), result.channel.Id)
	}
	if r.winner != 0 && result.err == nil && result.settle != nil {
		result.settle(info)
	}
	return result, backups
}
