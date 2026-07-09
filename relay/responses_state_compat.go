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
	responsesMissingReasoningItemCompatibilityKey = "responses_missing_reasoning_item_retry"
	ginKeyResponsesMissingReasoningItemRetried    = "responses_missing_reasoning_item_retried"
)

var missingResponsesItemPattern = regexp.MustCompile(`(?i)^item with id ['"](rs_[a-z0-9]+)['"] not found\.?$`)

type responsesRetryRequestFunc func(io.Reader) (any, error)

func retryMissingResponsesReasoningItem(
	c *gin.Context,
	info *relaycommon.RelayInfo,
	source common.BodyStorage,
	upstreamError *types.NewAPIError,
	doRequest responsesRetryRequestFunc,
) (any, *types.NewAPIError, bool) {
	if !operation_setting.IsResponsesMissingReasoningItemRetryEnabled() || c == nil || info == nil || source == nil || doRequest == nil {
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
	normalized, removed, err := relaycommon.RemoveMissingResponsesReasoningItem(jsonData, itemID)
	if err != nil {
		logger.LogError(c, fmt.Sprintf("responses compatibility could not normalize retry body: %v", err))
		return nil, nil, false
	}
	if removed == 0 {
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
	logger.LogWarn(c, fmt.Sprintf("responses compatibility retry: removed missing reasoning item %s and retried on channel #%d", itemID, channelID))

	resp, requestErr := doRequest(body)
	outcome := "accepted"
	if requestErr != nil {
		outcome = "request_error"
	} else if httpResp, ok := resp.(*http.Response); !ok || httpResp == nil || httpResp.StatusCode != http.StatusOK {
		outcome = "upstream_error"
	}
	service.RecordRelayCompatibilityEvent(c, service.RelayCompatibilityEvent{
		Key:     responsesMissingReasoningItemCompatibilityKey,
		Action:  "remove_missing_empty_reasoning_item_and_retry_same_channel",
		Outcome: outcome,
		ItemID:  itemID,
		Count:   removed,
		Retried: true,
	})
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
