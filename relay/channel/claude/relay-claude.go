package claude

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/logger"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/service/relayconvert"
	"github.com/QuantumNous/new-api/setting/model_setting"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
)

func stopReasonClaude2OpenAI(reason string) string {
	return relayconvert.StopReasonClaudeToOpenAI(reason)
}

func maybeMarkClaudeRefusal(c *gin.Context, stopReason string) {
	if c == nil {
		return
	}
	if strings.EqualFold(stopReason, "refusal") {
		common.SetContextKey(c, constant.ContextKeyAdminRejectReason, "claude_stop_reason=refusal")
	}
}

func claudeRefusalResponseText(response *dto.ClaudeResponse) string {
	if response == nil {
		return ""
	}
	var text strings.Builder
	for _, content := range response.Content {
		if content.Text != nil {
			text.WriteString(*content.Text)
		}
		if content.Thinking != nil {
			text.WriteString(*content.Thinking)
		}
	}
	if text.Len() == 0 {
		text.WriteString(response.Completion)
	}
	if text.Len() == 0 && response.StopDetails != nil {
		text.WriteString(response.StopDetails.Explanation)
	}
	return text.String()
}

func maybeEstimateMissingClaudeRefusalUsage(c *gin.Context, info *relaycommon.RelayInfo, claudeInfo *ClaudeResponseInfo, responseText string) bool {
	if c == nil || info == nil || claudeInfo == nil ||
		common.GetContextKeyString(c, constant.ContextKeyAdminRejectReason) != "claude_stop_reason=refusal" ||
		strings.TrimSpace(responseText) == "" {
		return false
	}
	if claudeInfo.Usage == nil {
		claudeInfo.Usage = &dto.Usage{}
	}

	hasReportedInput := claudeInfo.Usage.PromptTokens > 0 ||
		claudeInfo.Usage.PromptTokensDetails.CachedTokens > 0 ||
		claudeInfo.Usage.PromptTokensDetails.CachedCreationTokens > 0 ||
		claudeInfo.Usage.ClaudeCacheCreation5mTokens > 0 ||
		claudeInfo.Usage.ClaudeCacheCreation1hTokens > 0
	if claudeInfo.Usage.CompletionTokens > 0 && hasReportedInput {
		return false
	}

	estimated := service.ResponseText2Usage(c, responseText, info.UpstreamModelName, info.GetEstimatePromptTokens())
	changed := false
	if claudeInfo.Usage.CompletionTokens == 0 && estimated.CompletionTokens > 0 {
		claudeInfo.Usage.CompletionTokens = estimated.CompletionTokens
		changed = true
	}
	if !hasReportedInput && estimated.PromptTokens > 0 {
		claudeInfo.Usage.PromptTokens = estimated.PromptTokens
		changed = true
	}
	if !changed {
		return false
	}

	claudeInfo.Usage.TotalTokens = claudeInfo.Usage.PromptTokens + claudeInfo.Usage.CompletionTokens
	claudeInfo.Usage.UsageSemantic = dto.BillingUsageSemanticAnthropic
	billingUsage := dto.NewClaudeMessagesBillingUsage(buildMessageDeltaPatchUsage(nil, claudeInfo))
	if billingUsage != nil {
		billingUsage.Estimated = true
		claudeInfo.Usage.BillingUsage = billingUsage
	}
	return true
}

func StreamResponseClaude2OpenAI(claudeResponse *dto.ClaudeResponse) *dto.ChatCompletionsStreamResponse {
	return relayconvert.StreamResponseClaude2OpenAI(claudeResponse)
}

func ResponseClaude2OpenAI(claudeResponse *dto.ClaudeResponse) *dto.OpenAITextResponse {
	return relayconvert.ResponseClaude2OpenAI(claudeResponse)
}

type ClaudeResponseInfo = relayconvert.ClaudeResponseInfo

