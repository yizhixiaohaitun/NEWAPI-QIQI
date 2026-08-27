package sora

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel"
	taskcommon "github.com/QuantumNous/new-api/relay/channel/task/taskcommon"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
	"github.com/pkg/errors"
	"github.com/tidwall/sjson"
)

// ============================
// Request / Response structures
// ============================

type ContentItem struct {
	Type     string    `json:"type"`                // "text" or "image_url"
	Text     string    `json:"text,omitempty"`      // for text type
	ImageURL *ImageURL `json:"image_url,omitempty"` // for image_url type
}

type ImageURL struct {
	URL string `json:"url"`
}

type responseError struct {
	Message string `json:"message"`
	Code    string `json:"code"`
}

type responseTask struct {
	ID                 string         `json:"id"`
	TaskID             string         `json:"task_id,omitempty"` //兼容旧接口
	Object             string         `json:"object"`
	Model              string         `json:"model"`
	Status             string         `json:"status"`
	Progress           int            `json:"progress"`
	CreatedAt          int64          `json:"created_at"`
	CompletedAt        int64          `json:"completed_at,omitempty"`
	ExpiresAt          int64          `json:"expires_at,omitempty"`
	Seconds            string         `json:"seconds,omitempty"`
	Size               string         `json:"size,omitempty"`
	RemixedFromVideoID string         `json:"remixed_from_video_id,omitempty"`
	Error              *responseError `json:"error,omitempty"`
}

// taskResponseWire accepts both the flat OpenAI/Sora task protocol and
// providers that wrap the task in {success, data:{...}}. Data is deliberately
// raw because the wrapped task itself may contain data.outputs.
type taskResponseWire struct {
	ID                 string          `json:"id"`
	TaskID             string          `json:"task_id"`
	Object             string          `json:"object"`
	Model              string          `json:"model"`
	Status             string          `json:"status"`
	Progress           json.RawMessage `json:"progress"`
	CreatedAt          int64           `json:"created_at"`
	CompletedAt        int64           `json:"completed_at"`
	ExpiresAt          int64           `json:"expires_at"`
	Seconds            string          `json:"seconds"`
	Size               string          `json:"size"`
	RemixedFromVideoID string          `json:"remixed_from_video_id"`
	FailReason         string          `json:"fail_reason"`
	Error              *responseError  `json:"error"`
	Outputs            []any           `json:"outputs"`
	Data               json.RawMessage `json:"data"`
}

func parseTaskResponse(body []byte) (taskResponseWire, bool, error) {
	var root taskResponseWire
	if err := common.Unmarshal(body, &root); err != nil {
		return root, false, err
	}
	if len(root.Data) == 0 || string(root.Data) == "null" {
		return root, false, nil
	}

	var nested taskResponseWire
	if err := common.Unmarshal(root.Data, &nested); err != nil {
		return root, false, nil
	}
	if nested.ID == "" && nested.TaskID == "" && nested.Status == "" && len(nested.Progress) == 0 && nested.FailReason == "" {
		return root, false, nil
	}
	return nested, true, nil
}

func responseProgress(raw json.RawMessage) int {
	if len(raw) == 0 || string(raw) == "null" {
		return 0
	}
	var number float64
	if err := common.Unmarshal(raw, &number); err == nil {
		return int(number)
	}
	var text string
	if err := common.Unmarshal(raw, &text); err == nil {
		value, _ := strconv.ParseFloat(strings.TrimSuffix(strings.TrimSpace(text), "%"), 64)
		return int(value)
	}
	return 0
}

func responseOutputURL(task taskResponseWire) string {
	outputs := task.Outputs
	if len(outputs) == 0 && len(task.Data) > 0 {
		var payload struct {
			Outputs []any `json:"outputs"`
		}
		if common.Unmarshal(task.Data, &payload) == nil {
			outputs = payload.Outputs
		}
	}
	for _, output := range outputs {
		switch value := output.(type) {
		case string:
			if strings.TrimSpace(value) != "" {
				return value
			}
		case map[string]any:
			for _, key := range []string{"url", "video_url", "file_url", "output_url"} {
				if url, ok := value[key].(string); ok && strings.TrimSpace(url) != "" {
					return url
				}
			}
		}
	}
	return ""
}

