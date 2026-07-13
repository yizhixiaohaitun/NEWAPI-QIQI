package openai

import (
	"bytes"
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

const responsesStreamRetryBufferLimit = 4 << 20

type responsesStreamRetryWriter struct {
	gin.ResponseWriter
	header        http.Header
	buffer        bytes.Buffer
	pendingStatus int
	committed     bool
}

func newResponsesStreamRetryWriter(writer gin.ResponseWriter) *responsesStreamRetryWriter {
	return &responsesStreamRetryWriter{
		ResponseWriter: writer,
		header:         writer.Header().Clone(),
	}
}

func (w *responsesStreamRetryWriter) Header() http.Header {
	return w.header
}

func (w *responsesStreamRetryWriter) WriteHeader(code int) {
	if w.committed {
		w.ResponseWriter.WriteHeader(code)
		return
	}
	if w.pendingStatus == 0 {
		w.pendingStatus = code
	}
}

func (w *responsesStreamRetryWriter) WriteHeaderNow() {
	if w.committed {
		w.ResponseWriter.WriteHeaderNow()
		return
	}
	if w.pendingStatus == 0 {
		w.pendingStatus = http.StatusOK
	}
}

func (w *responsesStreamRetryWriter) Write(data []byte) (int, error) {
	if w.committed {
		return w.ResponseWriter.Write(data)
	}
	if w.buffer.Len()+len(data) > responsesStreamRetryBufferLimit {
		if err := w.Commit(); err != nil {
			return 0, err
		}
		return w.ResponseWriter.Write(data)
	}
	return w.buffer.Write(data)
}

func (w *responsesStreamRetryWriter) WriteString(data string) (int, error) {
	if w.committed {
		return w.ResponseWriter.WriteString(data)
	}
	if w.buffer.Len()+len(data) > responsesStreamRetryBufferLimit {
		if err := w.Commit(); err != nil {
			return 0, err
		}
		return w.ResponseWriter.WriteString(data)
	}
	return w.buffer.WriteString(data)
}

func (w *responsesStreamRetryWriter) Flush() {
	if w.committed {
		w.ResponseWriter.Flush()
	}
}

func (w *responsesStreamRetryWriter) Written() bool {
	return w.committed && w.ResponseWriter.Written()
}

func (w *responsesStreamRetryWriter) Status() int {
	if w.pendingStatus != 0 {
		return w.pendingStatus
	}
	return w.ResponseWriter.Status()
}

func (w *responsesStreamRetryWriter) Size() int {
	if w.committed {
		return w.ResponseWriter.Size()
	}
	return w.buffer.Len()
}

func (w *responsesStreamRetryWriter) Commit() error {
	if w.committed {
		return nil
	}
	for key, values := range w.header {
		w.ResponseWriter.Header()[key] = append([]string(nil), values...)
	}
	if w.pendingStatus != 0 {
		w.ResponseWriter.WriteHeader(w.pendingStatus)
	}
	w.committed = true
	if w.buffer.Len() == 0 {
		return nil
	}
	_, err := w.ResponseWriter.Write(w.buffer.Bytes())
	return err
}

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

	// 写入新的 response body
	service.IOCopyBytesGracefully(c, resp, responseBody)

	// compute usage
	usage := dto.Usage{}
	if responsesResponse.Usage != nil {
		usage.PromptTokens = responsesResponse.Usage.InputTokens
		usage.CompletionTokens = responsesResponse.Usage.OutputTokens
		usage.TotalTokens = responsesResponse.Usage.TotalTokens
		if responsesResponse.Usage.InputTokensDetails != nil {
			usage.PromptTokensDetails.CachedTokens = responsesResponse.Usage.InputTokensDetails.CachedTokens
			usage.PromptTokensDetails.CacheWriteTokens = responsesResponse.Usage.InputTokensDetails.CacheWriteTokens
		}
	}
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
	var retryWriter *responsesStreamRetryWriter
	if retryEnabled {
		originalWriter := c.Writer
		retryWriter = newResponsesStreamRetryWriter(originalWriter)
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
				sendResponsesStreamData(c, streamResponse, data)
				return
			}
			if retryWriter != nil && !retryWriter.committed {
				streamErr = upstreamStreamErr
				markResponsesStreamErrorBeforeCommit(c, info)
				sr.Stop(streamErr)
				return
			}
			sr.Error(upstreamStreamErr)
			sendResponsesStreamData(c, streamResponse, data)
			return
		}

		if retryWriter != nil && !retryWriter.committed && responsesStreamEventCommitsOutput(streamResponse.Type) {
			info.ResetFirstResponseTime()
			info.SetFirstResponseTime()
			if err := retryWriter.Commit(); err != nil {
				streamErr = types.NewOpenAIError(err, types.ErrorCodeBadResponse, http.StatusInternalServerError)
				sr.Stop(streamErr)
				return
			}
		}

		sendResponsesStreamData(c, streamResponse, data)
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
					if streamResponse.Response.Usage.InputTokens != 0 {
						usage.PromptTokens = streamResponse.Response.Usage.InputTokens
					}
					if streamResponse.Response.Usage.OutputTokens != 0 {
						usage.CompletionTokens = streamResponse.Response.Usage.OutputTokens
					}
					if streamResponse.Response.Usage.TotalTokens != 0 {
						usage.TotalTokens = streamResponse.Response.Usage.TotalTokens
					}
					if streamResponse.Response.Usage.InputTokensDetails != nil {
						usage.PromptTokensDetails.CachedTokens = streamResponse.Response.Usage.InputTokensDetails.CachedTokens
						usage.PromptTokensDetails.CacheWriteTokens = streamResponse.Response.Usage.InputTokensDetails.CacheWriteTokens
					}
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
	case errorType == "too_many_requests", errorCode == "rate_limit_exceeded":
		return http.StatusTooManyRequests
	case errorType == "invalid_request", errorType == "invalid_request_error":
		return http.StatusBadRequest
	case errorType == "server_error", errorType == "model_error", errorCode == "server_error":
		return http.StatusInternalServerError
	default:
		return http.StatusBadGateway
	}
}

func responsesStreamEventCommitsOutput(eventType string) bool {
	switch strings.ToLower(strings.TrimSpace(eventType)) {
	case "response.created", "response.queued", "response.in_progress", "keepalive", "ping":
		return false
	default:
		return true
	}
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