func cacheCreationTokensForOpenAIUsage(usage *dto.Usage) int {
	if usage == nil {
		return 0
	}
	openAIUsage := relayconvert.UsageFromClaudeUsage(usage)
	if openAIUsage == nil {
		return 0
	}
	return openAIUsage.PromptTokens - usage.PromptTokens - usage.PromptTokensDetails.CachedTokens
}

func buildOpenAIStyleUsageFromClaudeUsage(usage *dto.Usage) dto.Usage {
	mapped := relayconvert.UsageFromClaudeUsage(usage)
	if mapped == nil {
		return dto.Usage{}
	}
	return *mapped
}

func buildMessageDeltaPatchUsage(claudeResponse *dto.ClaudeResponse, claudeInfo *ClaudeResponseInfo) *dto.ClaudeUsage {
	return relayconvert.BuildMessageDeltaPatchUsage(claudeResponse, claudeInfo)
}

func shouldSkipClaudeMessageDeltaUsagePatch(info *relaycommon.RelayInfo) bool {
	if model_setting.GetGlobalSettings().PassThroughRequestEnabled {
		return true
	}
	if info == nil {
		return false
	}
	return info.ChannelSetting.PassThroughBodyEnabled
}

func patchClaudeMessageDeltaUsageData(data string, usage *dto.ClaudeUsage) string {
	return relayconvert.PatchClaudeMessageDeltaUsageData(data, usage)
}

func patchClaudeUsageObjectForClient(raw map[string]json.RawMessage, adjusted *dto.ClaudeUsage) {
	if raw == nil || adjusted == nil {
		return
	}
	for key, value := range map[string]int{
		"input_tokens":                adjusted.InputTokens,
		"cache_read_input_tokens":     adjusted.CacheReadInputTokens,
		"cache_creation_input_tokens": adjusted.CacheCreationInputTokens,
		"output_tokens":               adjusted.OutputTokens,
	} {
		raw[key], _ = common.Marshal(value)
	}
	for key, value := range map[string]int{
		"claude_cache_creation_5_m_tokens": adjusted.ClaudeCacheCreation5mTokens,
		"claude_cache_creation_1_h_tokens": adjusted.ClaudeCacheCreation1hTokens,
	} {
		if _, exists := raw[key]; exists {
			raw[key], _ = common.Marshal(value)
		}
	}
	var creation map[string]json.RawMessage
	if common.Unmarshal(raw["cache_creation"], &creation) == nil && creation != nil {
		creation["ephemeral_5m_input_tokens"], _ = common.Marshal(adjusted.GetCacheCreation5mTokens())
		creation["ephemeral_1h_input_tokens"], _ = common.Marshal(adjusted.GetCacheCreation1hTokens())
		raw["cache_creation"], _ = common.Marshal(creation)
	}
	delete(raw, "billing_usage")
}

func patchClaudeResponseUsageForClient(data string, info *relaycommon.RelayInfo, usage *dto.ClaudeUsage, nestedMessage bool) string {
	adjusted := service.AdjustClaudeUsageForClient(info, usage)
	if adjusted == usage {
		return data
	}
	var envelope map[string]json.RawMessage
	if err := common.UnmarshalJsonStr(data, &envelope); err != nil {
		return data
	}
	target := envelope
	if nestedMessage {
		var message map[string]json.RawMessage
		if err := common.Unmarshal(envelope["message"], &message); err != nil || message == nil {
			return data
		}
		target = message
	}
	var rawUsage map[string]json.RawMessage
	if err := common.Unmarshal(target["usage"], &rawUsage); err != nil || rawUsage == nil {
		return data
	}
	patchClaudeUsageObjectForClient(rawUsage, adjusted)
	target["usage"], _ = common.Marshal(rawUsage)
	if nestedMessage {
		envelope["message"], _ = common.Marshal(target)
	}
	encoded, err := common.Marshal(envelope)
	if err != nil {
		return data
	}
	return string(encoded)
}

