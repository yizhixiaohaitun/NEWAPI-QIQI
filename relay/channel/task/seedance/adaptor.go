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

const (
	seedanceAsyncPath          = "/async/tasks"
	seedanceDiscountCreatePath = "/kyyReactApiServer/v1/seedance-discount/videos"
	seedanceDiscountResultPath = "/kyyReactApiServer/v1/result"
)

var windowsPathPattern = regexp.MustCompile(`^[A-Za-z]:[\\/]`)

var supportedModels = map[string]struct{}{
	"seedance-2.0":      {},
	"seedance-2.0-fast": {},
}

type TaskAdaptor struct {
	taskcommon.BaseBilling
	// Protocol is explicit and persisted in task.Platform. It is never inferred
	// from the model name or an upstream error response.
	Protocol string
	apiKey   string
	baseURL  string
}

type responseError struct {
	Message string `json:"message"`
	Code    string `json:"code,omitempty"`
}

func (e *responseError) UnmarshalJSON(data []byte) error {
	var text string
	if err := common.Unmarshal(data, &text); err == nil {
		e.Message = text
		return nil
	}
	type alias responseError
	return common.Unmarshal(data, (*alias)(e))
}

type taskWire struct {
	ID           string          `json:"id,omitempty"`
	TaskID       string          `json:"task_id,omitempty"`
	Object       string          `json:"object,omitempty"`
	Model        string          `json:"model,omitempty"`
	Status       string          `json:"status"`
	Progress     json.RawMessage `json:"progress,omitempty"`
	FailReason   string          `json:"fail_reason,omitempty"`
	Data         json.RawMessage `json:"data,omitempty"`
	Outputs      []any           `json:"outputs,omitempty"`
	VideoURL     string          `json:"video_url,omitempty"`
	LastFrameURL string          `json:"last_frame_url,omitempty"`
	Created      int64           `json:"created,omitempty"`
	CreatedAt    int64           `json:"created_at,omitempty"`
	Error        *responseError  `json:"error,omitempty"`
	Amount       float64         `json:"amount,omitempty"`
	TotalTokens  int             `json:"totalTokens,omitempty"`
}

type discountMediaURL struct {
	URL any `json:"url"`
}

type discountContentItem struct {
	Type     string            `json:"type"`
	Text     string            `json:"text,omitempty"`
	Role     string            `json:"role,omitempty"`
	ImageURL *discountMediaURL `json:"image_url,omitempty"`
	VideoURL *discountMediaURL `json:"video_url,omitempty"`
	AudioURL *discountMediaURL `json:"audio_url,omitempty"`
}

type discountRequest struct {
	Model           string                `json:"model"`
	Ratio           string                `json:"ratio"`
	Duration        int                   `json:"duration"`
	GenerateAudio   *bool                 `json:"generate_audio,omitempty"`
	ReturnLastFrame *bool                 `json:"return_last_frame,omitempty"`
	Seed            *int                  `json:"seed,omitempty"`
	Tools           []any                 `json:"tools,omitempty"`
	Content         []discountContentItem `json:"content"`
}

