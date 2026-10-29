package openai

import (
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/logger"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
)

func OaiResponsesHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*dto.Usage, *types.NewAPIError) {
	defer service.CloseResponseBodyGracefully(resp)

	// read response body
	var responsesResponse dto.OpenAIResponsesResponse
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeReadResponseBodyFailed, http.StatusInternalServerError)
	}
	err = common.Unmarshal(responseBody, &responsesResponse)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}
	if oaiError := responsesResponse.GetOpenAIError(); oaiError != nil && oaiError.Type != "" {
		return nil, types.WithOpenAIError(*oaiError, resp.StatusCode)
	}

	if responsesResponse.HasImageGenerationCall() {
		c.Set("image_generation_call", true)
		c.Set("image_generation_call_quality", responsesResponse.GetQuality())
		c.Set("image_generation_call_size", responsesResponse.GetSize())
	}
	recordResponsesStateChannelAffinity(c, info, service.CollectOpenAIResponsesAffinityAliases(&responsesResponse))

	// compute usage before writing so the public response can use the same
	// deduction semantics while settlement retains this original value.
	usage := normalizedResponsesUsage(responsesResponse.Usage)
	responseBody = []byte(patchResponsesUsageForClient(string(responseBody), info, &usage, false))
	service.IOCopyBytesGracefully(c, resp, responseBody)
	if info == nil || info.ResponsesUsageInfo == nil || info.ResponsesUsageInfo.BuiltInTools == nil {
		return &usage, nil
	}
	// 解析 Tools 用量
	for _, tool := range responsesResponse.Tools {
		buildToolinfo, ok := info.ResponsesUsageInfo.BuiltInTools[common.Interface2String(tool["type"])]
		if !ok || buildToolinfo == nil {
			logger.LogError(c, fmt.Sprintf("BuiltInTools not found for tool type: %v", tool["type"]))
			continue
		}
		buildToolinfo.CallCount++
	}
	return &usage, nil
}

func OaiResponsesStreamHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*dto.Usage, *types.NewAPIError) {
	if resp == nil || resp.Body == nil {
		logger.LogError(c, "invalid response or response body")
		return nil, types.NewError(fmt.Errorf("invalid response"), types.ErrorCodeBadResponse)
	}

	defer service.CloseResponseBodyGracefully(resp)

	var usage = &dto.Usage{}
	var responseTextBuilder strings.Builder
	var streamErr *types.NewAPIError
	retryEnabled := operation_setting.IsResponsesStreamErrorRetryEnabled()
	var retryWriter *upstreamStreamRetryWriter
	if retryEnabled {
		originalWriter := c.Writer
		retryWriter = newUpstreamStreamRetryWriter(originalWriter)
		c.Writer = retryWriter
		defer func() {
			c.Writer = originalWriter
		}()
	}
	var affinityAliases []string
	addAffinityAliases := func(values ...string) {
		affinityAliases = append(affinityAliases, values...)
	}
	addResponseAffinityAliases := func(response *dto.OpenAIResponsesResponse) {
		addAffinityAliases(service.CollectOpenAIResponsesAffinityAliases(response)...)
	}

	helper.StreamScannerHandler(c, resp, info, func(data string, sr *helper.StreamResult) {

		// 检查当前数据是否包含 completed 状态和 usage 信息
		var streamResponse dto.ResponsesStreamResponse
		if err := common.UnmarshalJsonStr(data, &streamResponse); err != nil {
			logger.LogError(c, "failed to unmarshal stream response: "+err.Error())
			sr.Error(err)
			return
		}
		upstreamStreamErr := responsesStreamEventError(&streamResponse)
		if upstreamStreamErr != nil {
			if !retryEnabled {
				recordResponsesStreamRetryRecommendation(c)
				sendResponsesStreamData(c, info, streamResponse, data)
				return
			}
			if retryWriter != nil && !retryWriter.committed {
				streamErr = upstreamStreamErr
				markResponsesStreamErrorBeforeCommit(c, info)
				sr.Stop(streamErr)
				return
			}
			sr.Error(upstreamStreamErr)
			sendResponsesStreamData(c, info, streamResponse, data)
			return
		}

		if retryWriter != nil && !retryWriter.committed && responsesStreamEventCommitsOutput(&streamResponse) {
			// Commit time is the real first byte; keep the scanner's received count.
			received := info.ReceivedResponseCount
			info.ResetFirstResponseTime()
			info.ReceivedResponseCount = received
			info.SetFirstResponseTime()
			if err := retryWriter.Commit(); err != nil {
				streamErr = types.NewOpenAIError(err, types.ErrorCodeBadResponse, http.StatusInternalServerError)
				sr.Stop(streamErr)
				return
			}
		}

		sendResponsesStreamData(c, info, streamResponse, data)
		addResponseAffinityAliases(streamResponse.Response)
		if streamResponse.Item != nil {
			addAffinityAliases(streamResponse.Item.ID)
		}
		if streamResponse.ItemID != "" {
			addAffinityAliases(streamResponse.ItemID)
		}
		switch streamResponse.Type {
		case "response.completed", "response.incomplete":
			if streamResponse.Response != nil {
				if streamResponse.Response.Usage != nil {
					normalized := normalizedResponsesUsage(streamResponse.Response.Usage)
					*usage = normalized
				}
				if streamResponse.Response.HasImageGenerationCall() {
					c.Set("image_generation_call", true)
					c.Set("image_generation_call_quality", streamResponse.Response.GetQuality())
					c.Set("image_generation_call_size", streamResponse.Response.GetSize())
				}
			}
		case "response.output_text.delta":
			// 处理输出文本
			responseTextBuilder.WriteString(streamResponse.Delta)
		case dto.ResponsesOutputTypeItemDone:
			// 函数调用处理
			if streamResponse.Item != nil {
				switch streamResponse.Item.Type {
				case dto.BuildInCallWebSearchCall:
					if info != nil && info.ResponsesUsageInfo != nil && info.ResponsesUsageInfo.BuiltInTools != nil {
						if webSearchTool, exists := info.ResponsesUsageInfo.BuiltInTools[dto.BuildInToolWebSearchPreview]; exists && webSearchTool != nil {
							webSearchTool.CallCount++
						}
					}
				}
			}
		}
	})
	if streamErr != nil {
		return nil, streamErr
	}
	if retryWriter != nil && !retryWriter.committed {
		streamErr = responsesStreamPrematureEndError(info)
		markResponsesStreamErrorBeforeCommit(c, info)
		return nil, streamErr
	}
	recordResponsesStateChannelAffinity(c, info, affinityAliases)

	if usage.CompletionTokens == 0 {
		// 计算输出文本的 token 数量
		tempStr := responseTextBuilder.String()
		if len(tempStr) > 0 {
			// 非正常结束，使用输出文本的 token 数量
			completionTokens := service.CountTextToken(tempStr, info.UpstreamModelName)
			usage.CompletionTokens = completionTokens
		}
	}

	if usage.PromptTokens == 0 && usage.CompletionTokens != 0 {
		usage.PromptTokens = info.GetEstimatePromptTokens()
	}

	usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens

	return usage, nil
}

func responsesStreamEventError(streamResponse *dto.ResponsesStreamResponse) *types.NewAPIError {
	if streamResponse == nil {
		return nil
	}
	eventType := strings.ToLower(strings.TrimSpace(streamResponse.Type))
	if eventType != "error" && eventType != "response.error" && eventType != "response.failed" && streamResponse.Error == nil {
		return nil
	}

	var openAIError *types.OpenAIError
	if streamResponse.Response != nil {
		openAIError = streamResponse.Response.GetOpenAIError()
	}
	if openAIError == nil {
		openAIError = dto.GetOpenAIError(streamResponse.Error)
	}
	if openAIError == nil || (openAIError.Type == "" && openAIError.Message == "") {
		return types.NewOpenAIError(
			fmt.Errorf("upstream Responses stream returned %s", streamResponse.Type),
			types.ErrorCodeBadResponse,
			http.StatusBadGateway,
		)
	}
	return types.WithOpenAIError(*openAIError, responsesStreamErrorStatus(openAIError))
}