func FormatClaudeResponseInfo(claudeResponse *dto.ClaudeResponse, oaiResponse *dto.ChatCompletionsStreamResponse, claudeInfo *ClaudeResponseInfo) bool {
	return relayconvert.FormatClaudeResponseInfo(claudeResponse, oaiResponse, claudeInfo)
}

// Audit results and hosted-tool execution counters may be nested. Inspect
// only while the race is private, not on every published output chunk.
func claudeFrameHasProtectedSignal(value any) bool {
	switch node := value.(type) {
	case map[string]any:
		for key, child := range node {
			switch key {
			case "safeguards", "safeguard_results", "prompt_filter_results", "content_filter_results", "safety_ratings":
				return true
			case "server_tool_use":
				if child == nil {
					continue
				}
				counters, ok := child.(map[string]any)
				if !ok {
					return true
				}
				for _, n := range counters {
					if count, ok := n.(float64); !ok || count != 0 {
						return true
					}
				}
			}
			if claudeFrameHasProtectedSignal(child) {
				return true
			}
		}
	case []any:
		for _, child := range node {
			if claudeFrameHasProtectedSignal(child) {
				return true
			}
		}
	}
	return false
}

func claudeFrameProtectsReplay(event *dto.ClaudeResponse, data string) bool {
	var raw any
	if common.UnmarshalJsonStr(data, &raw) != nil || claudeFrameHasProtectedSignal(raw) {
		return true
	}
	for _, block := range []*dto.ClaudeMediaMessage{event.ContentBlock, event.Delta} {
		if block != nil && (block.Type == "server_tool_use" || block.Type == "tool_use") && (block.Id == "" || block.Name == "") {
			return true
		}
	}
	switch event.Type {
	case "content_block_start":
		return event.ContentBlock == nil || event.ContentBlock.Type == ""
	case "content_block_delta":
		return event.Delta == nil || event.Delta.Type == ""
	case "message_start", "content_block_stop", "message_delta", "message_stop", "ping", "error":
		return false
	default:
		// An unrecognized frame cannot prove there was no useful output.
		return true
	}
}

