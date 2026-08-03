package middleware

import (
	"bytes"
	"context"
	"encoding/base64"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"
	"unsafe"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
)

const (
	auditRedactedValue          = "[REDACTED]"
	auditUnsafeTruncatedJSON    = "[TRUNCATED JSON BODY OMITTED: unable to safely redact]"
	contextAuditBodyLimit       = int64(2 * 1024 * 1024)
	contextAuditQueueSize       = 32
	contextAuditWorkerCount     = 2
	contextAuditQueuedByteLimit = uint64(16 * 1024 * 1024)
	contextAuditWarnInterval    = time.Minute
)

var contextAuditStore = newContextAuditStore(contextAuditQueueSize, contextAuditWorkerCount, contextAuditQueuedByteLimit, model.RecordContextRequestLog)

type contextAuditQueueItem struct {
	entry *model.ContextRequestLog
	bytes uint64
}

type contextAuditQueue struct {
	queue       chan contextAuditQueueItem
	record      func(*model.ContextRequestLog) error
	stop        chan struct{}
	done        chan struct{}
	mu          sync.RWMutex
	stopOnce    sync.Once
	workers     sync.WaitGroup
	byteLimit   uint64
	queuedBytes uint64
	dropped     atomic.Uint64
	lastWarning atomic.Int64
}

func newContextAuditStore(capacity, workers int, byteLimit uint64, record func(*model.ContextRequestLog) error) *contextAuditQueue {
	store := &contextAuditQueue{queue: make(chan contextAuditQueueItem, capacity), record: record, stop: make(chan struct{}), done: make(chan struct{}), byteLimit: byteLimit}
	store.workers.Add(workers)
	for range workers {
		go func() {
			defer store.workers.Done()
			for item := range store.queue {
				if err := store.record(item.entry); err != nil {
					common.SysLog("failed to record context request log: " + err.Error())
				}
				store.mu.Lock()
				store.queuedBytes -= item.bytes
				store.mu.Unlock()
			}
		}()
	}
	go func() { store.workers.Wait(); close(store.done) }()
	return store
}
func (store *contextAuditQueue) enqueue(entry *model.ContextRequestLog) bool {
	item := contextAuditQueueItem{entry: entry, bytes: estimateContextAuditLogBytes(entry)}
	store.mu.Lock()
	defer store.mu.Unlock()
	select {
	case <-store.stop:
		return false
	default:
	}
	if item.bytes > store.byteLimit-store.queuedBytes {
		store.warnDropped("byte budget exceeded")
		return false
	}
	select {
	case store.queue <- item:
		store.queuedBytes += item.bytes
		return true
	default:
		store.warnDropped("queue full")
		return false
	}
}

func (store *contextAuditQueue) warnDropped(reason string) {
	dropped := store.dropped.Add(1)
	now := time.Now().UnixNano()
	last := store.lastWarning.Load()
	if now-last >= int64(contextAuditWarnInterval) && store.lastWarning.CompareAndSwap(last, now) {
		common.SysLog("context request audit " + reason + "; dropping records (total dropped: " + strconv.FormatUint(dropped, 10) + ")")
	}
}