// ============================
// Adaptor implementation
// ============================

type TaskAdaptor struct {
	taskcommon.BaseBilling
	ChannelType int
	apiKey      string
	baseURL     string
}

func (a *TaskAdaptor) Init(info *relaycommon.RelayInfo) {
	a.ChannelType = info.ChannelType
	a.baseURL = info.ChannelBaseUrl
	a.apiKey = info.ApiKey
}

func validateRemixRequest(c *gin.Context) *dto.TaskError {
	var req relaycommon.TaskSubmitReq
	if err := common.UnmarshalBodyReusable(c, &req); err != nil {
		return service.TaskErrorWrapperLocal(err, "invalid_request", http.StatusBadRequest)
	}
	if strings.TrimSpace(req.Prompt) == "" {
		return service.TaskErrorWrapperLocal(fmt.Errorf("field prompt is required"), "invalid_request", http.StatusBadRequest)
	}
	// 存储原始请求到 context，与 ValidateMultipartDirect 路径保持一致
	c.Set("task_request", req)
	return nil
}

func (a *TaskAdaptor) ValidateRequestAndSetAction(c *gin.Context, info *relaycommon.RelayInfo) (taskErr *dto.TaskError) {
	if info.Action == constant.TaskActionRemix {
		return validateRemixRequest(c)
	}
	return relaycommon.ValidateMultipartDirect(c, info)
}

// EstimateBilling 根据用户请求的 seconds 和 size 计算 OtherRatios。
func (a *TaskAdaptor) EstimateBilling(c *gin.Context, info *relaycommon.RelayInfo) map[string]float64 {
	// remix 路径的 OtherRatios 已在 ResolveOriginTask 中设置
	if info.Action == constant.TaskActionRemix {
		return nil
	}

	req, err := relaycommon.GetTaskRequest(c)
	if err != nil {
		return nil
	}

	seconds, _ := strconv.Atoi(req.Seconds)
	if seconds == 0 {
		seconds = req.Duration
	}
	if seconds <= 0 {
		seconds = 4
	}

	size := req.Size
	if size == "" {
		size = "720x1280"
	}

	ratios := map[string]float64{
		"seconds": float64(seconds),
		"size":    1,
	}
	if size == "1792x1024" || size == "1024x1792" {
		ratios["size"] = 1.666667
	}
	return ratios
}

func (a *TaskAdaptor) BuildRequestURL(info *relaycommon.RelayInfo) (string, error) {
	if info.Action == constant.TaskActionRemix {
		return fmt.Sprintf("%s/v1/videos/%s/remix", a.baseURL, info.OriginTaskID), nil
	}
	return fmt.Sprintf("%s/v1/videos", a.baseURL), nil
}

// BuildRequestHeader sets required headers.
func (a *TaskAdaptor) BuildRequestHeader(c *gin.Context, req *http.Request, info *relaycommon.RelayInfo) error {
	req.Header.Set("Authorization", "Bearer "+a.apiKey)
	req.Header.Set("Content-Type", c.Request.Header.Get("Content-Type"))
	// MiniMax-H3 documents Idempotency-Key for safely retrying task creation.
	// Keep this scoped to that protocol so existing generic/Sora providers do
	// not unexpectedly receive a client-controlled header.
	if isMiniMaxH3Model(info.UpstreamModelName) {
		if idempotencyKey := strings.TrimSpace(c.GetHeader("Idempotency-Key")); idempotencyKey != "" {
			req.Header.Set("Idempotency-Key", idempotencyKey)
		}
	}
	return nil
}

func isMiniMaxH3Model(modelName string) bool {
	return strings.EqualFold(strings.TrimSpace(modelName), "MiniMax-H3")
}

// isMiniMaxH3ResolutionModel identifies the newer provider-specific family.
// It intentionally excludes the legacy MiniMax-H3 spelling because that model
// uses a different {model,input} protocol.
func isMiniMaxH3ResolutionModel(modelName string) bool {
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(modelName)), "minimaxh3-")
}