func HandleStreamResponseData(c *gin.Context, info *relaycommon.RelayInfo, claudeInfo *ClaudeResponseInfo, data string) *types.NewAPIError {
	native := info.RelayFormat == types.RelayFormatClaude && info.ChannelType == constant.ChannelTypeAnthropic
	racing := info.ClaimStreamOutput != nil
	private := !c.Writer.Written()
	var claudeResponse dto.ClaudeResponse
	err := common.UnmarshalJsonStr(data, &claudeResponse)
	if err != nil {
		common.SysLog("error unmarshalling stream response: " + err.Error())
		if native {
			return types.NewErrorWithStatusCode(err, types.ErrorCodeBadResponseBody, http.StatusBadGateway, types.ErrOptionWithSkipRetry())
		}
		return types.NewError(err, types.ErrorCodeBadResponseBody)
	}
	protectsReplay := native && private && (racing || info.ClaudeEmptyStreamReplaySafe) && claudeFrameProtectsReplay(&claudeResponse, data)
	if claudeError := claudeResponse.GetClaudeError(); claudeError != nil && claudeError.Type != "" {
		status := types.ClaudeErrorStatus(claudeError.Type)
		if native && (protectsReplay || !private || c.GetBool("upstream_moderation_committed")) {
			if protectsReplay {
				c.Set("upstream_moderation_committed", true)
				if racing && !info.ClaimStreamOutput() {
					return types.NewError(relaycommon.ErrStreamRaceLost, types.ErrorCodeBadResponse, types.ErrOptionWithSkipRetry())
				}
				helper.ClaudeChunkData(c, claudeResponse, data)
			}
			return types.WithClaudeError(*claudeError, status, types.ErrOptionWithSkipRetry())
		}
		return types.WithClaudeError(*claudeError, status)
	}
	if native && (racing || info.ClaudeEmptyStreamReplaySafe) && claudeResponse.Type == "error" && !protectsReplay {
		return types.NewErrorWithStatusCode(errors.New("invalid upstream stream error frame"), types.ErrorCodeBadResponseBody, http.StatusBadGateway, types.ErrOptionWithSkipRetry())
	}
	if claudeResponse.StopReason != "" {
		maybeMarkClaudeRefusal(c, claudeResponse.StopReason)
	}
	if claudeResponse.Delta != nil && claudeResponse.Delta.StopReason != nil {
		maybeMarkClaudeRefusal(c, *claudeResponse.Delta.StopReason)
	}
	stopDetails := claudeResponse.StopDetails
	if stopDetails == nil && claudeResponse.Delta != nil {
		stopDetails = claudeResponse.Delta.StopDetails
	}
	if stopDetails != nil && strings.TrimSpace(stopDetails.Explanation) != "" {
		claudeInfo.ResponseText.WriteString(stopDetails.Explanation)
	}
	if private && (racing || info.ClaudeEmptyStreamReplaySafe) {
		switch {
		case protectsReplay || claudeStreamHasOutput(&claudeResponse) || claudeStreamHasHiddenThinkingProgress(&claudeResponse, data):
			if protectsReplay {
				c.Set("upstream_moderation_committed", true)
			}
			if racing && !info.ClaimStreamOutput() {
				return types.NewError(relaycommon.ErrStreamRaceLost, types.ErrorCodeBadResponse, types.ErrOptionWithSkipRetry())
			}
		case claudeResponse.Type == "message_stop":
			if info.ClaudeEmptyStreamReplaySafe && !c.GetBool("upstream_moderation_committed") {
				// Only a local explicit terminal frame can get this cause. EOF,
				// provider errors and already-published SSE keep their own paths.
				return types.NewErrorWithStatusCode(relaycommon.ErrClaudeStreamEndedWithoutOutput, types.ErrorCodeBadResponseBody, http.StatusBadGateway)
			}
			return types.NewErrorWithStatusCode(errors.New(relaycommon.ErrClaudeStreamEndedWithoutOutput.Error()), types.ErrorCodeBadResponseBody, http.StatusBadGateway, types.ErrOptionWithSkipRetry())
		}
	}
	if info.RelayFormat == types.RelayFormatClaude {
		FormatClaudeResponseInfo(&claudeResponse, nil, claudeInfo)

		if claudeResponse.Type == "message_start" {
			// message_start, 获取usage
			if claudeResponse.Message != nil {
				info.UpstreamModelName = claudeResponse.Message.Model
			}
		} else if claudeResponse.Type == "message_delta" {
			// 确保 message_delta 的 usage 包含完整的 input_tokens 和 cache 相关字段
			// 解决 AWS Bedrock 等上游返回的 message_delta 缺少这些字段的问题
			if !shouldSkipClaudeMessageDeltaUsagePatch(info) {
				data = patchClaudeMessageDeltaUsageData(data, buildMessageDeltaPatchUsage(&claudeResponse, claudeInfo))
			}
		}
		// Settlement keeps the original accumulated usage. Only the public event
		// receives the configured deduction, so retries and final billing cannot
		// apply it twice.
		switch claudeResponse.Type {
		case "message_start":
			if claudeResponse.Message != nil && claudeResponse.Message.Usage != nil {
				data = patchClaudeResponseUsageForClient(data, info, claudeResponse.Message.Usage, true)
			}
		case "message_delta":
			data = patchClaudeResponseUsageForClient(data, info, buildMessageDeltaPatchUsage(&claudeResponse, claudeInfo), false)
		default:
			if claudeResponse.Usage != nil {
				data = patchClaudeResponseUsageForClient(data, info, claudeResponse.Usage, false)
			}
		}
		// Native consumers may themselves convert Claude to content-only OpenAI.
		// Insert the unmodified upstream explanation before message_delta, without
		// changing the original refusal event/usage or duplicating it in estimates.
		if claudeInfo.MessageStarted && claudeInfo.OpenContentBlocks == 0 {
			if notice := claudeInfo.TakeRefusalNotice(&claudeResponse); notice != "" {
				index := common.GetPointer(claudeInfo.NextContentBlockIndex)
				for _, event := range []dto.ClaudeResponse{
					{Type: "content_block_start", Index: index, ContentBlock: &dto.ClaudeMediaMessage{Type: "text", Text: common.GetPointer("")}},
					{Type: "content_block_delta", Index: index, Delta: &dto.ClaudeMediaMessage{Type: "text_delta", Text: common.GetPointer(notice)}},
					{Type: "content_block_stop", Index: index},
				} {
					encoded, marshalErr := common.Marshal(event)
					if marshalErr != nil {
						return types.NewError(marshalErr, types.ErrorCodeBadResponseBody)
					}
					helper.ClaudeChunkData(c, event, string(encoded))
				}
			}
		}
		helper.ClaudeChunkData(c, claudeResponse, data)
	} else if info.RelayFormat == types.RelayFormatOpenAI {
		response := StreamResponseClaude2OpenAI(&claudeResponse)

		if !FormatClaudeResponseInfo(&claudeResponse, response, claudeInfo) {
			return nil
		}

		err = helper.ObjectData(c, response)
		if err != nil {
			logger.LogError(c, "send_stream_response_failed: "+err.Error())
		}
	}
	return nil
}

