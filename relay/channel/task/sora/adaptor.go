package sora

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
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
	return nil
}

func isMiniMaxH3Model(modelName string) bool {
	return strings.EqualFold(strings.TrimSpace(modelName), "MiniMax-H3")
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

	return common.Marshal(map[string]interface{}{
		"model": upstreamModel,
		"input": input,
	})
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
			if isMiniMaxH3Model(info.UpstreamModelName) {
				req, reqErr := relaycommon.GetTaskRequest(c)
				if reqErr != nil {
					return nil, reqErr
				}
				newBody, buildErr := buildMiniMaxH3Body(bodyMap, req, info.UpstreamModelName)
				if buildErr != nil {
					return nil, buildErr
				}
				return bytes.NewReader(newBody), nil
			}
			bodyMap["model"] = info.UpstreamModelName
			if newBody, err := common.Marshal(bodyMap); err == nil {
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
