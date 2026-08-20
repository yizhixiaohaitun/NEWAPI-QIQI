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
	"time"

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
)

const seedanceTasksPath = "/async/tasks"

var windowsPathPattern = regexp.MustCompile(`^[A-Za-z]:[\\/]`)

var modelSpecs = map[string]map[string]struct{}{
	"sd_2.0_discount":      resolutionSet("480p", "720p", "1080p"),
	"sd_2.0_fast_discount": resolutionSet("480p", "720p"),
	"sd_2.0_mini_discount": resolutionSet("480p", "720p"),
	"sd_2.0_special":       resolutionSet("720p", "1080p"),
	"sd_2.0_fast_special":  resolutionSet("720p"),
	"sd_2.0_mini_special":  resolutionSet("720p"),
}

var modelAliases = map[string]string{
	"seedance-2.0":      "sd_2.0_discount",
	"seedance-2.0-fast": "sd_2.0_fast_discount",
}

var allowedRatios = map[string]struct{}{
	"16:9": {}, "9:16": {}, "1:1": {}, "4:3": {}, "3:4": {}, "21:9": {}, "adaptive": {},
}

type TaskAdaptor struct {
	taskcommon.BaseBilling
	apiKey  string
	baseURL string
}

type seedanceRequest struct {
	Model string         `json:"model"`
	Input map[string]any `json:"input"`
}