// Usage, structural frames and signatures alone cannot win. Preserve refusal
// and new block/delta types conservatively rather than silently replaying them.
func claudeStreamHasOutput(event *dto.ClaudeResponse) bool {
	if event.Completion != "" || len(event.Content) != 0 || event.StopDetails != nil || meaningfulClaudeStop(event.StopReason) {
		return true
	}
	for _, part := range []*dto.ClaudeMediaMessage{event.Delta, event.Message} {
		if part != nil && (part.StopDetails != nil || part.StopReason != nil && meaningfulClaudeStop(*part.StopReason)) {
			return true
		}
	}
	if event.Message != nil {
		switch content := event.Message.Content.(type) {
		case nil:
		case []any:
			if len(content) != 0 {
				return true
			}
		case string:
			if content != "" {
				return true
			}
		default:
			return true
		}
	}
	var block *dto.ClaudeMediaMessage
	switch event.Type {
	case "content_block_start":
		block = event.ContentBlock
	case "content_block_delta":
		block = event.Delta
	default:
		return false
	}
	if block == nil {
		return false
	}
	switch block.Type {
	case "text", "text_delta":
		return block.GetText() != ""
	case "thinking", "thinking_delta":
		return block.Thinking != nil && *block.Thinking != ""
	case "input_json_delta":
		return block.PartialJson != nil && *block.PartialJson != ""
	case "tool_use", "server_tool_use":
		return block.Id != "" && block.Name != ""
	case "signature_delta":
		return false
	default:
		return block.Type != ""
	}
}

func claudeStreamHasHiddenThinkingProgress(event *dto.ClaudeResponse, data string) bool {
	if event.Type != "content_block_delta" || event.Delta == nil || event.Delta.Type != "thinking_delta" {
		return false
	}
	// A provider can report token progress with thinking.display=omitted.
	// Do not mistake that for a run which generated nothing.
	var progress struct {
		Delta struct {
			EstimatedTokens int64 `json:"estimated_tokens"`
		} `json:"delta"`
	}
	return common.UnmarshalJsonStr(data, &progress) == nil && progress.Delta.EstimatedTokens > 0
}

func meaningfulClaudeStop(reason string) bool {
	switch strings.ToLower(strings.TrimSpace(reason)) {
	case "", "end_turn":
		return false
	default:
		// Preserve explicit length/stop boundaries, refusal, tools and new reasons.
		return true
	}
}

