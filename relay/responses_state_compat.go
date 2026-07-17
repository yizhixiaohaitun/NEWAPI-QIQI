package relay

import (
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
)

const (
	ginKeyResponsesMissingReasoningItemRetried = "responses_missing_reasoning_item_retried"
	ginKeyResponsesEncryptedContentRetried     = "responses_encrypted_content_retried"
)

var missingResponsesItemPattern = regexp.MustCompile(`(?i)^item with id ['"](rs_[a-z0-9]+)['"] not found\.?$`)
var encryptedContentVerificationPattern = regexp.MustCompile(`(?i)^the encrypted content\s+\S+\s+could not be verified\.\s*reason:\s*encrypted content could not be decrypted or parsed\.?$`)

type responsesRetryRequestFunc func(io.Reader) (any, error)

func retryMissingResponsesReasoningItem(
	c *gin.Context,
	info *relaycommon.RelayInfo,
	source common.BodyStorage,
	upstreamError *types.NewAPIError,
	doRequest responsesRetryRequestFunc,
) (any, *types.NewAPIError, bool) {
	if c == nil || info == nil || source == nil {
		return nil, nil, false
	}
	if retried, _ := c.Get(ginKeyResponsesMissingReasoningItemRetried); retried == true {
		return nil, nil, false
	}

	itemID, ok := missingResponsesReasoningItemID(upstreamError)
	if !ok {
		return nil, nil, false
	}
	jsonData, err := source.Bytes()
	if err != nil {
		logger.LogError(c, fmt.Sprintf("responses compatibility could not read retry body: %v", err))
		return nil, nil, false
	}
	normalized, removed, err := relaycommon.RemoveMissingResponsesReasoningItems(jsonData, itemID)
	if err != nil {
		logger.LogError(c, fmt.Sprintf("responses compatibility could not normalize retry body: %v", err))
		return nil, nil, false
	}
	if removed == 0 {
		return nil, nil, false
	}

	if !operation_setting.IsResponsesMissingReasoningItemRetryEnabled() {
		event := service.NewRelayCompatibilityEvent(
			service.ResponsesMissingReasoningItemRule,
			service.RelayCompatibilityEventTypeRecommendation,
		)
		event.Action = "recommend_enable_rule"
		event.Outcome = "disabled"
		event.ItemID = itemID
		event.Count = removed
		service.RecordRelayCompatibilityEvent(c, event)
		logger.LogWarn(c, fmt.Sprintf(
			"responses compatibility recommendation: request matches rule %s but the rule is disabled",
			service.ResponsesMissingReasoningItemRule.ID,
		))
		return nil, nil, false
	}
	if doRequest == nil {
		return nil, nil, false
	}

	body, size, closer, err := relaycommon.NewOutboundJSONBody(normalized)
	if err != nil {
		logger.LogError(c, fmt.Sprintf("responses compatibility could not create retry body: %v", err))
		return nil, nil, false
	}
	defer closer.Close()

	c.Set(ginKeyResponsesMissingReasoningItemRetried, true)
	info.UpstreamRequestBodySize = size
	channelID := 0
	if info.ChannelMeta != nil {
		channelID = info.ChannelId
	}
	logger.LogWarn(c, fmt.Sprintf(
		"responses compatibility retry: trigger item %s, removed %d empty reasoning references, retried on channel #%d",
		itemID,
		removed,
		channelID,
	))

	resp, requestErr := doRequest(body)
	outcome := "accepted"
	if requestErr != nil {
		outcome = "request_error"
	} else if httpResp, ok := resp.(*http.Response); !ok || httpResp == nil || httpResp.StatusCode != http.StatusOK {
		outcome = "upstream_error"
	}
	event := service.NewRelayCompatibilityEvent(
		service.ResponsesMissingReasoningItemRule,
		service.RelayCompatibilityEventTypeApplied,
	)
	event.Action = "remove_all_empty_reasoning_references_and_retry_same_channel"
	event.Outcome = outcome
	event.ItemID = itemID
	event.Count = removed
	event.Retried = true
	service.RecordRelayCompatibilityEvent(c, event)
	if requestErr != nil {
		return nil, types.NewOpenAIError(requestErr, types.ErrorCodeDoRequestFailed, http.StatusInternalServerError), true
	}
	if httpResp, ok := resp.(*http.Response); !ok || httpResp == nil {
		return nil, types.NewOpenAIError(
			fmt.Errorf("responses compatibility retry returned unexpected response type %T", resp),
			types.ErrorCodeBadResponse,
			http.StatusInternalServerError,
		), true
	}
	return resp, nil, true
}