func buildMiniMaxH3Body(bodyMap map[string]interface{}, req relaycommon.TaskSubmitReq, upstreamModel string) ([]byte, error) {
	input := make(map[string]interface{})
	if original, ok := bodyMap["input"].(map[string]interface{}); ok {
		for key, value := range original {
			input[key] = value
		}
	}

	// Nested documented fields take precedence. Top-level accepted fields are
	// moved rather than copied, so the provider receives only its documented
	// model + input shape instead of NewAPI's internal compatibility mixture.
	for _, key := range []string{"prompt", "aspect_ratio", "resolution", "duration", "audio", "n"} {
		if _, exists := input[key]; exists {
			continue
		}
		if value, exists := bodyMap[key]; exists {
			input[key] = value
		}
	}
	if _, exists := input["prompt"]; !exists && req.Prompt != "" {
		input["prompt"] = req.Prompt
	}
	if _, exists := input["duration"]; !exists {
		if req.Duration != 0 {
			input["duration"] = req.Duration
		} else if req.Seconds != "" {
			input["duration"] = req.Seconds
		}
	}

	payload := map[string]interface{}{
		"model": upstreamModel,
		"input": input,
	}
	// callback_url is part of the documented root request rather than input.
	if callbackURL, exists := bodyMap["callback_url"]; exists {
		payload["callback_url"] = callbackURL
	}
	return common.Marshal(payload)
}

func buildMiniMaxH3ResolutionBody(bodyMap map[string]interface{}, req relaycommon.TaskSubmitReq, upstreamModel string) ([]byte, error) {
	// This provider family accepts the OpenAI videos endpoint but not the
	// legacy MiniMax-H3 input envelope. Preserve top-level vendor parameters,
	// normalize shared fields from accepted compatibility shapes, and remove
	// input so model/prompt/resolution/duration stay at the document root.
	payload := make(map[string]interface{}, len(bodyMap)+1)
	for key, value := range bodyMap {
		if key != "input" {
			payload[key] = value
		}
	}
	payload["model"] = upstreamModel

	for _, key := range []string{"prompt", "resolution", "duration"} {
		if _, exists := payload[key]; exists {
			continue
		}
		if value, exists := req.Input[key]; exists {
			payload[key] = value
		}
	}
	if _, exists := payload["prompt"]; !exists && req.Prompt != "" {
		payload["prompt"] = req.Prompt
	}
	if _, exists := payload["duration"]; !exists {
		switch {
		case req.Duration != 0:
			payload["duration"] = req.Duration
		case req.Seconds != "":
			payload["duration"] = req.Seconds
		}
	}
	if _, exists := payload["resolution"]; !exists && req.Metadata != nil {
		if resolution, ok := req.Metadata["resolution"]; ok {
			payload["resolution"] = resolution
		}
	}

	return common.Marshal(payload)
}

func appendReferenceURLs(urls []string, seen map[string]struct{}, value interface{}) []string {
	switch typed := value.(type) {
	case string:
		url := strings.TrimSpace(typed)
		if url != "" {
			if _, exists := seen[url]; !exists {
				seen[url] = struct{}{}
				urls = append(urls, url)
			}
		}
	case []string:
		for _, item := range typed {
			urls = appendReferenceURLs(urls, seen, item)
		}
	case []interface{}:
		for _, item := range typed {
			urls = appendReferenceURLs(urls, seen, item)
		}
	case map[string]interface{}:
		for _, key := range []string{"url", "image_url", "video_url", "audio_url", "image", "source"} {
			if nested, exists := typed[key]; exists {
				urls = appendReferenceURLs(urls, seen, nested)
			}
		}
	}
	return urls
}

func seedanceBodyValue(bodyMap map[string]interface{}, keys ...string) (interface{}, bool) {
	for _, key := range keys {
		if value, exists := bodyMap[key]; exists && value != nil {
			return value, true
		}
	}
	if input, ok := bodyMap["input"].(map[string]interface{}); ok {
		for _, key := range keys {
			if value, exists := input[key]; exists && value != nil {
				return value, true
			}
		}
	}
	return nil, false
}

