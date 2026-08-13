package seedance

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel"
	"github.com/QuantumNous/new-api/relay/channel/task/taskcommon"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/pkg/errors"
	"github.com/tidwall/sjson"
)

var windowsPathPattern = regexp.MustCompile(`^[A-Za-z]:[\\/]`)

var supportedModels = map[string]struct{}{
	"seedance-2.0":      {},
	"seedance-2.0-fast": {},
}

type TaskAdaptor struct {
	taskcommon.BaseBilling
	apiKey  string
	baseURL string
}

type responseError struct {
	Message string `json:"message"`
}

type taskWire struct {
	TaskID     string          `json:"task_id"`
	Status     string          `json:"status"`
	Progress   json.RawMessage `json:"progress"`
	FailReason string          `json:"fail_reason"`
	Data       json.RawMessage `json:"data"`
	Outputs    []any           `json:"outputs"`
	Error      *responseError  `json:"error"`
}

type responseWire struct {
	Success bool            `json:"success"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

func (a *TaskAdaptor) Init(info *relaycommon.RelayInfo) {
	a.apiKey = info.ApiKey
	a.baseURL = strings.TrimRight(info.ChannelBaseUrl, "/")
}

func normalizedModel(model string) string { return strings.ToLower(strings.TrimSpace(model)) }

func parsePositiveInt(v any) (int, bool) {
	switch n := v.(type) {
	case float64:
		if n == float64(int(n)) {
			return int(n), true
		}
	case json.Number:
		i, err := strconv.Atoi(n.String())
		return i, err == nil
	case string:
		i, err := strconv.Atoi(strings.TrimSpace(n))
		return i, err == nil
	case int:
		return n, true
	}
	return 0, false
}

func inputString(input map[string]any, key string) string {
	value, _ := input[key].(string)
	return strings.TrimSpace(value)
}

func inputArray(input map[string]any, key string) []any {
	items, _ := input[key].([]any)
	return items
}

func invalidRequest(err error) *dto.TaskError {
	return service.TaskErrorWrapperLocal(err, "invalid_request", http.StatusBadRequest)
}

func validateReferenceValue(value any) error {
	switch item := value.(type) {
	case string:
		value := strings.TrimSpace(item)
		if value == "" {
			return fmt.Errorf("media reference must not be empty")
		}
		lower := strings.ToLower(value)
		if strings.HasPrefix(lower, "file://") || strings.HasPrefix(value, "/") || strings.HasPrefix(value, `\\`) || windowsPathPattern.MatchString(value) {
			return fmt.Errorf("local file paths are not supported")
		}
		if strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") || strings.HasPrefix(lower, "data:") {
			if strings.HasPrefix(lower, "http") {
				if parsed, err := url.ParseRequestURI(value); err != nil || parsed.Host == "" {
					return fmt.Errorf("invalid media reference URL")
				}
			}
			return nil
		}
		if _, err := base64.StdEncoding.DecodeString(value); err == nil {
			return nil
		}
		if _, err := base64.RawStdEncoding.DecodeString(value); err == nil {
			return nil
		}
		return fmt.Errorf("media reference must be a URL, data URI, or base64 value")
	case map[string]any:
		// Providers may add descriptive fields to reference objects. Require at
		// least one usable payload field while preserving every extension field.
		hasPayload := false
		for _, key := range []string{"url", "image_url", "video_url", "audio_url", "data", "base64"} {
			if nested, ok := item[key]; ok {
				if err := validateReferenceValue(nested); err != nil {
					return fmt.Errorf("%s: %w", key, err)
				}
				hasPayload = true
			}
		}
		if !hasPayload {
			return fmt.Errorf("media reference object must contain url, image_url, video_url, audio_url, data, or base64")
		}
		return nil
	default:
		return fmt.Errorf("invalid media reference")
	}
}

func validateReferences(input map[string]any, key string, max int) error {
	items := inputArray(input, key)
	if raw, exists := input[key]; exists && raw != nil && items == nil {
		return fmt.Errorf("input.%s must be an array", key)
	}
	if len(items) > max {
		return fmt.Errorf("input.%s supports at most %d item(s)", key, max)
	}
	for _, item := range items {
		if err := validateReferenceValue(item); err != nil {
			return fmt.Errorf("input.%s: %w", key, err)
		}
	}
	return nil
}

func validateSeedanceRequest(req relaycommon.TaskSubmitReq) error {
	modelName := normalizedModel(req.Model)
	if _, ok := supportedModels[modelName]; !ok {
		return fmt.Errorf("model must be seedance-2.0 or seedance-2.0-fast")
	}
	input := req.Input
	if strings.TrimSpace(req.Prompt) == "" {
		return fmt.Errorf("input.prompt is required")
	}
	duration, durationOK := parsePositiveInt(input["duration"])
	if !durationOK || duration != req.Duration {
		return fmt.Errorf("input.duration must be an integer between 4 and 15")
	}
	if duration < 4 || duration > 15 {
		return fmt.Errorf("input.duration must be an integer between 4 and 15")
	}
	resolution := strings.ToLower(inputString(input, "resolution"))
	if resolution == "" {
		return fmt.Errorf("input.resolution is required")
	}
	if modelName == "seedance-2.0-fast" {
		if resolution != "480p" && resolution != "720p" {
			return fmt.Errorf("seedance-2.0-fast resolution must be 480p or 720p")
		}
	} else {
		if resolution != "480p" && resolution != "720p" && resolution != "1080p" {
			return fmt.Errorf("seedance-2.0 resolution must be 480p, 720p, or 1080p")
		}
		if resolution == "1080p" && duration > 12 {
			return fmt.Errorf("seedance-2.0 1080p duration must be between 4 and 12")
		}
	}
	if ratio := inputString(input, "aspect_ratio"); ratio != "16:9" && ratio != "9:16" && ratio != "1:1" {
		return fmt.Errorf("input.aspect_ratio must be 16:9, 9:16, or 1:1")
	}
	if rawN, exists := input["n"]; exists {
		if n, ok := parsePositiveInt(rawN); !ok || n != 1 {
			return fmt.Errorf("input.n must be 1")
		}
	}
	if audio, exists := input["audio"]; exists {
		if _, ok := audio.(bool); !ok {
			return fmt.Errorf("input.audio must be a boolean")
		}
	}
	for key, max := range map[string]int{
		"image_references": 4,
		"video_references": 3,
		"audio_references": 1,
		"start_frames":     1,
		"end_frames":       1,
	} {
		if err := validateReferences(input, key, max); err != nil {
			return err
		}
	}
	if len(inputArray(input, "end_frames")) > 0 && len(inputArray(input, "start_frames")) == 0 {
		return fmt.Errorf("input.end_frames requires input.start_frames")
	}
	return nil
}

func (a *TaskAdaptor) ValidateRequestAndSetAction(c *gin.Context, info *relaycommon.RelayInfo) *dto.TaskError {
	var req relaycommon.TaskSubmitReq
	if err := common.UnmarshalBodyReusable(c, &req); err != nil {
		return invalidRequest(err)
	}
	if err := validateSeedanceRequest(req); err != nil {
		return invalidRequest(err)
	}
	if _, exists := req.Input["n"]; !exists {
		req.Input["n"] = 1
	}
	if _, exists := req.Input["audio"]; !exists {
		req.Input["audio"] = true
	}
	info.Action = constant.TaskActionGenerate
	c.Set("task_request", req)
	return nil
}

func (a *TaskAdaptor) EstimateBilling(c *gin.Context, _ *relaycommon.RelayInfo) map[string]float64 {
	req, err := relaycommon.GetTaskRequest(c)
	if err != nil {
		return nil
	}
	resolutionRatio := 1.0
	switch strings.ToLower(inputString(req.Input, "resolution")) {
	case "480p":
		resolutionRatio = 0.5
	case "1080p":
		resolutionRatio = 2.5
	}
	return map[string]float64{"seconds": float64(req.Duration), "resolution": resolutionRatio}
}

func (a *TaskAdaptor) BuildRequestURL(_ *relaycommon.RelayInfo) (string, error) {
	return a.baseURL + "/async/tasks", nil
}

func (a *TaskAdaptor) BuildRequestHeader(_ *gin.Context, req *http.Request, _ *relaycommon.RelayInfo) error {
	req.Header.Set("Authorization", "Bearer "+a.apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	return nil
}

func (a *TaskAdaptor) BuildRequestBody(c *gin.Context, info *relaycommon.RelayInfo) (io.Reader, error) {
	req, err := relaycommon.GetTaskRequest(c)
	if err != nil {
		return nil, err
	}
	input := make(map[string]any, len(req.Input))
	for key, value := range req.Input {
		input[key] = value
	}
	input["prompt"] = req.Prompt
	input["duration"] = req.Duration
	input["resolution"] = strings.ToLower(inputString(input, "resolution"))
	input["aspect_ratio"] = inputString(input, "aspect_ratio")
	input["n"] = 1
	if _, exists := input["audio"]; !exists {
		input["audio"] = true
	}
	payload, err := common.Marshal(map[string]any{"model": info.UpstreamModelName, "input": input})
	if err != nil {
		return nil, err
	}
	return bytes.NewReader(payload), nil
}

func (a *TaskAdaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (*http.Response, error) {
	return channel.DoTaskApiRequest(a, c, info, requestBody)
}

func parseNestedTask(body []byte) (taskWire, error) {
	var envelope responseWire
	if err := common.Unmarshal(body, &envelope); err != nil {
		return taskWire{}, err
	}
	var task taskWire
	if len(envelope.Data) == 0 || string(envelope.Data) == "null" {
		return task, fmt.Errorf("response data is empty")
	}
	if err := common.Unmarshal(envelope.Data, &task); err != nil {
		return task, err
	}
	return task, nil
}

func (a *TaskAdaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (string, []byte, *dto.TaskError) {
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", nil, service.TaskErrorWrapper(err, "read_response_body_failed", http.StatusInternalServerError)
	}
	_ = resp.Body.Close()
	task, err := parseNestedTask(body)
	if err != nil {
		return "", nil, service.TaskErrorWrapper(errors.Wrapf(err, "body: %s", body), "invalid_response", http.StatusInternalServerError)
	}
	if strings.TrimSpace(task.TaskID) == "" {
		return "", nil, service.TaskErrorWrapper(fmt.Errorf("task_id is empty"), "invalid_response", http.StatusInternalServerError)
	}
	publicBody, err := rewritePublicTaskIDs(body, info.PublicTaskID)
	if err != nil {
		return "", nil, service.TaskErrorWrapper(err, "rewrite_response_failed", http.StatusInternalServerError)
	}
	c.Data(http.StatusOK, "application/json", publicBody)
	return task.TaskID, body, nil
}

func (a *TaskAdaptor) FetchTask(baseURL, key string, body map[string]any, proxy string) (*http.Response, error) {
	taskID, ok := body["task_id"].(string)
	if !ok || strings.TrimSpace(taskID) == "" {
		return nil, fmt.Errorf("invalid task_id")
	}
	req, err := http.NewRequest(http.MethodGet, strings.TrimRight(baseURL, "/")+"/async/tasks/"+url.PathEscape(taskID), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Accept", "application/json")
	client, err := service.GetHttpClientWithProxy(proxy)
	if err != nil {
		return nil, err
	}
	return client.Do(req)
}

func responseProgress(raw json.RawMessage) string {
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	var number float64
	if common.Unmarshal(raw, &number) == nil {
		return fmt.Sprintf("%d%%", int(number))
	}
	var text string
	if common.Unmarshal(raw, &text) == nil {
		text = strings.TrimSpace(text)
		if text != "" && !strings.HasSuffix(text, "%") {
			text += "%"
		}
		return text
	}
	return ""
}

func outputURL(task taskWire) string {
	outputs := task.Outputs
	if len(outputs) == 0 && len(task.Data) > 0 {
		var nested struct {
			Outputs []any `json:"outputs"`
		}
		if common.Unmarshal(task.Data, &nested) == nil {
			outputs = nested.Outputs
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
				if value, ok := value[key].(string); ok && strings.TrimSpace(value) != "" {
					return value
				}
			}
		}
	}
	return ""
}

func (a *TaskAdaptor) ParseTaskResult(body []byte) (*relaycommon.TaskInfo, error) {
	task, err := parseNestedTask(body)
	if err != nil {
		return nil, errors.Wrap(err, "unmarshal task result failed")
	}
	result := &relaycommon.TaskInfo{TaskID: task.TaskID, Progress: responseProgress(task.Progress)}
	switch strings.ToLower(strings.TrimSpace(task.Status)) {
	case "pending", "queued", "submitted", "not_start":
		result.Status = model.TaskStatusQueued
	case "running", "processing", "in_progress":
		result.Status = model.TaskStatusInProgress
	case "success", "succeeded", "completed", "complete":
		result.Status = model.TaskStatusSuccess
		result.Url = outputURL(task)
	case "failure", "failed", "cancelled", "canceled":
		result.Status = model.TaskStatusFailure
		result.Reason = task.FailReason
		if result.Reason == "" && task.Error != nil {
			result.Reason = task.Error.Message
		}
		if result.Reason == "" {
			result.Reason = "task failed"
		}
	}
	return result, nil
}

func rewritePublicTaskIDs(body []byte, publicTaskID string) ([]byte, error) {
	result, err := sjson.SetBytes(body, "data.task_id", publicTaskID)
	if err != nil {
		return nil, err
	}
	var envelope map[string]any
	if err := common.Unmarshal(body, &envelope); err != nil {
		return nil, err
	}
	data, _ := envelope["data"].(map[string]any)
	if _, exists := data["id"]; exists {
		result, err = sjson.SetBytes(result, "data.id", publicTaskID)
		if err != nil {
			return nil, err
		}
	}
	if nested, ok := data["data"].(map[string]any); ok {
		if _, exists := nested["id"]; exists {
			result, err = sjson.SetBytes(result, "data.data.id", publicTaskID)
			if err != nil {
				return nil, err
			}
		}
	}
	return result, nil
}

func (a *TaskAdaptor) ConvertTaskResponse(task *model.Task) ([]byte, error) {
	if len(task.Data) == 0 {
		return nil, fmt.Errorf("task response is empty")
	}
	return rewritePublicTaskIDs(task.Data, task.TaskID)
}

func (a *TaskAdaptor) GetModelList() []string { return []string{"seedance-2.0", "seedance-2.0-fast"} }
func (a *TaskAdaptor) GetChannelName() string { return "Seedance Async" }
