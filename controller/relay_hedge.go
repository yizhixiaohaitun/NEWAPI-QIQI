package controller

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
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

func (r *streamRace) startBackup() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.started >= maxStreamRaceLanes || r.winner != 0 || r.ctx.Err() != nil {
		return false
	}
	for lane := 0; lane < r.started; lane++ {
		if !r.done[lane] && r.lanes[lane].Err() == nil {
			r.started++
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

// A race is limited to the first attempt and native HTTP SSE adapters. Use the
// effective billing group, never a different auto group or a pinned resource.
func canRaceStream(c *gin.Context, info *relaycommon.RelayInfo, channel *model.Channel) bool {
	if info.PurityDetectionRequest || !info.IsStream || c.Writer.Written() || service.ShouldSkipRetryAfterChannelAffinityFailure(c) || service.ShouldProtectResponsesStateAffinity(c) {
		return false
	}
	if _, pinned := common.GetContextKey(c, constant.ContextKeyTokenSpecificChannelId); pinned {
		return false
	}
	return nativeRaceStream(info, channel.Id, channel.Type)
}

func nativeRaceStream(info *relaycommon.RelayInfo, id, channelType int) bool {
	return info.RelayFormat == types.RelayFormatClaude &&
		channelType == constant.ChannelTypeAnthropic && !service.ShouldChatCompletionsUseResponsesGlobal(id, channelType, info.OriginModelName)
}

type streamRaceAttempt struct {
	c          *gin.Context
	info       relaycommon.RelayInfo
	channel    *model.Channel
	writer     *raceStreamWriter
	settle     func(*relaycommon.RelayInfo)
	err        *types.NewAPIError
	contextErr error
}

func newStreamRaceAttempt(c *gin.Context, info *relaycommon.RelayInfo, writer gin.ResponseWriter, header http.Header, status int, body []byte) *streamRaceAttempt {
	local := c.Copy()
	local.Request = c.Request.Clone(c.Request.Context())
	local.Request.Body = io.NopCloser(bytes.NewReader(body))
	local.Set(common.KeyBodyStorage, nil)
	local.Set(common.KeyRequestBody, nil)
	w := &raceStreamWriter{ResponseWriter: writer, header: header.Clone(), status: status}
	local.Writer = w
	a := &streamRaceAttempt{c: local, info: *info, writer: w}
	a.info.RequestConversionChain = append([]types.RelayFormat(nil), info.RequestConversionChain...)
	// InitChannelMeta resets the request's model; give each lane its own DTO.
	if req, ok := info.Request.(*dto.ClaudeRequest); ok {
		copy := *req
		a.info.Request = &copy
	}
	return a
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
	a.err = relay.ClaudeHelper(a.c, &a.info)
	a.contextErr = ctx.Err()
	if publishErr != nil {
		a.err = types.NewError(publishErr, types.ErrorCodeBadResponseBody, types.ErrOptionWithSkipRetry())
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

// A nil attempt means the ordinary relay path should run. The returned
// backups are actual attempts, each counted against the shared retry budget.
func raceRelayStream(c *gin.Context, info *relaycommon.RelayInfo, primary *model.Channel, budget context.Context, storage common.BodyStorage, wait time.Duration, sameChannelFirst bool, retries int) (*streamRaceAttempt, []*model.Channel) {
	if !canRaceStream(c, info, primary) || storage.IsDisk() || storage.Size() > 1<<20 {
		return nil, nil
	}
	body, err := storage.Bytes()
	if err != nil {
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
		attempts[i] = newStreamRaceAttempt(c, info, c.Writer, header, status, body)
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
			candidate := attempts[i]
			channel := primary
			if !sameChannelFirst || i != 1 {
				channel, err = model.GetRandomSatisfiedChannelExcluding(group, info.OriginModelName, 0, c.Request.URL.Path, map[int]bool{primary.Id: true})
				if err != nil || channel == nil || !nativeRaceStream(&candidate.info, channel.Id, channel.Type) {
					continue
				}
				candidate.c.Set(string(constant.ContextKeyChannelOrganization), "")
				candidate.c.Set(string(constant.ContextKeyChannelMultiKeyIndex), 0)
				if err := middleware.SetupContextForSelectedChannel(candidate.c, channel, candidate.info.OriginModelName); err != nil {
					continue
				}
			}
			// The same-channel attempt reuses the selected channel configuration
			// captured before the primary began, including its existing key.
			if !r.startBackup() {
				continue
			}
			candidate.channel = channel
			candidate.info.RetryIndex += i
			service.MarkChannelAffinityUsed(candidate.c, group, channel.Id)
			logger.LogInfo(candidate.c, "stream backup started")
			launch(i)
			running++
			if i+1 < count {
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
	if r.winner == 0 && errors.Is(result.err, relaycommon.ErrClaudeStreamEndedWithoutOutput) {
		// An empty primary is not evidence that an already-started backup
		// also ended safely. Do not replay unless every started lane agrees.
		for _, lane := range attempts[1:r.started] {
			if !errors.Is(lane.err, relaycommon.ErrClaudeStreamEndedWithoutOutput) ||
				types.IsSkipRetryError(lane.err) || lane.c.GetBool("upstream_moderation_committed") {
				types.ErrOptionWithSkipRetry()(result.err)
				break
			}
		}
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
	if r.winner != 0 && c.Writer.Written() {
		c.Set(string(constant.ContextKeyStreamHedgeWinner), result.channel.Id)
	}
	if r.winner != 0 && result.err == nil && result.settle != nil {
		result.settle(info)
	}
	return result, backups
}