type responseWire struct {
	Success bool            `json:"success"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

func normalizeBaseURL(raw string) string {
	base := strings.TrimRight(strings.TrimSpace(raw), "/")
	if strings.HasSuffix(strings.ToLower(base), "/v1") {
		base = strings.TrimRight(base[:len(base)-3], "/")
	}
	return base
}

func (a *TaskAdaptor) Init(info *relaycommon.RelayInfo) {
	a.apiKey = info.ApiKey
	a.baseURL = normalizeBaseURL(info.ChannelBaseUrl)
	if a.Protocol == "" {
		a.Protocol = string(dto.VideoUpstreamProtocolSeedanceAsync)
	}
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
		if key == "video_references" || key == "audio_references" {
			if object, ok := item.(map[string]any); ok {
				if _, exists := object["strength"]; exists {
					return fmt.Errorf("input.%s does not support strength", key)
				}
			}
		}
	}
	return nil
}

func publicSize(size, resolution, aspectRatio string) (string, string, error) {
	size = strings.ToLower(strings.TrimSpace(size))
	resolution = strings.ToLower(strings.TrimSpace(resolution))
	aspectRatio = strings.TrimSpace(aspectRatio)
	if size == "" {
		if resolution == "" {
			resolution = "720p"
		}
		if aspectRatio == "" {
			aspectRatio = "16:9"
		}
		return resolution, aspectRatio, nil
	}
	var sizeResolution, sizeRatio string
	switch size {
	case "854x480", "480p", "480p-landscape":
		sizeResolution, sizeRatio = "480p", "16:9"
	case "480x854", "480p-portrait":
		sizeResolution, sizeRatio = "480p", "9:16"
	case "1280x720", "720p", "720p-landscape":
		sizeResolution, sizeRatio = "720p", "16:9"
	case "720x1280", "720p-portrait":
		sizeResolution, sizeRatio = "720p", "9:16"
	case "1920x1080", "1080p", "1080p-landscape":
		sizeResolution, sizeRatio = "1080p", "16:9"
	case "1080x1920", "1080p-portrait":
		sizeResolution, sizeRatio = "1080p", "9:16"
	case "480x480":
		sizeResolution, sizeRatio = "480p", "1:1"
	case "720x720":
		sizeResolution, sizeRatio = "720p", "1:1"
	case "1080x1080":
		sizeResolution, sizeRatio = "1080p", "1:1"
	default:
		return "", "", fmt.Errorf("size must be a supported 480p, 720p, or 1080p landscape, portrait, or square size")
	}
	if resolution != "" && resolution != sizeResolution {
		return "", "", fmt.Errorf("size conflicts with resolution")
	}
	if aspectRatio != "" && aspectRatio != sizeRatio {
		return "", "", fmt.Errorf("size conflicts with aspect_ratio")
	}
	return sizeResolution, sizeRatio, nil
}

func normalizePublicRequest(req *relaycommon.TaskSubmitReq) error {
	if req.Input == nil {
		req.Input = map[string]any{}
	}
	input := req.Input
	if nestedPrompt, ok := input["prompt"].(string); ok && strings.TrimSpace(req.Prompt) != "" && strings.TrimSpace(nestedPrompt) != strings.TrimSpace(req.Prompt) {
		return fmt.Errorf("prompt conflicts with input.prompt")
	}
	if req.Prompt == "" {
		req.Prompt = inputString(input, "prompt")
	}

	duration := req.Duration
	if req.Seconds != "" {
		seconds, err := strconv.Atoi(strings.TrimSpace(req.Seconds))
		if err != nil {
			return fmt.Errorf("seconds must be an integer")
		}
		if duration != 0 && duration != seconds {
			return fmt.Errorf("seconds conflicts with duration")
		}
		duration = seconds
	}
	for _, key := range []string{"duration", "seconds"} {
		if nested, exists := input[key]; exists {
			n, ok := parsePositiveInt(nested)
			if !ok {
				return fmt.Errorf("input.%s must be an integer", key)
			}
			if duration != 0 && duration != n {
				return fmt.Errorf("duration/seconds conflicts with input.%s", key)
			}
			duration = n
		}
	}
	if duration == 0 {
		duration = 4
	}
	req.Duration = duration

	resolution := req.Resolution
	if nested := inputString(input, "resolution"); nested != "" {
		if resolution != "" && !strings.EqualFold(resolution, nested) {
			return fmt.Errorf("resolution conflicts with input.resolution")
		}
		resolution = nested
	}
	aspectRatio := strings.TrimSpace(req.AspectRatio)
	if nested := inputString(input, "aspect_ratio"); nested != "" {
		if aspectRatio != "" && aspectRatio != nested {
			return fmt.Errorf("aspect_ratio conflicts with input.aspect_ratio")
		}
		aspectRatio = nested
	}
	mappedResolution, mappedRatio, err := publicSize(req.Size, resolution, aspectRatio)
	if err != nil {
		return err
	}
	input["resolution"], input["aspect_ratio"] = mappedResolution, mappedRatio
	input["duration"] = duration
	var audioValue *bool
	if req.GenerateAudio != nil {
		value := *req.GenerateAudio
		audioValue = &value
	}
	for _, key := range []string{"generate_audio", "audio"} {
		if nested, exists := input[key]; exists {
			value, ok := nested.(bool)
			if !ok {
				return fmt.Errorf("input.%s must be a boolean", key)
			}
			if audioValue != nil && *audioValue != value {
				return fmt.Errorf("generate_audio conflicts with input.%s", key)
			}
			audioValue = &value
		}
	}
	if audioValue == nil {
		value := true
		audioValue = &value
	}
	input["audio"] = *audioValue
	delete(input, "generate_audio")

	if req.InputReference != nil {
		items, ok := req.InputReference.([]any)
		if !ok {
			items = []any{req.InputReference}
		}
		for _, raw := range items {
			if value, ok := raw.(string); ok {
				input["start_frames"] = append(inputArray(input, "start_frames"), strings.TrimSpace(value))
				continue
			}
			item, ok := raw.(map[string]any)
			if !ok {
				return fmt.Errorf("input_reference items must be objects or URL strings")
			}
			typ, _ := item["type"].(string)
			switch strings.ToLower(strings.TrimSpace(typ)) {
			case "image":
				value := item["image_url"]
				if value == nil {
					return fmt.Errorf("image input_reference requires image_url")
				}
				if strength, exists := item["strength"]; exists {
					value = map[string]any{"url": value, "strength": strength}
				}
				input["start_frames"] = append(inputArray(input, "start_frames"), value)
			case "video":
				value := item["video_url"]
				if value == nil {
					return fmt.Errorf("video input_reference requires video_url")
				}
				if _, exists := item["strength"]; exists {
					return fmt.Errorf("video input_reference does not support strength")
				}
				input["video_references"] = append(inputArray(input, "video_references"), value)
			default:
				return fmt.Errorf("input_reference type must be image or video")
			}
		}
	}
	input["n"] = 1
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

func appendMultipartInputReference(c *gin.Context, req *relaycommon.TaskSubmitReq) error {
	if !strings.Contains(c.GetHeader("Content-Type"), "multipart/form-data") {
		return nil
	}
	form, err := common.ParseMultipartFormReusable(c)
	if err != nil {
		return err
	}
	files := form.File["input_reference"]
	if len(files) > 1 {
		return fmt.Errorf("input_reference supports at most 1 uploaded file")
	}
	if len(files) == 0 {
		return nil
	}
	file, err := files[0].Open()
	if err != nil {
		return err
	}
	defer file.Close()
	data, err := io.ReadAll(file)
	if err != nil {
		return err
	}
	contentType := files[0].Header.Get("Content-Type")
	if contentType == "" || contentType == "application/octet-stream" {
		contentType = http.DetectContentType(data)
	}
	req.InputReference = "data:" + contentType + ";base64," + base64.StdEncoding.EncodeToString(data)
	return nil
}

func (a *TaskAdaptor) ValidateRequestAndSetAction(c *gin.Context, info *relaycommon.RelayInfo) *dto.TaskError {
	var req relaycommon.TaskSubmitReq
	if err := common.UnmarshalBodyReusable(c, &req); err != nil {
		return invalidRequest(err)
	}
	if err := appendMultipartInputReference(c, &req); err != nil {
		return invalidRequest(err)
	}
	if strings.HasPrefix(c.Request.URL.Path, "/v1/videos") {
		if err := normalizePublicRequest(&req); err != nil {
			return invalidRequest(err)
		}
	} else {
		// Native /async/tasks keeps the documented envelope while normalizing
		// scalar spellings used by existing clients.
		if req.Input == nil {
			req.Input = map[string]any{}
		}
		if duration, ok := parsePositiveInt(req.Input["duration"]); ok {
			req.Duration = duration
			req.Input["duration"] = duration
		}
		if resolution := inputString(req.Input, "resolution"); resolution != "" {
			req.Input["resolution"] = strings.ToLower(resolution)
		}
		if _, exists := req.Input["n"]; !exists {
			req.Input["n"] = 1
		}
		if _, exists := req.Input["audio"]; !exists {
			req.Input["audio"] = true
		}
	}
	if err := validateSeedanceRequest(req); err != nil {
		return invalidRequest(err)
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
	if a.Protocol == string(dto.VideoUpstreamProtocolSeedanceDiscount) {
		return a.baseURL + "/kyyReactApiServer/v1/seedance-discount/videos", nil
	}
	return a.baseURL + "/async/tasks", nil
}

func (a *TaskAdaptor) BuildRequestHeader(_ *gin.Context, req *http.Request, _ *relaycommon.RelayInfo) error {
	req.Header.Set("Authorization", "Bearer "+a.apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	return nil
}

func discountReferenceContent(input map[string]any, prompt string) []discountContentItem {
	content := []discountContentItem{{Type: "text", Text: prompt}}
	appendRefs := func(key, mediaType, role, valueKey string) {
		for _, raw := range inputArray(input, key) {
			value := raw
			if object, ok := raw.(map[string]any); ok {
				for _, candidate := range []string{"url", valueKey, "data", "base64"} {
					if object[candidate] != nil {
						value = object[candidate]
						break
					}
				}
			}
			item := discountContentItem{Type: mediaType + "_url", Role: role}
			switch mediaType {
			case "image":
				item.ImageURL = &discountMediaURL{URL: value}
			case "video":
				item.VideoURL = &discountMediaURL{URL: value}
			case "audio":
				item.AudioURL = &discountMediaURL{URL: value}
			}
			content = append(content, item)
		}
	}
	appendRefs("start_frames", "image", "first_frame", "image_url")
	appendRefs("end_frames", "image", "last_frame", "image_url")
	appendRefs("image_references", "image", "reference_image", "image_url")
	appendRefs("video_references", "video", "reference_video", "video_url")
	appendRefs("audio_references", "audio", "reference_audio", "audio_url")
	return content
}

func discountModel(modelName, resolution string, hasVideo bool) string {
	family := "sd_2.0"
	if strings.Contains(strings.ToLower(modelName), "fast") {
		family = "sd_2.0_fast"
	}
	name := family + "_discount_" + strings.TrimSuffix(strings.ToLower(resolution), "p") + "p"
	if hasVideo {
		name += "_with_video_ref"
	}
	return name
}

func optionalBool(input map[string]any, key string) (*bool, error) {
	raw, exists := input[key]
	if !exists {
		return nil, nil
	}
	value, ok := raw.(bool)
	if !ok {
		return nil, fmt.Errorf("input.%s must be a boolean", key)
	}
	return &value, nil
}

func optionalInt(input map[string]any, key string) (*int, error) {
	raw, exists := input[key]
	if !exists {
		return nil, nil
	}
	value, ok := parsePositiveInt(raw)
	if !ok {
		return nil, fmt.Errorf("input.%s must be an integer", key)
	}
	return &value, nil
}

func (a *TaskAdaptor) BuildRequestBody(c *gin.Context, info *relaycommon.RelayInfo) (io.Reader, error) {
	req, err := relaycommon.GetTaskRequest(c)
	if err != nil {
		return nil, err
	}
	if isAsyncTaskRequest(c) {
		payload, marshalErr := common.Marshal(map[string]any{"model": info.UpstreamModelName, "input": req.Input})
		if marshalErr != nil {
			return nil, marshalErr
		}
		return bytes.NewReader(payload), nil
	}
	input := map[string]any{
		"prompt":       req.Prompt,
		"duration":     req.Duration,
		"resolution":   strings.ToLower(inputString(req.Input, "resolution")),
		"aspect_ratio": inputString(req.Input, "aspect_ratio"),
		"n":            1,
	}
	if audio, ok := req.Input["audio"].(bool); ok {
		input["audio"] = audio
	} else {
		input["audio"] = true
	}
	for _, key := range []string{"image_references", "video_references", "audio_references", "start_frames", "end_frames"} {
		if values := inputArray(req.Input, key); len(values) > 0 {
			input[key] = values
		}
	}
	var wire any = map[string]any{"model": info.UpstreamModelName, "input": input}
	if a.Protocol == string(dto.VideoUpstreamProtocolSeedanceDiscount) {
		hasVideo := len(inputArray(req.Input, "video_references")) > 0
		modelName := discountModel(req.Model, inputString(req.Input, "resolution"), hasVideo)
		audio, _ := req.Input["audio"].(bool)
		returnLastFrame, optionErr := optionalBool(req.Input, "return_last_frame")
		if optionErr != nil {
			return nil, optionErr
		}
		seed, optionErr := optionalInt(req.Input, "seed")
		if optionErr != nil {
			return nil, optionErr
		}
		wire = discountRequest{
			Model: modelName, Ratio: inputString(req.Input, "aspect_ratio"), Duration: req.Duration,
			GenerateAudio: &audio, ReturnLastFrame: returnLastFrame, Seed: seed,
			Tools: inputArray(req.Input, "tools"), Content: discountReferenceContent(req.Input, req.Prompt),
		}
	}
	payload, err := common.Marshal(wire)
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
		return task, fmt.Errorf("response data is empty: %s", strings.TrimSpace(envelope.Message))
	}
	if err := common.Unmarshal(envelope.Data, &task); err != nil {
		return task, err
	}
	if task.TaskID == "" {
		task.TaskID = task.ID
	}
	if task.ID == "" {
		task.ID = task.TaskID
	}
	return task, nil
}

func (a *TaskAdaptor) parseUpstreamTask(body []byte) (taskWire, error) {
	if a.Protocol == string(dto.VideoUpstreamProtocolSeedanceDiscount) {
		var task taskWire
		if err := common.Unmarshal(body, &task); err != nil {
			return task, err
		}
		if task.TaskID == "" {
			task.TaskID = task.ID
		}
		if task.ID == "" {
			task.ID = task.TaskID
		}
		return task, nil
	}
	return parseNestedTask(body)
}

func readableUpstreamError(resp *http.Response, body []byte) error {
	contentType := resp.Header.Get("Content-Type")
	summary := strings.TrimSpace(string(body))
	if len(summary) > 240 {
		summary = summary[:240] + "..."
	}
	if strings.Contains(strings.ToLower(contentType), "text/html") {
		summary = "upstream returned HTML instead of JSON"
	}
	return fmt.Errorf("upstream status=%d content-type=%q: %s", resp.StatusCode, contentType, summary)
}

func openAIStatus(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "queued", "pending", "submitted", "not_start":
		return dto.VideoStatusQueued
	case "running", "processing", "in_progress":
		return dto.VideoStatusInProgress
	case "success", "succeeded", "completed", "complete":
		return dto.VideoStatusCompleted
	case "failure", "failed", "cancelled", "canceled":
		return dto.VideoStatusFailed
	default:
		return dto.VideoStatusUnknown
	}
}

func toOpenAIVideo(task taskWire, publicID, modelName string) map[string]any {
	if publicID == "" {
		publicID = task.TaskID
	}
	if modelName == "" {
		modelName = task.Model
	}
	status := openAIStatus(task.Status)
	progress := 0
	switch status {
	case dto.VideoStatusQueued:
		progress = 20
	case dto.VideoStatusInProgress:
		progress = 50
	case dto.VideoStatusCompleted, dto.VideoStatusFailed:
		progress = 100
	}
	result := map[string]any{"id": publicID, "object": "video", "model": modelName, "status": status, "progress": progress}
	created := task.CreatedAt
	if created == 0 {
		created = task.Created
	}
	if created > 0 {
		result["created_at"] = created
	}
	if videoURL := outputURL(task); videoURL != "" {
		result["metadata"] = map[string]any{"url": videoURL}
	}
	message := strings.TrimSpace(task.FailReason)
	if message == "" && task.Error != nil {
		message = strings.TrimSpace(task.Error.Message)
	}
	if message != "" {
		result["error"] = map[string]any{"message": message}
	}
	return result
}

func isAsyncTaskRequest(c *gin.Context) bool {
	return c != nil && c.Request != nil && strings.HasPrefix(c.Request.URL.Path, "/async/tasks")
}

func (a *TaskAdaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (string, []byte, *dto.TaskError) {
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", nil, service.TaskErrorWrapper(err, "read_response_body_failed", http.StatusInternalServerError)
	}
	_ = resp.Body.Close()
	if (resp.StatusCode != 0 && (resp.StatusCode < 200 || resp.StatusCode >= 300)) || strings.Contains(strings.ToLower(resp.Header.Get("Content-Type")), "text/html") {
		return "", nil, service.TaskErrorWrapper(readableUpstreamError(resp, body), "invalid_upstream_response", http.StatusBadGateway)
	}
	task, err := a.parseUpstreamTask(body)
	if err != nil {
		return "", nil, service.TaskErrorWrapper(readableUpstreamError(resp, body), "invalid_response", http.StatusBadGateway)
	}
	if strings.TrimSpace(task.TaskID) == "" {
		return "", nil, service.TaskErrorWrapper(fmt.Errorf("upstream response did not contain task_id"), "invalid_response", http.StatusBadGateway)
	}
	if isAsyncTaskRequest(c) {
		publicBody, rewriteErr := rewritePublicTaskIDs(body, info.PublicTaskID)
		if rewriteErr != nil {
			return "", nil, service.TaskErrorWrapper(rewriteErr, "rewrite_response_failed", http.StatusInternalServerError)
		}
		c.Data(http.StatusOK, "application/json", publicBody)
	} else {
		c.JSON(http.StatusOK, toOpenAIVideo(task, info.PublicTaskID, info.OriginModelName))
	}
	return task.TaskID, body, nil
}

func (a *TaskAdaptor) FetchTask(baseURL, key string, body map[string]any, proxy string) (*http.Response, error) {
	taskID, ok := body["task_id"].(string)
	if !ok || strings.TrimSpace(taskID) == "" {
		return nil, fmt.Errorf("invalid task_id")
	}
	baseURL = normalizeBaseURL(baseURL)
	path := "/async/tasks/" + url.PathEscape(taskID)
	if a.Protocol == string(dto.VideoUpstreamProtocolSeedanceDiscount) {
		path = "/kyyReactApiServer/v1/result/" + url.PathEscape(taskID)
	}
	req, err := http.NewRequest(http.MethodGet, baseURL+path, nil)
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
	if strings.TrimSpace(task.VideoURL) != "" {
		return strings.TrimSpace(task.VideoURL)
	}
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
	task, err := a.parseUpstreamTask(body)
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
	result.TotalTokens = task.TotalTokens
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

func (a *TaskAdaptor) ConvertToOpenAIVideo(task *model.Task) ([]byte, error) {
	if len(task.Data) == 0 {
		return nil, fmt.Errorf("task response is empty")
	}
	upstream, err := a.parseUpstreamTask(task.Data)
	if err != nil {
		return nil, errors.Wrap(err, "unmarshal Seedance task data failed")
	}
	video := toOpenAIVideo(upstream, task.TaskID, task.Properties.OriginModelName)
	video["status"] = task.Status.ToVideoStatus()
	if task.Progress != "" {
		if progress, parseErr := strconv.Atoi(strings.TrimSuffix(task.Progress, "%")); parseErr == nil {
			video["progress"] = progress
		}
	}
	if resultURL := task.GetResultURL(); resultURL != "" {
		video["metadata"] = map[string]any{"url": resultURL}
	}
	if task.FailReason != "" {
		video["error"] = map[string]any{"message": task.FailReason}
	}
	return common.Marshal(video)
}

func (a *TaskAdaptor) GetModelList() []string { return []string{"seedance-2.0", "seedance-2.0-fast"} }
func (a *TaskAdaptor) GetChannelName() string { return "Seedance Async" }
