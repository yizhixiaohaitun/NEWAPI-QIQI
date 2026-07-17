package channel_purity

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	baseservice "github.com/QuantumNous/new-api/service"
)

const maxResponseBytes = 1 << 20

type Outcome struct {
	Status     string
	Conclusion string
	Risk       string
	Coverage   int
	Summary    string
	ErrorClass string
	Result     *model.ChannelPurityResult
}

type chatUsage struct {
	PromptTokens     *int `json:"prompt_tokens"`
	CompletionTokens *int `json:"completion_tokens"`
	TotalTokens      *int `json:"total_tokens"`
}

type chatResponse struct {
	ID      string     `json:"id"`
	Object  string     `json:"object"`
	Model   string     `json:"model"`
	Choices []any      `json:"choices"`
	Usage   *chatUsage `json:"usage"`
}

func RunQuickProbe(ctx context.Context, channel *model.Channel, requestedModel string) Outcome {
	started := time.Now()
	outcome := Outcome{
		Status:     model.ChannelPurityStatusCompleted,
		Conclusion: model.ChannelPurityConclusionUnknown,
		Risk:       model.ChannelPurityRiskUnknown,
		Summary:    "Quick probe evidence is insufficient; conclusion unknown",
	}
	result := &model.ChannelPurityResult{ChannelID: channel.Id, CreatedAt: time.Now().Unix()}
	outcome.Result = result
	evidence := dto.ChannelPurityEvidence{}
	defer func() {
		result.LatencyMS = time.Since(started).Milliseconds()
		evidence.HTTPStatus = result.HTTPStatus
		evidence.DeclaredModel = result.DeclaredModel
		evidence.HasModelField = result.HasModelField
		evidence.HasUsage = result.HasUsage
		evidence.HasOutput = result.HasOutput
		evidence.Usage = dto.ChannelPurityUsage{
			PromptTokens: result.PromptTokens, CompletionTokens: result.CompletionTokens, TotalTokens: result.TotalTokens,
		}
		encoded, err := common.Marshal(evidence)
		if err == nil {
			result.EvidenceJSON = string(encoded)
		} else {
			result.EvidenceJSON = "{}"
		}
	}()

	if !supportsOpenAIChat(channel.Type) {
		outcome.ErrorClass = "unsupported_channel_type"
		outcome.Summary = "Quick probe does not support this channel protocol; conclusion unknown"
		return outcome
	}

	endpoint, err := buildChatCompletionsEndpoint(channel.GetBaseURL())
	if err != nil {
		outcome.ErrorClass = "invalid_base_url"
		outcome.Summary = "Channel base URL is invalid; conclusion unknown"
		return outcome
	}
	upstreamModel := mapModel(requestedModel, channel.GetModelMapping())
	evidence.MappedModel = upstreamModel
	body, err := common.Marshal(map[string]any{
		"model": upstreamModel,
		"messages": []map[string]string{{
			"role": "user", "content": "Reply with exactly: OK",
		}},
		"temperature": 0,
		"max_tokens":  8,
		"stream":      false,
	})
	if err != nil {
		outcome.ErrorClass = "request_build_error"
		return outcome
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		outcome.ErrorClass = "request_build_error"
		return outcome
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Content-Type", "application/json")
	key, _, keyErr := channel.GetNextEnabledKey()
	if keyErr != nil || strings.TrimSpace(key) == "" {
		outcome.ErrorClass = "credential_unavailable"
		outcome.Summary = "Channel credential is unavailable; probe was not sent"
		return outcome
	}
	request.Header.Set("Authorization", "Bearer "+key)

	response, err := newProbeHTTPClient().Do(request)
	if err != nil {
		outcome.ErrorClass = classifyTransportError(err)
		outcome.Summary = "Probe request failed; conclusion unknown"
		return outcome
	}
	defer response.Body.Close()
	result.HTTPStatus = response.StatusCode
	evidence.ContentType = response.Header.Get("Content-Type")
	data, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes+1))
	if err != nil {
		outcome.ErrorClass = "response_read_error"
		return outcome
	}
	if len(data) > maxResponseBytes {
		outcome.ErrorClass = "response_too_large"
		outcome.Summary = "Upstream response exceeded the probe limit; conclusion unknown"
		return outcome
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		outcome.ErrorClass = classifyHTTPError(response.StatusCode)
		outcome.Summary = "Upstream returned an operational error; no purity risk was inferred"
		return outcome
	}

	var parsed chatResponse
	if err := common.Unmarshal(data, &parsed); err != nil {
		outcome.Conclusion = model.ChannelPurityConclusionRisk
		outcome.Risk = model.ChannelPurityRiskMedium
		outcome.Coverage = 25
		outcome.ErrorClass = "invalid_json"
		outcome.Summary = "Successful response is not valid JSON; protocol risk detected"
		return outcome
	}
	result.DeclaredModel = parsed.Model
	result.HasModelField = strings.TrimSpace(parsed.Model) != ""
	result.HasOutput = len(parsed.Choices) > 0
	result.HasUsage = hasUsageFields(parsed.Usage)
	result.PromptTokens = usageValue(parsed.Usage, func(usage *chatUsage) *int { return usage.PromptTokens })
	result.CompletionTokens = usageValue(parsed.Usage, func(usage *chatUsage) *int { return usage.CompletionTokens })
	result.TotalTokens = usageValue(parsed.Usage, func(usage *chatUsage) *int { return usage.TotalTokens })
	result.ProtocolValid = result.HasOutput && !invalidUsage(parsed.Usage)
	evidence.Object = parsed.Object
	evidence.HasChoices = len(parsed.Choices) > 0
	if parsed.ID != "" {
		prefixLength := len(parsed.ID)
		if prefixLength > 12 {
			prefixLength = 12
		}
		evidence.ResponseIDPrefix = parsed.ID[:prefixLength]
	}

	outcome.Coverage = 100
	if !result.HasOutput {
		outcome.Conclusion = model.ChannelPurityConclusionRisk
		outcome.Risk = model.ChannelPurityRiskMedium
		outcome.Summary = "Successful response lacks choices output; protocol risk detected"
		return outcome
	}
	if invalidUsage(parsed.Usage) {
		outcome.Conclusion = model.ChannelPurityConclusionRisk
		outcome.Risk = model.ChannelPurityRiskMedium
		outcome.ErrorClass = "invalid_usage"
		outcome.Summary = "Successful response contains inconsistent token usage; protocol risk detected"
		return outcome
	}
	if result.HasModelField && !sameModelFamily(upstreamModel, parsed.Model) {
		evidence.Warnings = append(evidence.Warnings, "declared_model_differs_from_mapped_request")
		outcome.Coverage = 75
		outcome.Summary = "Declared model differs from the mapped request; this is weak evidence only"
		return outcome
	}
	if !result.HasModelField || !result.HasUsage {
		outcome.Summary = "Some optional structural evidence is missing; conclusion unknown"
		return outcome
	}
	outcome.Conclusion = model.ChannelPurityConclusionNoObviousRisk
	outcome.Risk = model.ChannelPurityRiskLow
	outcome.Summary = "Quick probe found no obvious structural risk; model identity is not proven"
	return outcome
}