func retryUndecryptableResponsesReasoningContent(
	c *gin.Context,
	info *relaycommon.RelayInfo,
	source common.BodyStorage,
	upstreamError *types.NewAPIError,
	doRequest responsesRetryRequestFunc,
) (any, *types.NewAPIError, bool) {
	if c == nil || info == nil || source == nil || doRequest == nil || !isEncryptedContentVerificationError(upstreamError) {
		return nil, nil, false
	}
	if retried, _ := c.Get(ginKeyResponsesEncryptedContentRetried); retried == true {
		return nil, nil, false
	}

	jsonData, err := source.Bytes()
	if err != nil {
		logger.LogError(c, fmt.Sprintf("responses encrypted-content recovery could not read retry body: %v", err))
		return nil, nil, false
	}
	normalized, removed, err := relaycommon.RemoveUndecryptableResponsesReasoningItems(jsonData)
	if err != nil {
		logger.LogError(c, fmt.Sprintf("responses encrypted-content recovery could not normalize retry body: %v", err))
		return nil, nil, false
	}
	if removed == 0 {
		return nil, nil, false
	}

	body, size, closer, err := relaycommon.NewOutboundJSONBody(normalized)
	if err != nil {
		logger.LogError(c, fmt.Sprintf("responses encrypted-content recovery could not create retry body: %v", err))
		return nil, nil, false
	}
	defer closer.Close()

	c.Set(ginKeyResponsesEncryptedContentRetried, true)
	info.UpstreamRequestBodySize = size
	logger.LogWarn(c, fmt.Sprintf(
		"responses encrypted-content recovery: removed %d undecryptable reasoning item(s) and retried once on channel #%d",
		removed,
		info.ChannelId,
	))

	resp, requestErr := doRequest(body)
	outcome := "accepted"
	if requestErr != nil {
		outcome = "request_error"
	} else if httpResp, ok := resp.(*http.Response); !ok || httpResp == nil || httpResp.StatusCode != http.StatusOK {
		outcome = "upstream_error"
	}
	event := service.NewRelayCompatibilityEvent(
		service.ResponsesEncryptedContentRecoveryRule,
		service.RelayCompatibilityEventTypeApplied,
	)
	event.Action = "remove_encrypted_reasoning_items_and_retry_same_channel_once"
	event.Outcome = outcome
	event.Count = removed
	event.Retried = true
	service.RecordRelayCompatibilityEvent(c, event)

	if requestErr != nil {
		return nil, types.NewOpenAIError(requestErr, types.ErrorCodeDoRequestFailed, http.StatusInternalServerError), true
	}
	httpResp, ok := resp.(*http.Response)
	if !ok || httpResp == nil {
		return nil, types.NewOpenAIError(
			fmt.Errorf("responses encrypted-content recovery returned unexpected response type %T", resp),
			types.ErrorCodeBadResponse,
			http.StatusInternalServerError,
		), true
	}
	return httpResp, nil, true
}

func isEncryptedContentVerificationError(err *types.NewAPIError) bool {
	if err == nil || err.StatusCode != http.StatusBadRequest || err.GetErrorType() != types.ErrorTypeOpenAIError {
		return false
	}
	return encryptedContentVerificationPattern.MatchString(strings.TrimSpace(err.ToOpenAIError().Message))
}

func encryptedContentRecoveryUserError(err *types.NewAPIError) *types.NewAPIError {
	if !isEncryptedContentVerificationError(err) {
		return err
	}
	return types.NewOpenAIError(
		fmt.Errorf("上游无法验证会话中的加密推理内容；已尝试一次安全恢复但仍失败，请新建会话或移除旧的推理上下文后重试"),
		types.ErrorCodeInvalidRequest,
		http.StatusBadRequest,
		types.ErrOptionWithSkipRetry(),
	)
}

func missingResponsesReasoningItemID(err *types.NewAPIError) (string, bool) {
	if err == nil || err.StatusCode != http.StatusBadRequest || err.GetErrorType() != types.ErrorTypeOpenAIError {
		return "", false
	}
	openAIError := err.ToOpenAIError()
	if !strings.EqualFold(strings.TrimSpace(openAIError.Param), "input") ||
		!strings.EqualFold(strings.TrimSpace(openAIError.Type), "invalid_request_error") {
		return "", false
	}
	matches := missingResponsesItemPattern.FindStringSubmatch(strings.TrimSpace(openAIError.Message))
	if len(matches) != 2 {
		return "", false
	}
	return matches[1], true
}