func HandleStreamFinalResponse(c *gin.Context, info *relaycommon.RelayInfo, claudeInfo *ClaudeResponseInfo) {
	maybeEstimateMissingClaudeRefusalUsage(c, info, claudeInfo, claudeInfo.ResponseText.String())
	if claudeInfo.Usage.PromptTokens == 0 {
		//上游出错
	}
	if claudeInfo.Usage.CompletionTokens == 0 || !claudeInfo.Done {
		if common.DebugEnabled {
			common.SysLog("claude response usage is not complete, maybe upstream error")
		}
		// 只补缺失字段，不整份覆盖——保留 message_start 已拿到的 cache 字段
		fallback := service.ResponseText2Usage(c, claudeInfo.ResponseText.String(), info.UpstreamModelName, info.GetEstimatePromptTokens())
		if claudeInfo.Usage.CompletionTokens == 0 ||
			(!claudeInfo.Done && fallback.CompletionTokens > claudeInfo.Usage.CompletionTokens) {
			claudeInfo.Usage.CompletionTokens = fallback.CompletionTokens
		}
		hasReportedInput := claudeInfo.Usage.PromptTokensDetails.CachedTokens > 0 ||
			claudeInfo.Usage.PromptTokensDetails.CachedCreationTokens > 0 ||
			claudeInfo.Usage.ClaudeCacheCreation5mTokens > 0 ||
			claudeInfo.Usage.ClaudeCacheCreation1hTokens > 0
		if claudeInfo.Usage.PromptTokens == 0 && !hasReportedInput {
			claudeInfo.Usage.PromptTokens = fallback.PromptTokens
		}
		claudeInfo.Usage.TotalTokens = claudeInfo.Usage.PromptTokens + claudeInfo.Usage.CompletionTokens
	}
	if claudeInfo.Usage != nil {
		claudeInfo.Usage.UsageSemantic = "anthropic"
	}
	if claudeInfo.Usage != nil && claudeInfo.Usage.BillingUsage == nil {
		claudeInfo.Usage.BillingUsage = dto.NewClaudeMessagesBillingUsage(buildMessageDeltaPatchUsage(nil, claudeInfo))
	}

	if info.RelayFormat == types.RelayFormatClaude {
		//
	} else if info.RelayFormat == types.RelayFormatOpenAI {
		if info.ShouldIncludeUsage {
			publicUsage := service.AdjustUsageForClient(info, claudeInfo.Usage)
			openAIUsage := buildOpenAIStyleUsageFromClaudeUsage(publicUsage)
			response := helper.GenerateFinalUsageResponse(claudeInfo.ResponseId, claudeInfo.Created, info.UpstreamModelName, openAIUsage)
			err := helper.ObjectData(c, response)
			if err != nil {
				common.SysLog("send final response failed: " + err.Error())
			}
		}
		helper.Done(c)
	}
}