func estimateContextAuditLogBytes(entry *model.ContextRequestLog) uint64 {
	if entry == nil {
		return 0
	}
	// Account for the fixed struct (including string descriptors) plus every
	// string backing store retained by an accepted queue item.
	fixedBytes := uint64(unsafe.Sizeof(*entry))
	return fixedBytes + uint64(len(entry.RequestId)+len(entry.Method)+len(entry.Path)+len(entry.Ip)+
		len(entry.UserAgent)+len(entry.Username)+len(entry.TokenName)+len(entry.ModelName)+len(entry.Group)+
		len(entry.Error)+len(entry.ChannelName)+len(entry.NodeName)+len(entry.RuleName)+len(entry.DecisionSource)+
		len(entry.RequestHeaders)+len(entry.ResponseHeaders)+len(entry.RequestBody)+len(entry.RequestBodyEncoding)+
		len(entry.ResponseBody)+len(entry.ResponseBodyEncoding))
}
func (store *contextAuditQueue) shutdown(ctx context.Context) error {
	store.stopOnce.Do(func() { store.mu.Lock(); close(store.stop); close(store.queue); store.mu.Unlock() })
	select {
	case <-store.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
func ShutdownContextRequestAudit(ctx context.Context) error { return contextAuditStore.shutdown(ctx) }

var auditSensitiveJSONFieldPattern = regexp.MustCompile(`(?i)("(?:api[_-]?key|access[_-]?token|refresh[_-]?token|authorization|client[_-]?secret|secret|password)"\s*:\s*)"(?:\\.|[^"\\])*"`)

type contextRequestAuditResponseWriter struct {
	gin.ResponseWriter
	body  bytes.Buffer
	total int64
}

func (w *contextRequestAuditResponseWriter) Write(data []byte) (int, error) {
	w.capture(data)
	return w.ResponseWriter.Write(data)
}
func (w *contextRequestAuditResponseWriter) WriteString(data string) (int, error) {
	w.capture([]byte(data))
	return w.ResponseWriter.WriteString(data)
}
func (w *contextRequestAuditResponseWriter) capture(data []byte) {
	w.total += int64(len(data))
	remaining := contextAuditBodyLimit - int64(w.body.Len())
	if remaining > 0 {
		if int64(len(data)) > remaining {
			data = data[:remaining]
		}
		_, _ = w.body.Write(data)
	}
}

func ContextRequestAudit() gin.HandlerFunc {
	return func(c *gin.Context) {
		// The global setting is a hard gate: the disabled path does not touch rules or bodies.
		if !operation_setting.IsContextRequestLoggingEnabled() {
			c.Next()
			return
		}
		modelName := c.GetString("original_model")
		if modelName == "" && model.ContextLogRulesNeedModel(c.GetInt("id")) {
			modelName = auditOriginalModel(c)
		}
		decision := model.GetContextLogDecision(c.GetInt("id"), modelName)
		if !decision.Capture {
			c.Next()
			return
		}
		start := time.Now()
		writer := &contextRequestAuditResponseWriter{ResponseWriter: c.Writer}
		c.Writer = writer
		c.Next()
		if resolved := c.GetString("original_model"); resolved != "" {
			modelName = resolved
		}
		recordContextRequestAudit(c, start, writer, modelName, decision)
	}
}

func auditOriginalModel(c *gin.Context) string {
	if value := c.GetString("original_model"); value != "" {
		return value
	}
	if c.Request == nil {
		return ""
	}
	// Gemini carries the original model in the path and need not parse a body.
	if marker := strings.Index(c.Request.URL.Path, "/models/"); marker >= 0 {
		value := c.Request.URL.Path[marker+len("/models/"):]
		if colon := strings.IndexByte(value, ':'); colon >= 0 {
			value = value[:colon]
		}
		if value != "" {
			return value
		}
	}
	storage, err := common.GetBodyStorage(c)
	if err != nil {
		return ""
	}
	_, _ = storage.Seek(0, io.SeekStart)
	limited, _ := io.ReadAll(io.LimitReader(storage, contextAuditBodyLimit+1))
	_, _ = storage.Seek(0, io.SeekStart)
	c.Request.Body = io.NopCloser(storage)
	if int64(len(limited)) > contextAuditBodyLimit {
		return ""
	}
	var payload struct {
		Model string `json:"model"`
	}
	if common.Unmarshal(limited, &payload) == nil {
		return payload.Model
	}
	return ""
}

func recordContextRequestAudit(c *gin.Context, start time.Time, writer *contextRequestAuditResponseWriter, modelName string, decision model.ContextLogDecision) {
	requestBody, requestSize, requestTruncated, requestBodyErr := readAuditRequestBody(c)
	requestBody = redactAuditJSON(requestBody, c.Request.Header.Get("Content-Type"), requestTruncated)
	requestBodyText, requestBodyEncoding := encodeAuditBody(requestBody, c.Request.Header.Get("Content-Type"))
	responseBody := redactAuditJSON(writer.body.Bytes(), c.Writer.Header().Get("Content-Type"), writer.total > contextAuditBodyLimit)
	responseBodyText, responseBodyEncoding := encodeAuditBody(responseBody, c.Writer.Header().Get("Content-Type"))
	errorText := strings.TrimSpace(c.Errors.String())
	if requestBodyErr != nil {
		if errorText != "" {
			errorText += "\n"
		}
		errorText += "request body capture failed: " + requestBodyErr.Error()
	}
	entry := &model.ContextRequestLog{
		UserId: c.GetInt("id"), CreatedAt: start.Unix(), RequestId: auditRequestId(c), Method: c.Request.Method,
		Path: sanitizeAuditRequestURI(c), Ip: c.ClientIP(), UserAgent: c.Request.UserAgent(), Username: c.GetString("username"),
		TokenId: c.GetInt("token_id"), TokenName: c.GetString("token_name"), ModelName: modelName,
		Group: common.GetContextKeyString(c, constant.ContextKeyUsingGroup), IsStream: common.GetContextKeyBool(c, constant.ContextKeyIsStream),
		StatusCode: writer.Status(), LatencyMs: time.Since(start).Milliseconds(), Error: errorText,
		ChannelId: common.GetContextKeyInt(c, constant.ContextKeyChannelId), ChannelName: common.GetContextKeyString(c, constant.ContextKeyChannelName),
		ChannelType: common.GetContextKeyInt(c, constant.ContextKeyChannelType), NodeName: common.NodeName,
		RuleId: decision.RuleId, RuleName: decision.RuleName, DecisionSource: decision.Source,
		RequestHeaders: marshalAuditJSON(cloneSanitizedAuditHeaders(c.Request.Header)), ResponseHeaders: marshalAuditJSON(cloneSanitizedAuditHeaders(c.Writer.Header())),
		RequestBody: requestBodyText, RequestBodyEncoding: requestBodyEncoding, RequestBodySize: requestSize, RequestBodyTruncated: requestTruncated,
		ResponseBody: responseBodyText, ResponseBodyEncoding: responseBodyEncoding, ResponseBodySize: writer.total, ResponseBodyTruncated: writer.total > contextAuditBodyLimit,
	}
	if entry.Group == "" {
		entry.Group = common.GetContextKeyString(c, constant.ContextKeyTokenGroup)
	}
	contextAuditStore.enqueue(entry)
}

func auditRequestId(c *gin.Context) string {
	if id := c.GetString(common.RequestIdKey); id != "" {
		return id
	}
	return common.NewRequestId()
}
func readAuditRequestBody(c *gin.Context) ([]byte, int64, bool, error) {
	if c.Request == nil || c.Request.Body == nil || (c.Request.ContentLength == 0 && c.Request.Method == http.MethodGet) {
		return nil, 0, false, nil
	}
	storage, err := common.GetBodyStorage(c)
	if err != nil {
		return nil, 0, false, err
	}
	size := storage.Size()
	_, err = storage.Seek(0, io.SeekStart)
	if err != nil {
		return nil, size, false, err
	}
	body, err := io.ReadAll(io.LimitReader(storage, contextAuditBodyLimit))
	_, seekErr := storage.Seek(0, io.SeekStart)
	c.Request.Body = io.NopCloser(storage)
	if err == nil {
		err = seekErr
	}
	return body, size, size > contextAuditBodyLimit, err
}
func encodeAuditBody(body []byte, contentType string) (string, string) {
	if len(body) == 0 {
		return "", ""
	}
	if shouldStoreAuditBodyAsText(body, contentType) {
		return string(body), "text"
	}
	return base64.StdEncoding.EncodeToString(body), "base64"
}
func shouldStoreAuditBodyAsText(body []byte, contentType string) bool {
	media := strings.ToLower(strings.TrimSpace(strings.Split(contentType, ";")[0]))
	return strings.HasPrefix(media, "text/") || strings.Contains(media, "json") || strings.Contains(media, "xml") || strings.Contains(media, "javascript") || strings.Contains(media, "event-stream") || media == "application/x-www-form-urlencoded" || media == "application/graphql" || utf8.Valid(body)
}
func cloneSanitizedAuditHeaders(headers http.Header) map[string][]string {
	cloned := make(map[string][]string, len(headers))
	for key, values := range headers {
		canonical := http.CanonicalHeaderKey(key)
		if isSensitiveAuditHeader(key) {
			cloned[canonical] = []string{auditRedactedValue}
		} else {
			cloned[canonical] = append([]string(nil), values...)
		}
	}
	return cloned
}
func isSensitiveAuditHeader(key string) bool {
	switch strings.ToLower(key) {
	case "authorization", "proxy-authorization", "x-api-key", "x-goog-api-key", "api-key", "openai-api-key", "mj-api-secret", "cookie", "set-cookie", "sec-websocket-protocol":
		return true
	}
	return false
}
func sanitizeAuditRequestURI(c *gin.Context) string {
	if c.Request == nil || c.Request.URL == nil {
		return ""
	}
	u := *c.Request.URL
	q := u.Query()
	for key := range q {
		if isSensitiveAuditQueryKey(key) {
			q.Set(key, auditRedactedValue)
		}
	}
	u.RawQuery = q.Encode()
	return u.RequestURI()
}
func isSensitiveAuditQueryKey(key string) bool {
	switch strings.ToLower(key) {
	case "key", "api_key", "apikey", "access_token", "token", "authorization", "auth":
		return true
	}
	return false
}
func marshalAuditJSON(value any) string {
	data, err := common.Marshal(value)
	if err != nil {
		return ""
	}
	return string(data)
}
func redactAuditJSON(body []byte, contentType string, truncated bool) []byte {
	if len(body) == 0 || !strings.Contains(strings.ToLower(contentType), "json") {
		return body
	}
	var value any
	if common.Unmarshal(body, &value) != nil {
		if truncated {
			return []byte(auditUnsafeTruncatedJSON)
		}
		return auditSensitiveJSONFieldPattern.ReplaceAll(body, []byte(`${1}"`+auditRedactedValue+`"`))
	}
	redactAuditJSONValue(value)
	data, err := common.Marshal(value)
	if err != nil {
		return []byte(auditRedactedValue)
	}
	return data
}
func redactAuditJSONValue(value any) {
	switch typed := value.(type) {
	case map[string]any:
		for key, item := range typed {
			if isSensitiveAuditJSONKey(key) {
				typed[key] = auditRedactedValue
			} else {
				redactAuditJSONValue(item)
			}
		}
	case []any:
		for _, item := range typed {
			redactAuditJSONValue(item)
		}
	}
}
func isSensitiveAuditJSONKey(key string) bool {
	normalized := strings.ToLower(strings.ReplaceAll(key, "-", "_"))
	switch normalized {
	case "api_key", "apikey", "access_token", "refresh_token", "authorization", "client_secret", "secret", "password":
		return true
	}
	return false
}