func buildChatCompletionsEndpoint(baseURL string) (string, error) {
	base, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil || base.Scheme == "" || base.Host == "" || base.Opaque != "" || base.User != nil {
		return "", errors.New("invalid base URL")
	}
	if base.Scheme != "http" && base.Scheme != "https" {
		return "", errors.New("unsupported base URL scheme")
	}
	base.Fragment = ""
	path := strings.TrimRight(base.Path, "/")
	if !strings.HasSuffix(strings.ToLower(path), "/v1") {
		path += "/v1"
	}
	base.Path = path + "/chat/completions"
	base.RawPath = ""
	return base.String(), nil
}

func newProbeHTTPClient() *http.Client {
	transport := http.RoundTripper(http.DefaultTransport)
	if baseClient := baseservice.GetHttpClient(); baseClient != nil && baseClient.Transport != nil {
		transport = baseClient.Transport
	}
	return &http.Client{
		Transport: transport,
		Timeout:   45 * time.Second,
		CheckRedirect: func(next *http.Request, via []*http.Request) error {
			if len(via) == 0 {
				return nil
			}
			origin := via[0].URL
			if !strings.EqualFold(next.URL.Scheme, origin.Scheme) ||
				!strings.EqualFold(next.URL.Hostname(), origin.Hostname()) ||
				effectivePort(next.URL) != effectivePort(origin) {
				return http.ErrUseLastResponse
			}
			if len(via) >= 3 {
				return errors.New("too many redirects")
			}
			return nil
		},
	}
}