func ClaudeStreamHandler(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (*dto.Usage, *types.NewAPIError) {
	claudeInfo := &ClaudeResponseInfo{
		ResponseId:   helper.GetResponseID(c),
		Created:      common.GetTimestamp(),
		Model:        info.UpstreamModelName,
		ResponseText: strings.Builder{},
		Usage:        &dto.Usage{},
	}
	var err *types.NewAPIError
	helper.StreamScannerHandler(c, resp, info, func(data string, sr *helper.StreamResult) {
		err = HandleStreamResponseData(c, info, claudeInfo, data)
		if err != nil {
			sr.Stop(err)
		}
	})
	if err != nil {
		return nil, err
	}
	// 流异常结束（timeout/client_gone/scanner_error 等）且未收到任何上游数据时，
	// 按失败返回错误、不进入计费兜底（避免用本地估算 prompt tokens 对 0 数据请求扣费），
	// 让上层 shouldRetry/换渠道逻辑接管。
	if emptyErr := helper.StreamAbnormalEmptyError(c, info); emptyErr != nil {
		return nil, emptyErr
	}

	HandleStreamFinalResponse(c, info, claudeInfo)
	return claudeInfo.Usage, nil
}

func HandleClaudeResponseData(c *gin.Context, info *relaycommon.RelayInfo, claudeInfo *ClaudeResponseInfo, httpResp *http.Response, data []byte) *types.NewAPIError {
	var claudeResponse dto.ClaudeResponse
	err := common.Unmarshal(data, &claudeResponse)
	if err != nil {
		return types.NewError(err, types.ErrorCodeBadResponseBody)
	}
	if claudeError := claudeResponse.GetClaudeError(); claudeError != nil && claudeError.Type != "" {
		status := types.ClaudeErrorStatus(claudeError.Type)
		if httpResp != nil && httpResp.StatusCode >= 400 && httpResp.StatusCode <= 599 {
			status = httpResp.StatusCode
		}
		return types.WithClaudeError(*claudeError, status)
	}
	maybeMarkClaudeRefusal(c, claudeResponse.StopReason)
	if claudeInfo.Usage == nil {
		claudeInfo.Usage = &dto.Usage{}
	}
	if claudeResponse.Usage != nil {
		claudeInfo.Usage.PromptTokens = claudeResponse.Usage.InputTokens
		claudeInfo.Usage.CompletionTokens = claudeResponse.Usage.OutputTokens
		claudeInfo.Usage.TotalTokens = claudeResponse.Usage.InputTokens + claudeResponse.Usage.OutputTokens
		claudeInfo.Usage.UsageSemantic = "anthropic"
		claudeInfo.Usage.BillingUsage = dto.NewClaudeMessagesBillingUsage(claudeResponse.Usage)
		claudeInfo.Usage.PromptTokensDetails.CachedTokens = claudeResponse.Usage.CacheReadInputTokens
		claudeInfo.Usage.PromptTokensDetails.CachedCreationTokens = claudeResponse.Usage.CacheCreationInputTokens
		claudeInfo.Usage.ClaudeCacheCreation5mTokens = claudeResponse.Usage.GetCacheCreation5mTokens()
		claudeInfo.Usage.ClaudeCacheCreation1hTokens = claudeResponse.Usage.GetCacheCreation1hTokens()
	}
	maybeEstimateMissingClaudeRefusalUsage(c, info, claudeInfo, claudeRefusalResponseText(&claudeResponse))
	var responseData []byte
	switch info.RelayFormat {
	case types.RelayFormatOpenAI:
		openaiResponse := ResponseClaude2OpenAI(&claudeResponse)
		publicUsage := service.AdjustUsageForClient(info, claudeInfo.Usage)
		openaiResponse.Usage = buildOpenAIStyleUsageFromClaudeUsage(publicUsage)
		responseData, err = common.Marshal(openaiResponse)
		if err != nil {
			return types.NewError(err, types.ErrorCodeBadResponseBody)
		}
	case types.RelayFormatClaude:
		responseData = []byte(patchClaudeResponseUsageForClient(string(data), info, claudeResponse.Usage, false))
	}

	if claudeResponse.Usage != nil && claudeResponse.Usage.ServerToolUse != nil && claudeResponse.Usage.ServerToolUse.WebSearchRequests > 0 {
		c.Set("claude_web_search_requests", claudeResponse.Usage.ServerToolUse.WebSearchRequests)
	}

	service.IOCopyBytesGracefully(c, httpResp, responseData)
	return nil
}

func ClaudeHandler(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (*dto.Usage, *types.NewAPIError) {
	defer service.CloseResponseBodyGracefully(resp)

	claudeInfo := &ClaudeResponseInfo{
		ResponseId:   helper.GetResponseID(c),
		Created:      common.GetTimestamp(),
		Model:        info.UpstreamModelName,
		ResponseText: strings.Builder{},
		Usage:        &dto.Usage{},
	}
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, types.NewError(err, types.ErrorCodeBadResponseBody)
	}
	logger.LogDebug(c, "responseBody: %s", responseBody)
	handleErr := HandleClaudeResponseData(c, info, claudeInfo, resp, responseBody)
	if handleErr != nil {
		return nil, handleErr
	}
	return claudeInfo.Usage, nil
}