func collectSeedanceMediaURLs(bodyMap map[string]interface{}, rootKeys, nestedKeys []string) []string {
	urls := make([]string, 0)
	seen := make(map[string]struct{})
	for _, key := range rootKeys {
		urls = appendReferenceURLs(urls, seen, bodyMap[key])
	}
	for _, containerKey := range []string{"metadata", "input"} {
		container, ok := bodyMap[containerKey].(map[string]interface{})
		if !ok {
			continue
		}
		for _, key := range nestedKeys {
			urls = appendReferenceURLs(urls, seen, container[key])
		}
	}
	return urls
}

func usesXinshujuContentProtocol(info *relaycommon.RelayInfo) bool {
	if info.ChannelSetting.VideoUpstreamProtocol == dto.VideoUpstreamProtocolXinshujuContent {
		return true
	}
	// Migration compatibility for channels configured before the dedicated
	// protocol existed. Scope the fallback to Xinshuju's real hostname instead
	// of the model name so other Seedance providers keep their own wire format.
	if info.ChannelSetting.VideoUpstreamProtocol != dto.VideoUpstreamProtocolOpenAI {
		return false
	}
	upstreamURL, err := url.Parse(info.ChannelBaseUrl)
	if err != nil {
		return false
	}
	hostname := strings.ToLower(upstreamURL.Hostname())
	return hostname == "xinshuju.net" || strings.HasSuffix(hostname, ".xinshuju.net")
}

// buildXinshujuContentBody translates the public OpenAI-compatible request
// into Xinshuju's multimodal content protocol. The conversion is enabled only
// by the explicit per-channel xinshuju_content protocol; model names must not
// select a provider-specific wire format.
func buildXinshujuContentBody(bodyMap map[string]interface{}, upstreamModel string) map[string]interface{} {
	payload := map[string]interface{}{"model": upstreamModel}
	content := make([]map[string]interface{}, 0, 5)

	if prompt, exists := seedanceBodyValue(bodyMap, "prompt"); exists {
		content = append(content, map[string]interface{}{"type": "text", "text": prompt})
	}
	appendMedia := func(urls []string, mediaType, role string) {
		for _, mediaURL := range urls {
			content = append(content, map[string]interface{}{
				"type":             mediaType + "_url",
				mediaType + "_url": map[string]interface{}{"url": mediaURL},
				"role":             role,
			})
		}
	}
	appendMedia(
		collectSeedanceMediaURLs(bodyMap,
			[]string{"images", "image_urls", "reference_images", "image", "input_reference"},
			[]string{"images", "image_urls", "reference_images", "image_references", "image", "input_reference"}),
		"image", "reference_image",
	)
	appendMedia(
		collectSeedanceMediaURLs(bodyMap,
			[]string{"videos", "video_urls", "reference_videos"},
			[]string{"videos", "video_urls", "reference_videos", "video_references"}),
		"video", "reference_video",
	)
	appendMedia(
		collectSeedanceMediaURLs(bodyMap,
			[]string{"audios", "audio_urls", "reference_audios"},
			[]string{"audios", "audio_urls", "reference_audios", "audio_references"}),
		"audio", "reference_audio",
	)
	payload["content"] = content

	if value, exists := seedanceBodyValue(bodyMap, "generate_audio", "audio"); exists {
		payload["generate_audio"] = value
	}
	if value, exists := seedanceBodyValue(bodyMap, "ratio", "aspect_ratio"); exists {
		payload["ratio"] = value
	}
	for _, key := range []string{"duration", "resolution", "return_last_frame", "seed", "tools"} {
		if value, exists := seedanceBodyValue(bodyMap, key); exists {
			payload[key] = value
		}
	}
	if value, exists := seedanceBodyValue(bodyMap, "watermark"); exists {
		payload["watermark"] = value
	} else {
		payload["watermark"] = false
	}
	return payload
}