func responsesStreamErrorStatus(openAIError *types.OpenAIError) int {
	if openAIError == nil {
		return http.StatusBadGateway
	}
	errorType := strings.ToLower(strings.TrimSpace(openAIError.Type))
	errorCode := strings.ToLower(strings.TrimSpace(fmt.Sprintf("%v", openAIError.Code)))
	switch {
	case errorType == "too_many_requests", errorType == "rate_limit_error", errorType == "rate_limit_exceeded",
		errorCode == "rate_limit_exceeded", errorCode == "429",
		strings.Contains(strings.ToLower(openAIError.Message), "exceeded token rate limit"),
		strings.Contains(strings.ToLower(openAIError.Message), "exceeded request rate limit"):

		return http.StatusTooManyRequests
	case errorType == "invalid_request", errorType == "invalid_request_error":
		return http.StatusBadRequest
	case errorType == "server_error", errorType == "model_error", errorCode == "server_error":
		return http.StatusInternalServerError
	default:
		return http.StatusBadGateway
	}
}

func responsesStreamEventCommitsOutput(event *dto.ResponsesStreamResponse) bool {
	switch strings.ToLower(strings.TrimSpace(event.Type)) {
	case "response.created", "response.queued", "response.in_progress", "keepalive", "ping":
		return false
	case "response.output_item.added":
		// Empty message/reasoning/tool containers carry no visible output yet.
		// Azure can send them before an in-stream token rate-limit error.
		return !responsesOutputItemIsEmpty(event.Item)
	case "response.content_part.added", "response.reasoning_summary_part.added":
		return event.Part == nil || event.Part.Text != ""
	default:
		return true
	}
}

func responsesOutputItemIsEmpty(item *dto.ResponsesOutput) bool {
	if item == nil {
		return false
	}
	switch item.Type {
	case "message", "reasoning", "function_call", "custom_tool_call":
	default:
		return false
	}
	for _, content := range item.Content {
		if content.Text != "" {
			return false
		}
	}
	arguments := strings.TrimSpace(string(item.Arguments))
	return arguments == "" || arguments == `""` || arguments == "null"
}

func responsesStreamPrematureEndError(info *relaycommon.RelayInfo) *types.NewAPIError {
	if info != nil && info.StreamStatus != nil {
		switch info.StreamStatus.EndReason {
		case relaycommon.StreamEndReasonClientGone:
			return types.NewOpenAIError(
				fmt.Errorf("client disconnected before the Responses stream produced output"),
				types.ErrorCodeBadResponse,
				499,
				types.ErrOptionWithSkipRetry(),
			)
		case relaycommon.StreamEndReasonTimeout:
			return types.NewOpenAIError(
				fmt.Errorf("upstream Responses stream timed out before producing output"),
				types.ErrorCodeBadResponse,
				http.StatusGatewayTimeout,
			)
		}
	}
	return types.NewOpenAIError(
		fmt.Errorf("upstream Responses stream ended before completion or output"),
		types.ErrorCodeBadResponse,
		http.StatusBadGateway,
	)
}

func markResponsesStreamErrorBeforeCommit(c *gin.Context, info *relaycommon.RelayInfo) {
	if info != nil {
		info.ResponsesStreamErrorBeforeCommit = true
		info.ResetFirstResponseTime()
	}
	helper.ResetEventStreamHeaders(c)
	event := service.NewRelayCompatibilityEvent(
		service.ResponsesStreamErrorRetryRule,
		service.RelayCompatibilityEventTypeApplied,
	)
	event.Action = "retry_early_responses_stream_error_before_downstream_commit"
	event.Outcome = "accepted"
	event.Retried = true
	service.RecordRelayCompatibilityEvent(c, event)
}

func recordResponsesStreamRetryRecommendation(c *gin.Context) {
	event := service.NewRelayCompatibilityEvent(
		service.ResponsesStreamErrorRetryRule,
		service.RelayCompatibilityEventTypeRecommendation,
	)
	event.Action = "recommend_enable_rule"
	event.Outcome = "disabled"
	service.RecordRelayCompatibilityEvent(c, event)
}

func recordResponsesStateChannelAffinity(c *gin.Context, info *relaycommon.RelayInfo, aliases []string) {
	if info == nil {
		return
	}
	service.RecordResponsesStateChannelAffinity(c, info.ChannelId, info.OriginModelName, info.UsingGroup, aliases)
}