type responseWire struct {
	Success bool            `json:"success"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

func redactReferenceResource(value string) string {
	value = strings.TrimSpace(value)
	parsed, err := url.Parse(value)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return value
	}
	query := parsed.Query()
	changed := false
	for key := range query {
		normalized := strings.ToLower(strings.NewReplacer("_", "", "-", "").Replace(key))
		switch normalized {
		case "authorization", "apikey", "accesstoken", "refreshtoken", "credential", "password":
			query.Set(key, "[REDACTED]")
			changed = true
		}
	}
	if changed {
		parsed.RawQuery = query.Encode()
	}
	return parsed.String()
}

func taskReferenceResources(req *seedanceRequest) []string {
	if req == nil {
		return nil
	}
	keys := []string{"image_references", "video_references", "audio_references", "start_frames", "end_frames"}
	candidates := make([]string, 0)
	var collect func(any)
	collect = func(value any) {
		switch item := value.(type) {
		case string:
			candidates = append(candidates, item)
		case []any:
			for _, child := range item {
				collect(child)
			}
		case map[string]any:
			for _, key := range []string{"url", "image_url", "video_url", "audio_url", "data", "base64"} {
				if child, ok := item[key]; ok {
					collect(child)
					break
				}
			}
		}
	}
	for _, key := range keys {
		collect(req.Input[key])
	}

	seen := make(map[string]struct{}, len(candidates))
	resources := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		candidate = redactReferenceResource(candidate)
		if candidate == "" {
			continue
		}
		if _, ok := seen[candidate]; ok {
			continue
		}
		seen[candidate] = struct{}{}
		resources = append(resources, candidate)
	}
	return resources
}

type responseError struct {
	Message string `json:"message,omitempty"`
	Code    string `json:"code,omitempty"`
}

type taskWire struct {
	ID             string          `json:"id"`
	TaskID         string          `json:"task_id,omitempty"`
	Object         string          `json:"object"`
	Created        int64           `json:"created"`
	CreatedAt      int64           `json:"created_at,omitempty"`
	Model          string          `json:"model"`
	Status         string          `json:"status"`
	Progress       json.RawMessage `json:"progress,omitempty"`
	FailReason     string          `json:"fail_reason,omitempty"`
	Data           json.RawMessage `json:"data,omitempty"`
	Outputs        []any           `json:"outputs,omitempty"`
	ResultURL      string          `json:"result_url,omitempty"`
	VideoURL       string          `json:"video_url,omitempty"`
	LastFrameURL   string          `json:"last_frame_url,omitempty"`
	UpstreamURL    string          `json:"upstream_url,omitempty"`
	UpstreamURLs   []string        `json:"upstream_urls,omitempty"`
	Error          json.RawMessage `json:"error,omitempty"`
	Message        string          `json:"message,omitempty"`
	Msg            string          `json:"msg,omitempty"`
	Amount         float64         `json:"amount,omitempty"`
	ActualDuration any             `json:"actualDuration,omitempty"`
	TotalTokens    int             `json:"totalTokens,omitempty"`
	TotalTokensAlt int             `json:"total_tokens,omitempty"`
}

type openAIVideoWire struct {
	ID           string                 `json:"id"`
	TaskID       string                 `json:"task_id,omitempty"`
	Object       string                 `json:"object"`
	Model        string                 `json:"model"`
	Status       string                 `json:"status"`
	Progress     int                    `json:"progress"`
	CreatedAt    int64                  `json:"created_at"`
	CompletedAt  int64                  `json:"completed_at,omitempty"`
	VideoURL     string                 `json:"video_url,omitempty"`
	LastFrameURL string                 `json:"last_frame_url,omitempty"`
	Error        *dto.OpenAIVideoError  `json:"error,omitempty"`
	Metadata     map[string]interface{} `json:"metadata,omitempty"`
}

func resolutionSet(values ...string) map[string]struct{} {
	result := make(map[string]struct{}, len(values))
	for _, value := range values {
		result[value] = struct{}{}
	}
	return result
}

func normalizedModel(value string) string { return strings.ToLower(strings.TrimSpace(value)) }

func canonicalModel(value string) (string, bool) {
	value = normalizedModel(value)
	if target, ok := modelAliases[value]; ok {
		value = target
	}
	_, ok := modelSpecs[value]
	return value, ok
}

func IsSupportedModel(value string) bool {
	_, ok := canonicalModel(value)
	return ok
}

func (a *TaskAdaptor) Init(info *relaycommon.RelayInfo) {
	a.apiKey = info.ApiKey
	a.baseURL = strings.TrimRight(info.ChannelBaseUrl, "/")
}

func parseInteger(value any) (int, bool) {
	switch number := value.(type) {
	case float64:
		if number == float64(int(number)) {
			return int(number), true
		}
	case json.Number:
		result, err := strconv.Atoi(number.String())
		return result, err == nil
	case string:
		result, err := strconv.Atoi(strings.TrimSpace(number))
		return result, err == nil
	case int:
		return number, true
	}
	return 0, false
}

func inputString(input map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := input[key].(string); ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func inputArray(input map[string]any, keys ...string) []any {
	for _, key := range keys {
		if values, ok := input[key].([]any); ok {
			return values
		}
		if values, ok := input[key].([]string); ok {
			result := make([]any, len(values))
			for i, value := range values {
				result[i] = value
			}
			return result
		}
	}
	return nil
}

func inputValue(input map[string]any, metadata map[string]any, keys ...string) (any, bool) {
	for _, source := range []map[string]any{input, metadata} {
		for _, key := range keys {
			if source != nil {
				if value, ok := source[key]; ok {
					return value, true
				}
			}
		}
	}
	return nil, false
}

func invalidRequest(err error) *dto.TaskError {
	return service.TaskErrorWrapperLocal(err, "invalid_request", http.StatusBadRequest)
}

func mergeTopLevelCompatibilityFields(body []byte, req *relaycommon.TaskSubmitReq) error {
	var root map[string]any
	if err := common.Unmarshal(body, &root); err != nil {
		return err
	}
	if req.Input == nil {
		req.Input = make(map[string]any)
	}
	for _, key := range []string{
		"duration", "seconds", "aspect_ratio", "ratio", "resolution", "generate_audio", "audio", "watermark", "seed", "tools",
		"reference_images", "image_references", "reference_videos", "video_references",
		"reference_audios", "audio_references", "first_image", "last_image", "start_frames", "end_frames",
	} {
		if _, exists := req.Input[key]; exists {
			continue
		}
		if value, exists := root[key]; exists {
			req.Input[key] = value
		}
	}
	if rawReferences, exists := root["input_reference"].([]any); exists {
		for _, rawReference := range rawReferences {
			reference, ok := rawReference.(map[string]any)
			if !ok {
				return fmt.Errorf("input_reference items must be objects")
			}
			mediaType, _ := reference["type"].(string)
			mediaType = strings.ToLower(strings.TrimSpace(mediaType))
			var targetKey, valueKey string
			switch mediaType {
			case "image":
				targetKey, valueKey = "image_references", "image_url"
			case "video":
				targetKey, valueKey = "video_references", "video_url"
			case "audio":
				targetKey, valueKey = "audio_references", "audio_url"
			default:
				return fmt.Errorf("unsupported input_reference type: %s", mediaType)
			}
			value, exists := reference[valueKey]
			if !exists {
				return fmt.Errorf("input_reference %s item is missing %s", mediaType, valueKey)
			}
			if strength, exists := reference["strength"]; exists {
				value = map[string]any{"url": value, "strength": strength}
			}
			req.Input[targetKey] = append(inputArray(req.Input, targetKey), value)
		}
	}
	return mergeContentResources(root["content"], req)
}

func mergeContentResources(raw any, req *relaycommon.TaskSubmitReq) error {
	items, ok := raw.([]any)
	if raw == nil {
		return nil
	}
	if !ok {
		return fmt.Errorf("content must be an array")
	}

	resources := map[string][]any{
		"reference_images": {},
		"reference_videos": {},
		"reference_audios": {},
	}
	for _, rawItem := range items {
		item, ok := rawItem.(map[string]any)
		if !ok {
			return fmt.Errorf("content items must be objects")
		}
		contentType, _ := item["type"].(string)
		role, _ := item["role"].(string)
		switch strings.ToLower(strings.TrimSpace(contentType)) {
		case "text":
			if strings.TrimSpace(req.Prompt) == "" {
				if text, ok := item["text"].(string); ok {
					req.Prompt = strings.TrimSpace(text)
				}
			}
		case "image_url":
			value, exists := item["image_url"]
			if !exists {
				return fmt.Errorf("content image_url item is missing image_url")
			}
			switch strings.ToLower(strings.TrimSpace(role)) {
			case "first_frame":
				if _, exists := req.Input["first_image"]; !exists {
					req.Input["first_image"] = value
				}
			case "last_frame":
				if _, exists := req.Input["last_image"]; !exists {
					req.Input["last_image"] = value
				}
			default:
				resources["reference_images"] = append(resources["reference_images"], value)
			}
		case "video_url":
			value, exists := item["video_url"]
			if !exists {
				return fmt.Errorf("content video_url item is missing video_url")
			}
			resources["reference_videos"] = append(resources["reference_videos"], value)
		case "audio_url":
			value, exists := item["audio_url"]
			if !exists {
				return fmt.Errorf("content audio_url item is missing audio_url")
			}
			resources["reference_audios"] = append(resources["reference_audios"], value)
		default:
			return fmt.Errorf("unsupported content type: %s", contentType)
		}
	}
	for key, values := range resources {
		if len(values) == 0 {
			continue
		}
		if _, exists := req.Input[key]; !exists {
			req.Input[key] = values
		}
	}
	return nil
}

func validMediaReference(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" {
		return false
	}
	lower := strings.ToLower(value)
	if strings.HasPrefix(lower, "file://") || strings.HasPrefix(value, "/") || strings.HasPrefix(value, `\\`) || windowsPathPattern.MatchString(value) {
		return false
	}
	if strings.HasPrefix(lower, "assetid://") {
		return len(strings.TrimSpace(value[len("assetId://"):])) > 0
	}
	if strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") {
		parsed, err := url.ParseRequestURI(value)
		return err == nil && parsed.Host != ""
	}
	return false
}

func mediaReference(value any, preferredKeys ...string) (string, error) {
	switch item := value.(type) {
	case string:
		item = strings.TrimSpace(item)
		if !validMediaReference(item) {
			return "", fmt.Errorf("media reference must be an HTTP(S) URL or assetId:// reference")
		}
		return item, nil
	case map[string]any:
		keys := append([]string{}, preferredKeys...)
		keys = append(keys, "url", "image_url", "video_url", "audio_url")
		for _, key := range keys {
			if nested, ok := item[key]; ok {
				return mediaReference(nested, preferredKeys...)
			}
		}
		return "", fmt.Errorf("media reference object must contain a URL")
	default:
		return "", fmt.Errorf("invalid media reference")
	}
}

func mediaReferences(input map[string]any, max int, keys ...string) ([]string, error) {
	items := inputArray(input, keys...)
	if raw, exists := inputValue(input, nil, keys...); exists && raw != nil && items == nil {
		return nil, fmt.Errorf("%s must be an array", keys[0])
	}
	if len(items) > max {
		return nil, fmt.Errorf("%s supports at most %d item(s)", keys[0], max)
	}
	result := make([]string, 0, len(items))
	for _, item := range items {
		value, err := mediaReference(item, keys...)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", keys[0], err)
		}
		result = append(result, value)
	}
	return result, nil
}

func firstMediaReference(input map[string]any, keys ...string) (string, error) {
	for _, key := range keys {
		raw, exists := input[key]
		if !exists || raw == nil {
			continue
		}
		if items, ok := raw.([]any); ok {
			if len(items) == 0 {
				continue
			}
			if len(items) > 1 {
				return "", fmt.Errorf("%s supports at most 1 item", key)
			}
			return mediaReference(items[0], key)
		}
		return mediaReference(raw, key)
	}
	return "", nil
}

func requestDuration(req relaycommon.TaskSubmitReq) (int, error) {
	for _, source := range []map[string]any{req.Input, req.Metadata} {
		for _, key := range []string{"duration", "seconds"} {
			candidate, exists := source[key]
			if !exists {
				continue
			}
			result, ok := parseInteger(candidate)
			if !ok || result < 4 || result > 15 {
				return 0, fmt.Errorf("duration must be an integer between 4 and 15")
			}
			return result, nil
		}
	}
	if req.Duration != 0 {
		if req.Duration < 4 || req.Duration > 15 {
			return 0, fmt.Errorf("duration must be an integer between 4 and 15")
		}
		return req.Duration, nil
	}
	if strings.TrimSpace(req.Seconds) != "" {
		result, ok := parseInteger(req.Seconds)
		if !ok || result < 4 || result > 15 {
			return 0, fmt.Errorf("duration must be an integer between 4 and 15")
		}
		return result, nil
	}
	return 5, nil
}

func requestAspectRatio(req relaycommon.TaskSubmitReq) (string, error) {
	value := inputString(req.Input, "aspect_ratio", "ratio")
	if value == "" {
		value = inputString(req.Metadata, "aspect_ratio", "ratio")
	}
	if value == "" {
		value = ratioFromSize(req.Size)
	}
	if value == "" {
		value = "16:9"
	}
	if _, ok := allowedRatios[value]; !ok {
		return "", fmt.Errorf("aspect_ratio must be one of 16:9, 9:16, 1:1, 4:3, 3:4, 21:9, or adaptive")
	}
	return value, nil
}

func ratioFromSize(size string) string {
	parts := strings.Split(strings.ToLower(strings.TrimSpace(size)), "x")
	if len(parts) != 2 {
		return ""
	}
	width, widthErr := strconv.Atoi(strings.TrimSpace(parts[0]))
	height, heightErr := strconv.Atoi(strings.TrimSpace(parts[1]))
	if widthErr != nil || heightErr != nil || width <= 0 || height <= 0 {
		return ""
	}
	ratio := float64(width) / float64(height)
	candidates := []struct {
		name  string
		value float64
	}{{"16:9", 16.0 / 9.0}, {"9:16", 9.0 / 16.0}, {"1:1", 1}, {"4:3", 4.0 / 3.0}, {"3:4", 3.0 / 4.0}, {"21:9", 21.0 / 9.0}}
	bestName, bestDiff := "", 1.0
	for _, candidate := range candidates {
		diff := ratio - candidate.value
		if diff < 0 {
			diff = -diff
		}
		if diff < bestDiff {
			bestName, bestDiff = candidate.name, diff
		}
	}
	if bestDiff <= 0.08 {
		return bestName
	}
	return "adaptive"
}

func requestResolution(req relaycommon.TaskSubmitReq) (string, error) {
	value := inputString(req.Input, "resolution", "size")
	if value == "" {
		value = inputString(req.Metadata, "resolution", "size")
	}
	if value == "" {
		value = strings.TrimSpace(req.Size)
	}
	if value == "" {
		return "720p", nil
	}
	value = strings.ToLower(strings.TrimSpace(value))
	switch value {
	case "480p", "480":
		return "480p", nil
	case "720p", "720":
		return "720p", nil
	case "1080p", "1080":
		return "1080p", nil
	}
	parts := strings.Split(value, "x")
	if len(parts) == 2 {
		width, widthErr := strconv.Atoi(strings.TrimSpace(parts[0]))
		height, heightErr := strconv.Atoi(strings.TrimSpace(parts[1]))
		if widthErr == nil && heightErr == nil && width > 0 && height > 0 {
			shortSide := min(width, height)
			switch {
			case shortSide <= 480:
				return "480p", nil
			case shortSide <= 720:
				return "720p", nil
			default:
				return "1080p", nil
			}
		}
	}
	return "", fmt.Errorf("size or resolution must map to 480p, 720p, or 1080p")
}

func normalizedInput(req relaycommon.TaskSubmitReq) map[string]any {
	result := make(map[string]any, len(req.Input)+len(req.Metadata))
	for key, value := range req.Metadata {
		result[key] = value
	}
	for key, value := range req.Input {
		result[key] = value
	}
	return result
}

func boolOption(req relaycommon.TaskSubmitReq, keys ...string) (*bool, error) {
	value, exists := inputValue(req.Input, req.Metadata, keys...)
	if !exists {
		return nil, nil
	}
	result, ok := value.(bool)
	if !ok {
		return nil, fmt.Errorf("%s must be a boolean", keys[0])
	}
	return &result, nil
}

func intOption(req relaycommon.TaskSubmitReq, key string) (*int, error) {
	value, exists := inputValue(req.Input, req.Metadata, key)
	if !exists {
		return nil, nil
	}
	result, ok := parseInteger(value)
	if !ok {
		return nil, fmt.Errorf("%s must be an integer", key)
	}
	return &result, nil
}

func officialSeedanceModel(value string) (string, error) {
	value = normalizedModel(value)
	switch {
	case value == "seedance-2.0", value == "seedance-2.0-fast":
		return value, nil
	case strings.HasPrefix(value, "sd_2.0_") && strings.Contains(value, "fast"):
		return "seedance-2.0-fast", nil
	case strings.HasPrefix(value, "sd_2.0_"):
		return "seedance-2.0", nil
	default:
		return "", fmt.Errorf("unsupported Seedance model: %s", value)
	}
}

func validOfficialMediaValue(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" {
		return false
	}
	lower := strings.ToLower(value)
	if strings.HasPrefix(lower, "file:") || strings.HasPrefix(value, "/") || strings.HasPrefix(value, `\\`) || windowsPathPattern.MatchString(value) {
		return false
	}
	if strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") {
		parsed, err := url.ParseRequestURI(value)
		return err == nil && parsed.Host != ""
	}
	if strings.HasPrefix(lower, "data:") {
		return strings.Contains(value, ",")
	}
	if strings.HasPrefix(lower, "assetid://") {
		return len(strings.TrimSpace(value[len("assetId://"):])) > 0
	}
	if _, err := base64.StdEncoding.DecodeString(value); err == nil {
		return true
	}
	_, err := base64.RawStdEncoding.DecodeString(value)
	return err == nil
}

func officialMediaReference(value any, mediaType string) (any, error) {
	switch item := value.(type) {
	case string:
		item = strings.TrimSpace(item)
		if !validOfficialMediaValue(item) {
			return nil, fmt.Errorf("media reference must be an HTTP(S) URL, data URI, base64 value, or assetId:// reference")
		}
		return item, nil
	case map[string]any:
		var raw any
		for _, key := range []string{mediaType + "_url", "url", "data", "base64"} {
			if candidate, exists := item[key]; exists {
				raw = candidate
				break
			}
		}
		if raw == nil {
			return nil, fmt.Errorf("media reference object must contain a URL, data, or base64 value")
		}
		if _, exists := item["strength"]; exists && mediaType != "image" {
			return nil, fmt.Errorf("%s references do not support strength", mediaType)
		}
		normalized, err := officialMediaReference(raw, mediaType)
		if err != nil {
			return nil, err
		}
		if strength, exists := item["strength"]; exists {
			return map[string]any{"url": normalized, "strength": strength}, nil
		}
		return normalized, nil
	default:
		return nil, fmt.Errorf("invalid media reference")
	}
}

func officialMediaReferences(input map[string]any, mediaType string, max int, keys ...string) ([]any, error) {
	items := inputArray(input, keys...)
	if raw, exists := inputValue(input, nil, keys...); exists && raw != nil && items == nil {
		return nil, fmt.Errorf("%s must be an array", keys[0])
	}
	if len(items) > max {
		return nil, fmt.Errorf("%s supports at most %d item(s)", keys[0], max)
	}
	result := make([]any, 0, len(items))
	for _, item := range items {
		normalized, err := officialMediaReference(item, mediaType)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", keys[0], err)
		}
		result = append(result, normalized)
	}
	return result, nil
}

func officialFrameReferences(input map[string]any, keys ...string) ([]any, error) {
	for _, key := range keys {
		raw, exists := input[key]
		if !exists || raw == nil {
			continue
		}
		items, ok := raw.([]any)
		if !ok {
			items = []any{raw}
		}
		if len(items) > 1 {
			return nil, fmt.Errorf("%s supports at most 1 item", key)
		}
		if len(items) == 0 {
			return nil, nil
		}
		value, err := officialMediaReference(items[0], "image")
		if err != nil {
			return nil, fmt.Errorf("%s: %w", key, err)
		}
		return []any{value}, nil
	}
	return nil, nil
}

func buildSeedanceRequest(req relaycommon.TaskSubmitReq, upstreamModel string) (*seedanceRequest, error) {
	modelName, err := officialSeedanceModel(upstreamModel)
	if err != nil {
		modelName, err = officialSeedanceModel(req.Model)
	}
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(req.Prompt) == "" {
		return nil, fmt.Errorf("prompt is required")
	}
	canonical, ok := canonicalModel(req.Model)
	if !ok {
		canonical, ok = canonicalModel(upstreamModel)
	}
	if !ok {
		return nil, fmt.Errorf("unsupported Seedance model: %s", req.Model)
	}
	resolution, err := requestResolution(req)
	if err != nil {
		return nil, err
	}
	if _, ok := modelSpecs[canonical][resolution]; !ok {
		return nil, fmt.Errorf("model %s does not support resolution %s", canonical, resolution)
	}
	duration, err := requestDuration(req)
	if err != nil {
		return nil, err
	}
	aspectRatio, err := requestAspectRatio(req)
	if err != nil {
		return nil, err
	}
	input := normalizedInput(req)
	images, err := officialMediaReferences(input, "image", 4, "image_references", "reference_images")
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(req.InputReference) == "" && strings.TrimSpace(req.Image) == "" {
		for _, rawImage := range req.Images {
			image, imageErr := officialMediaReference(rawImage, "image")
			if imageErr != nil {
				return nil, fmt.Errorf("images: %w", imageErr)
			}
			images = append(images, image)
		}
	}
	if len(images) > 4 {
		return nil, fmt.Errorf("image_references supports at most 4 item(s)")
	}
	videos, err := officialMediaReferences(input, "video", 3, "video_references", "reference_videos")
	if err != nil {
		return nil, err
	}
	audios, err := officialMediaReferences(input, "audio", 1, "audio_references", "reference_audios")
	if err != nil {
		return nil, err
	}
	startFrames, err := officialFrameReferences(input, "start_frames", "first_image")
	if err != nil {
		return nil, err
	}
	endFrames, err := officialFrameReferences(input, "end_frames", "last_image")
	if err != nil {
		return nil, err
	}
	if len(startFrames) == 0 {
		for _, candidate := range []string{req.InputReference, req.Image} {
			if strings.TrimSpace(candidate) == "" {
				continue
			}
			value, valueErr := officialMediaReference(candidate, "image")
			if valueErr != nil {
				return nil, fmt.Errorf("input_reference: %w", valueErr)
			}
			startFrames = []any{value}
			break
		}
	}
	if len(endFrames) > 0 && len(startFrames) == 0 {
		return nil, fmt.Errorf("end_frames requires start_frames")
	}
	audio, err := boolOption(req, "generate_audio", "audio")
	if err != nil {
		return nil, err
	}
	inputPayload := map[string]any{
		"prompt": strings.TrimSpace(req.Prompt), "duration": duration, "aspect_ratio": aspectRatio,
		"resolution": resolution, "n": 1,
	}
	if audio != nil {
		inputPayload["audio"] = *audio
	}
	if len(images) > 0 {
		inputPayload["image_references"] = images
	}
	if len(videos) > 0 {
		inputPayload["video_references"] = videos
	}
	if len(audios) > 0 {
		inputPayload["audio_references"] = audios
	}
	if len(startFrames) > 0 {
		inputPayload["start_frames"] = startFrames
	}
	if len(endFrames) > 0 {
		inputPayload["end_frames"] = endFrames
	}
	if watermark, optionErr := boolOption(req, "watermark"); optionErr != nil {
		return nil, optionErr
	} else if watermark != nil {
		inputPayload["watermark"] = *watermark
	}
	if seed, optionErr := intOption(req, "seed"); optionErr != nil {
		return nil, optionErr
	} else if seed != nil {
		inputPayload["seed"] = *seed
	}
	if tools := inputArray(input, "tools"); len(tools) > 0 {
		inputPayload["tools"] = tools
	}
	return &seedanceRequest{Model: modelName, Input: inputPayload}, nil
}

func endpointURL(baseURL string, taskID string) string {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	parsed, err := url.Parse(baseURL)
	if err == nil && parsed.Scheme != "" && parsed.Host != "" {
		path := strings.TrimRight(parsed.Path, "/")
		lowerPath := strings.ToLower(path)
		if index := strings.Index(lowerPath, "/kyyreactapiserver"); index >= 0 {
			path = path[:index]
		} else if index := strings.Index(lowerPath, seedanceTasksPath); index >= 0 {
			path = path[:index]
		} else if strings.HasSuffix(lowerPath, "/v1") {
			path = path[:len(path)-len("/v1")]
		}
		parsed.Path = strings.TrimRight(path, "/") + seedanceTasksPath
		parsed.RawPath, parsed.RawQuery, parsed.Fragment = "", "", ""
		baseURL = parsed.String()
	} else {
		baseURL = strings.TrimSuffix(baseURL, "/v1") + seedanceTasksPath
	}
	if taskID != "" {
		baseURL += "/" + url.PathEscape(taskID)
	}
	return baseURL
}

func (a *TaskAdaptor) ValidateRequestAndSetAction(c *gin.Context, info *relaycommon.RelayInfo) *dto.TaskError {
	if strings.Contains(c.GetHeader("Content-Type"), "multipart/form-data") {
		if taskErr := relaycommon.ValidateMultipartDirect(c, info); taskErr != nil {
			return taskErr
		}
		form, err := common.ParseMultipartFormReusable(c)
		if err != nil {
			return invalidRequest(err)
		}
		if len(form.File["input_reference"]) > 0 {
			return invalidRequest(fmt.Errorf("file input_reference is not supported by this upstream; upload it through /v1/video/assets and pass assetId://{asset_id}"))
		}
	} else {
		storage, err := common.GetBodyStorage(c)
		if err != nil {
			return invalidRequest(err)
		}
		body, err := storage.Bytes()
		if err != nil {
			return invalidRequest(err)
		}
		requestBody := body
		var root map[string]any
		if err := common.Unmarshal(body, &root); err != nil {
			return invalidRequest(err)
		}
		if _, isArray := root["input_reference"].([]any); isArray {
			delete(root, "input_reference")
			requestBody, err = common.Marshal(root)
			if err != nil {
				return invalidRequest(err)
			}
		}
		var req relaycommon.TaskSubmitReq
		if err := common.Unmarshal(requestBody, &req); err != nil {
			return invalidRequest(err)
		}
		if err := mergeTopLevelCompatibilityFields(body, &req); err != nil {
			return invalidRequest(err)
		}
		info.Action = constant.TaskActionGenerate
		c.Set("task_request", req)
	}
	req, err := relaycommon.GetTaskRequest(c)
	if err != nil {
		return invalidRequest(err)
	}
	if !IsSupportedModel(req.Model) {
		return invalidRequest(fmt.Errorf("unsupported Seedance model: %s", req.Model))
	}
	payload, err := buildSeedanceRequest(req, req.Model)
	if err != nil {
		return invalidRequest(err)
	}
	info.TaskInput = strings.TrimSpace(req.Prompt)
	info.ReferenceResources = taskReferenceResources(payload)
	info.Action = constant.TaskActionGenerate
	return nil
}

func (a *TaskAdaptor) EstimateBilling(c *gin.Context, _ *relaycommon.RelayInfo) map[string]float64 {
	req, err := relaycommon.GetTaskRequest(c)
	if err != nil {
		return nil
	}
	resolution, err := requestResolution(req)
	if err != nil {
		return nil
	}
	duration, err := requestDuration(req)
	if err != nil {
		return nil
	}
	resolutionRatio := 1.0
	switch resolution {
	case "480p":
		resolutionRatio = 0.5
	case "1080p":
		resolutionRatio = 2.5
	}
	return map[string]float64{"seconds": float64(duration), "resolution": resolutionRatio}
}

func (a *TaskAdaptor) BuildRequestURL(_ *relaycommon.RelayInfo) (string, error) {
	return endpointURL(a.baseURL, ""), nil
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
	payload, err := buildSeedanceRequest(req, info.UpstreamModelName)
	if err != nil {
		return nil, err
	}
	data, err := common.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return bytes.NewReader(data), nil
}

func (a *TaskAdaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (*http.Response, error) {
	return channel.DoTaskApiRequest(a, c, info, requestBody)
}

func parseTask(body []byte) (taskWire, error) {
	var root map[string]json.RawMessage
	if err := common.Unmarshal(body, &root); err != nil {
		return taskWire{}, err
	}
	payload := body
	if raw, exists := root["success"]; exists && len(raw) > 0 {
		var envelope responseWire
		if err := common.Unmarshal(body, &envelope); err != nil {
			return taskWire{}, err
		}
		if len(envelope.Data) == 0 || string(envelope.Data) == "null" {
			return taskWire{}, fmt.Errorf("upstream response data is empty: %s", envelope.Message)
		}
		payload = envelope.Data
	}
	var task taskWire
	if err := common.Unmarshal(payload, &task); err != nil {
		return task, err
	}
	if task.ID == "" {
		task.ID = task.TaskID
	}
	if task.TaskID == "" {
		task.TaskID = task.ID
	}
	if task.Status == "" {
		task.Status = "queued"
	}
	return task, nil
}

func taskErrorMessage(task taskWire) string {
	if len(task.Error) > 0 && string(task.Error) != "null" {
		var text string
		if common.Unmarshal(task.Error, &text) == nil && strings.TrimSpace(text) != "" {
			return strings.TrimSpace(text)
		}
		var object responseError
		if common.Unmarshal(task.Error, &object) == nil && strings.TrimSpace(object.Message) != "" {
			return strings.TrimSpace(object.Message)
		}
	}
	if strings.TrimSpace(task.FailReason) != "" {
		return strings.TrimSpace(task.FailReason)
	}
	if strings.TrimSpace(task.Message) != "" {
		return strings.TrimSpace(task.Message)
	}
	return strings.TrimSpace(task.Msg)
}

func outputURL(task taskWire) string {
	for _, candidate := range append([]string{task.ResultURL, task.VideoURL, task.UpstreamURL}, task.UpstreamURLs...) {
		if strings.TrimSpace(candidate) != "" {
			return strings.TrimSpace(candidate)
		}
	}
	outputs := task.Outputs
	if len(outputs) == 0 && len(task.Data) > 0 && string(task.Data) != "null" {
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
				return strings.TrimSpace(value)
			}
		case map[string]any:
			for _, key := range []string{"url", "video_url", "file_url", "output_url"} {
				if candidate, ok := value[key].(string); ok && strings.TrimSpace(candidate) != "" {
					return strings.TrimSpace(candidate)
				}
			}
		}
	}
	return ""
}

func openAIStatus(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "queued", "pending", "submitted", "not_start":
		return dto.VideoStatusQueued
	case "processing", "running", "in_progress":
		return dto.VideoStatusInProgress
	case "completed", "complete", "succeeded", "success":
		return dto.VideoStatusCompleted
	case "failed", "failure", "cancelled", "canceled":
		return dto.VideoStatusFailed
	default:
		return dto.VideoStatusUnknown
	}
}

func numericProgress(raw json.RawMessage, status string) int {
	if len(raw) > 0 && string(raw) != "null" {
		var number float64
		if common.Unmarshal(raw, &number) == nil {
			return int(number)
		}
		var text string
		if common.Unmarshal(raw, &text) == nil {
			value, _ := strconv.Atoi(strings.TrimSuffix(strings.TrimSpace(text), "%"))
			return value
		}
	}
	switch status {
	case dto.VideoStatusQueued:
		return 20
	case dto.VideoStatusInProgress:
		return 50
	case dto.VideoStatusCompleted, dto.VideoStatusFailed:
		return 100
	default:
		return 0
	}
}

func toOpenAIVideo(task taskWire, publicID, modelName string) openAIVideoWire {
	if publicID == "" {
		publicID = task.ID
	}
	if modelName == "" {
		modelName = task.Model
	}
	createdAt := task.Created
	if createdAt == 0 {
		createdAt = task.CreatedAt
	}
	if createdAt == 0 {
		createdAt = time.Now().Unix()
	}
	status := openAIStatus(task.Status)
	result := openAIVideoWire{
		ID: publicID, TaskID: publicID, Object: "video", Model: modelName, Status: status,
		Progress: numericProgress(task.Progress, status), CreatedAt: createdAt,
		VideoURL: outputURL(task), LastFrameURL: task.LastFrameURL,
		Metadata: map[string]interface{}{"upstream_model": task.Model},
	}
	if status == dto.VideoStatusCompleted || status == dto.VideoStatusFailed {
		result.CompletedAt = time.Now().Unix()
	}
	if message := taskErrorMessage(task); message != "" {
		result.Error = &dto.OpenAIVideoError{Message: message}
	}
	if result.VideoURL != "" {
		result.Metadata["url"] = result.VideoURL
	}
	if task.LastFrameURL != "" {
		result.Metadata["last_frame_url"] = task.LastFrameURL
	}
	if task.Amount > 0 {
		result.Metadata["amount"] = task.Amount
	}
	if task.ActualDuration != nil {
		result.Metadata["actual_duration"] = task.ActualDuration
	}
	if task.TotalTokens > 0 {
		result.Metadata["total_tokens"] = task.TotalTokens
	} else if task.TotalTokensAlt > 0 {
		result.Metadata["total_tokens"] = task.TotalTokensAlt
	}
	return result
}

func isAsyncTaskRequest(c *gin.Context) bool {
	return c != nil && c.Request != nil && strings.HasPrefix(c.Request.URL.Path, "/async/tasks")
}

func asyncTaskResponse(task taskWire, publicID, modelName string) map[string]any {
	video := toOpenAIVideo(task, publicID, modelName)
	return map[string]any{
		"success": true,
		"message": "created",
		"data":    video,
	}
}

func (a *TaskAdaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (string, []byte, *dto.TaskError) {
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", nil, service.TaskErrorWrapper(err, "read_response_body_failed", http.StatusInternalServerError)
	}
	_ = resp.Body.Close()
	task, err := parseTask(body)
	if err != nil {
		return "", nil, service.TaskErrorWrapper(errors.Wrapf(err, "body: %s", body), "invalid_response", http.StatusInternalServerError)
	}
	if strings.TrimSpace(task.ID) == "" {
		message := taskErrorMessage(task)
		if message == "" {
			message = "task id is empty"
		}
		return "", nil, service.TaskErrorWrapper(errors.Wrapf(fmt.Errorf("%s", message), "body: %s", body), "invalid_response", http.StatusInternalServerError)
	}
	if isAsyncTaskRequest(c) {
		c.JSON(http.StatusOK, asyncTaskResponse(task, info.PublicTaskID, info.OriginModelName))
	} else {
		c.JSON(http.StatusCreated, toOpenAIVideo(task, info.PublicTaskID, info.OriginModelName))
	}
	return task.ID, body, nil
}

func (a *TaskAdaptor) FetchTask(baseURL, key string, body map[string]any, proxy string) (*http.Response, error) {
	taskID, ok := body["task_id"].(string)
	if !ok || strings.TrimSpace(taskID) == "" {
		return nil, fmt.Errorf("invalid task_id")
	}
	req, err := http.NewRequest(http.MethodGet, endpointURL(baseURL, taskID), nil)
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

func internalTaskStatus(status string) model.TaskStatus {
	switch openAIStatus(status) {
	case dto.VideoStatusQueued:
		return model.TaskStatusQueued
	case dto.VideoStatusInProgress:
		return model.TaskStatusInProgress
	case dto.VideoStatusCompleted:
		return model.TaskStatusSuccess
	case dto.VideoStatusFailed:
		return model.TaskStatusFailure
	default:
		return model.TaskStatusUnknown
	}
}

func (a *TaskAdaptor) ParseTaskResult(body []byte) (*relaycommon.TaskInfo, error) {
	task, err := parseTask(body)
	if err != nil {
		return nil, errors.Wrap(err, "unmarshal task result failed")
	}
	status := internalTaskStatus(task.Status)
	result := &relaycommon.TaskInfo{TaskID: task.ID, Status: string(status)}
	progress := numericProgress(task.Progress, openAIStatus(task.Status))
	if progress > 0 {
		result.Progress = fmt.Sprintf("%d%%", progress)
	}
	switch status {
	case model.TaskStatusSuccess:
		result.Url = outputURL(task)
	case model.TaskStatusFailure:
		result.Reason = taskErrorMessage(task)
		if result.Reason == "" {
			result.Reason = "task failed"
		}
	}
	if task.TotalTokens > 0 {
		result.TotalTokens = task.TotalTokens
	} else if task.TotalTokensAlt > 0 {
		result.TotalTokens = task.TotalTokensAlt
	}
	return result, nil
}

func storedOpenAIVideo(task *model.Task) (openAIVideoWire, error) {
	if len(task.Data) == 0 {
		return openAIVideoWire{}, fmt.Errorf("task response is empty")
	}
	upstream, err := parseTask(task.Data)
	if err != nil {
		return openAIVideoWire{}, errors.Wrap(err, "unmarshal Seedance task data failed")
	}
	video := toOpenAIVideo(upstream, task.TaskID, task.Properties.OriginModelName)
	video.Status = task.Status.ToVideoStatus()
	video.Progress = numericProgress(nil, video.Status)
	if task.Progress != "" {
		video.Progress, _ = strconv.Atoi(strings.TrimSuffix(task.Progress, "%"))
	}
	video.CreatedAt = task.CreatedAt
	if task.UpdatedAt > 0 && (video.Status == dto.VideoStatusCompleted || video.Status == dto.VideoStatusFailed) {
		video.CompletedAt = task.UpdatedAt
	}
	if task.FailReason != "" && video.Error == nil {
		video.Error = &dto.OpenAIVideoError{Message: task.FailReason}
	}
	return video, nil
}

func (a *TaskAdaptor) ConvertTaskResponse(task *model.Task) ([]byte, error) {
	video, err := storedOpenAIVideo(task)
	if err != nil {
		return nil, err
	}
	return common.Marshal(map[string]any{
		"success": true,
		"message": "created",
		"data":    video,
	})
}

func (a *TaskAdaptor) ConvertToOpenAIVideo(task *model.Task) ([]byte, error) {
	video, err := storedOpenAIVideo(task)
	if err != nil {
		return nil, err
	}
	return common.Marshal(video)
}

func (a *TaskAdaptor) GetModelList() []string {
	return []string{
		"seedance-2.0", "seedance-2.0-fast",
		"sd_2.0_discount", "sd_2.0_fast_discount", "sd_2.0_mini_discount",
		"sd_2.0_special", "sd_2.0_fast_special", "sd_2.0_mini_special",
	}
}

func (a *TaskAdaptor) GetChannelName() string { return "Seedance Model Center" }