func supportsOpenAIChat(channelType int) bool {
	switch channelType {
	case constant.ChannelTypeOpenAI, constant.ChannelTypeOpenAIMax, constant.ChannelTypeOhMyGPT,
		constant.ChannelTypeCustom, constant.ChannelTypeAIProxy, constant.ChannelTypeAPI2GPT,
		constant.ChannelTypeAIGC2D, constant.ChannelTypeOpenRouter, constant.ChannelTypeFastGPT,
		constant.ChannelTypeMoonshot, constant.ChannelTypePerplexity, constant.ChannelTypeSiliconFlow,
		constant.ChannelTypeMistral, constant.ChannelTypeDeepSeek, constant.ChannelTypeXinference,
		constant.ChannelTypeXai:
		return true
	default:
		return false
	}
}

func mapModel(name, mappingJSON string) string {
	if strings.TrimSpace(mappingJSON) == "" {
		return name
	}
	mapping := map[string]string{}
	if common.UnmarshalJsonStr(mappingJSON, &mapping) != nil {
		return name
	}

	current := name
	visited := map[string]struct{}{current: {}}
	for range 16 {
		next := strings.TrimSpace(mapping[current])
		if next == "" || next == current {
			break
		}
		if _, exists := visited[next]; exists {
			break
		}
		visited[next] = struct{}{}
		current = next
	}
	return current
}

func hasUsageFields(usage *chatUsage) bool {
	return usage != nil && (usage.PromptTokens != nil || usage.CompletionTokens != nil || usage.TotalTokens != nil)
}

func usageValue(usage *chatUsage, selectValue func(*chatUsage) *int) int {
	if usage == nil {
		return 0
	}
	value := selectValue(usage)
	if value == nil {
		return 0
	}
	return *value
}

func invalidUsage(usage *chatUsage) bool {
	if usage == nil {
		return false
	}
	for _, value := range []*int{usage.PromptTokens, usage.CompletionTokens, usage.TotalTokens} {
		if value != nil && *value < 0 {
			return true
		}
	}
	return usage.PromptTokens != nil && usage.CompletionTokens != nil && usage.TotalTokens != nil &&
		*usage.TotalTokens < *usage.PromptTokens+*usage.CompletionTokens
}

func sameModelFamily(expected, actual string) bool {
	normalize := func(value string) string {
		value = strings.ToLower(strings.TrimSpace(value))
		return strings.TrimPrefix(value, "models/")
	}
	expected = normalize(expected)
	actual = normalize(actual)
	return expected == actual || strings.HasPrefix(actual, expected+"-") || strings.HasPrefix(expected, actual+"-")
}

func effectivePort(target *url.URL) string {
	if target.Port() != "" {
		return target.Port()
	}
	if target.Scheme == "https" {
		return "443"
	}
	return "80"
}

func classifyTransportError(err error) string {
	if errors.Is(err, context.DeadlineExceeded) {
		return "timeout"
	}
	var netErr net.Error
	if errors.As(err, &netErr) {
		if netErr.Timeout() {
			return "timeout"
		}
		return "network_error"
	}
	return "transport_error"
}

func classifyHTTPError(status int) string {
	switch status {
	case http.StatusUnauthorized, http.StatusForbidden:
		return "authentication_error"
	case http.StatusTooManyRequests:
		return "rate_limit"
	case http.StatusNotFound:
		return "endpoint_or_model_not_found"
	default:
		if status >= http.StatusInternalServerError {
			return "upstream_server_error"
		}
		if status >= http.StatusMultipleChoices && status < http.StatusBadRequest {
			return "redirect_blocked"
		}
		return "upstream_api_error"
	}
}