func (a *TaskAdaptor) BuildRequestBody(c *gin.Context, info *relaycommon.RelayInfo) (io.Reader, error) {
	storage, err := common.GetBodyStorage(c)
	if err != nil {
		return nil, errors.Wrap(err, "get_request_body_failed")
	}
	cachedBody, err := storage.Bytes()
	if err != nil {
		return nil, errors.Wrap(err, "read_body_bytes_failed")
	}
	contentType := c.GetHeader("Content-Type")

	if strings.HasPrefix(contentType, "application/json") {
		var bodyMap map[string]interface{}
		if err := common.Unmarshal(cachedBody, &bodyMap); err == nil {
			if isMiniMaxH3Model(info.UpstreamModelName) || isMiniMaxH3ResolutionModel(info.UpstreamModelName) {
				req, reqErr := relaycommon.GetTaskRequest(c)
				if reqErr != nil {
					return nil, reqErr
				}
				var newBody []byte
				var buildErr error
				if isMiniMaxH3Model(info.UpstreamModelName) {
					newBody, buildErr = buildMiniMaxH3Body(bodyMap, req, info.UpstreamModelName)
				} else {
					newBody, buildErr = buildMiniMaxH3ResolutionBody(bodyMap, req, info.UpstreamModelName)
				}
				if buildErr != nil {
					return nil, buildErr
				}
				return bytes.NewReader(newBody), nil
			}
			bodyMap["model"] = info.UpstreamModelName
			if usesXinshujuContentProtocol(info) {
				bodyMap = buildXinshujuContentBody(bodyMap, info.UpstreamModelName)
			}
			if newBody, err := common.Marshal(bodyMap); err == nil {
				info.TaskRequestSnapshot = append(json.RawMessage(nil), newBody...)
				return bytes.NewReader(newBody), nil
			}
		}
		return bytes.NewReader(cachedBody), nil
	}

	if strings.Contains(contentType, "multipart/form-data") {
		formData, err := common.ParseMultipartFormReusable(c)
		if err != nil {
			return bytes.NewReader(cachedBody), nil
		}
		var buf bytes.Buffer
		writer := multipart.NewWriter(&buf)
		writer.WriteField("model", info.UpstreamModelName)
		for key, values := range formData.Value {
			if key == "model" {
				continue
			}
			for _, v := range values {
				writer.WriteField(key, v)
			}
		}
		for fieldName, fileHeaders := range formData.File {
			for _, fh := range fileHeaders {
				f, err := fh.Open()
				if err != nil {
					continue
				}
				ct := fh.Header.Get("Content-Type")
				if ct == "" || ct == "application/octet-stream" {
					buf512 := make([]byte, 512)
					n, _ := io.ReadFull(f, buf512)
					ct = http.DetectContentType(buf512[:n])
					// Re-open after sniffing so the full content is copied below
					f.Close()
					f, err = fh.Open()
					if err != nil {
						continue
					}
				}
				h := make(textproto.MIMEHeader)
				h.Set("Content-Disposition", fmt.Sprintf(`form-data; name="%s"; filename="%s"`, fieldName, fh.Filename))
				h.Set("Content-Type", ct)
				part, err := writer.CreatePart(h)
				if err != nil {
					f.Close()
					continue
				}
				io.Copy(part, f)
				f.Close()
			}
		}
		writer.Close()
		c.Request.Header.Set("Content-Type", writer.FormDataContentType())
		return &buf, nil
	}

	return common.ReaderOnly(storage), nil
}

