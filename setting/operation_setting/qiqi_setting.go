package operation_setting

import (
	"fmt"
	"strconv"

	"github.com/QuantumNous/new-api/setting/config"
)

const (
	DefaultResponsesStreamErrorRetryTimes = 2
	MaxResponsesStreamErrorRetryTimes     = 5
)

type QiqiSetting struct {
	ContextRequestLoggingEnabled              bool `json:"context_request_logging_enabled"`
	ResponsesMissingReasoningItemRetryEnabled bool `json:"responses_missing_reasoning_item_retry_enabled"`
	AzureResponsesResourceAffinityEnabled     bool `json:"azure_responses_resource_affinity_enabled"`
	ResponsesStreamErrorRetryEnabled          bool `json:"responses_stream_error_retry_enabled"`
	ResponsesStreamErrorRetryTimes            int  `json:"responses_stream_error_retry_times"`
}

var qiqiSetting = QiqiSetting{
	ContextRequestLoggingEnabled:              false,
	ResponsesMissingReasoningItemRetryEnabled: true,
	AzureResponsesResourceAffinityEnabled:     true,
	ResponsesStreamErrorRetryEnabled:          true,
	ResponsesStreamErrorRetryTimes:            DefaultResponsesStreamErrorRetryTimes,
}

func init() {
	config.GlobalConfig.Register("qiqi_setting", &qiqiSetting)
}

func GetQiqiSetting() *QiqiSetting {
	return &qiqiSetting
}

func IsContextRequestLoggingEnabled() bool {
	return qiqiSetting.ContextRequestLoggingEnabled
}

func IsResponsesMissingReasoningItemRetryEnabled() bool {
	return qiqiSetting.ResponsesMissingReasoningItemRetryEnabled
}

func IsAzureResponsesResourceAffinityEnabled() bool {
	return qiqiSetting.AzureResponsesResourceAffinityEnabled
}

func IsResponsesStreamErrorRetryEnabled() bool {
	return qiqiSetting.ResponsesStreamErrorRetryEnabled
}

func GetResponsesStreamErrorRetryTimes() int {
	retryTimes := qiqiSetting.ResponsesStreamErrorRetryTimes
	if retryTimes < 0 {
		return 0
	}
	if retryTimes > MaxResponsesStreamErrorRetryTimes {
		return MaxResponsesStreamErrorRetryTimes
	}
	return retryTimes
}

func ValidateResponsesStreamErrorRetryTimes(value string) error {
	retryTimes, err := strconv.Atoi(value)
	if err != nil || retryTimes < 0 || retryTimes > MaxResponsesStreamErrorRetryTimes {
		return fmt.Errorf("Responses stream error retry times must be an integer from 0 to %d", MaxResponsesStreamErrorRetryTimes)
	}
	return nil
}