// DoRequest delegates to common helper.
func (a *TaskAdaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (*http.Response, error) {
	return channel.DoTaskApiRequest(a, c, info, requestBody)
}

// DoResponse handles upstream response, returns taskID etc.
func (a *TaskAdaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (taskID string, taskData []byte, taskErr *dto.TaskError) {
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		taskErr = service.TaskErrorWrapper(err, "read_response_body_failed", http.StatusInternalServerError)
		return
	}
	_ = resp.Body.Close()

	parsed, nested, err := parseTaskResponse(responseBody)
	if err != nil {
		taskErr = service.TaskErrorWrapper(errors.Wrapf(err, "body: %s", responseBody), "unmarshal_response_body_failed", http.StatusInternalServerError)
		return
	}

	upstreamID := parsed.ID
	if upstreamID == "" {
		upstreamID = parsed.TaskID
	}
	if upstreamID == "" {
		taskErr = service.TaskErrorWrapper(fmt.Errorf("task_id is empty"), "invalid_response", http.StatusInternalServerError)
		return
	}

	// Preserve the provider's documented envelope while replacing its private
	// task ID. Existing flat OpenAI/Sora responses keep their historical shape.
	if nested {
		publicBody, setErr := sjson.SetBytes(responseBody, "data.task_id", info.PublicTaskID)
		if setErr != nil {
			taskErr = service.TaskErrorWrapper(setErr, "rewrite_response_failed", http.StatusInternalServerError)
			return
		}
		if parsed.ID != "" {
			publicBody, _ = sjson.SetBytes(publicBody, "data.id", info.PublicTaskID)
		}
		c.Data(http.StatusOK, "application/json", publicBody)
	} else {
		dResp := responseTask{
			ID: info.PublicTaskID, TaskID: info.PublicTaskID, Object: parsed.Object,
			Model: parsed.Model, Status: parsed.Status, Progress: responseProgress(parsed.Progress),
			CreatedAt: parsed.CreatedAt, CompletedAt: parsed.CompletedAt, ExpiresAt: parsed.ExpiresAt,
			Seconds: parsed.Seconds, Size: parsed.Size, RemixedFromVideoID: parsed.RemixedFromVideoID,
			Error: parsed.Error,
		}
		c.JSON(http.StatusOK, dResp)
	}
	return upstreamID, responseBody, nil
}

// FetchTask fetch task status
func (a *TaskAdaptor) FetchTask(baseUrl, key string, body map[string]any, proxy string) (*http.Response, error) {
	taskID, ok := body["task_id"].(string)
	if !ok {
		return nil, fmt.Errorf("invalid task_id")
	}

	uri := fmt.Sprintf("%s/v1/videos/%s", baseUrl, taskID)

	req, err := http.NewRequest(http.MethodGet, uri, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+key)

	client, err := service.GetHttpClientWithProxy(proxy)
	if err != nil {
		return nil, fmt.Errorf("new proxy http client failed: %w", err)
	}
	return client.Do(req)
}

func (a *TaskAdaptor) GetModelList() []string {
	return ModelList
}

func (a *TaskAdaptor) GetChannelName() string {
	return ChannelName
}

func (a *TaskAdaptor) ParseTaskResult(respBody []byte) (*relaycommon.TaskInfo, error) {
	resTask, _, err := parseTaskResponse(respBody)
	if err != nil {
		return nil, errors.Wrap(err, "unmarshal task result failed")
	}

	taskResult := relaycommon.TaskInfo{Code: 0}
	status := strings.ToLower(strings.TrimSpace(resTask.Status))
	switch status {
	case "queued", "pending", "submitted", "not_start":
		taskResult.Status = model.TaskStatusQueued
	case "processing", "in_progress", "running":
		taskResult.Status = model.TaskStatusInProgress
	case "completed", "complete", "succeeded", "success":
		taskResult.Status = model.TaskStatusSuccess
		taskResult.Url = responseOutputURL(resTask)
	case "failed", "failure", "cancelled", "canceled":
		taskResult.Status = model.TaskStatusFailure
		switch {
		case strings.TrimSpace(resTask.FailReason) != "":
			taskResult.Reason = resTask.FailReason
		case resTask.Error != nil && strings.TrimSpace(resTask.Error.Message) != "":
			taskResult.Reason = resTask.Error.Message
		default:
			taskResult.Reason = "task failed"
		}
	}
	if progress := responseProgress(resTask.Progress); progress > 0 && progress < 100 {
		taskResult.Progress = fmt.Sprintf("%d%%", progress)
	}

	return &taskResult, nil
}

func (a *TaskAdaptor) ConvertToOpenAIVideo(task *model.Task) ([]byte, error) {
	data := task.Data
	parsed, nested, _ := parseTaskResponse(data)
	if nested {
		var err error
		if data, err = sjson.SetBytes(data, "data.task_id", task.TaskID); err != nil {
			return nil, errors.Wrap(err, "set nested task_id failed")
		}
		if parsed.ID != "" {
			data, _ = sjson.SetBytes(data, "data.id", task.TaskID)
		}
		return data, nil
	}

	var err error
	if data, err = sjson.SetBytes(data, "id", task.TaskID); err != nil {
		return nil, errors.Wrap(err, "set id failed")
	}
	if parsed.TaskID != "" {
		data, _ = sjson.SetBytes(data, "task_id", task.TaskID)
	}
	return data, nil
}
