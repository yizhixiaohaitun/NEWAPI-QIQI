package operation_setting

import (
	"fmt"
	"strconv"

	"github.com/QuantumNous/new-api/setting/config"
)

const (
	DefaultResponsesStreamErrorRetryTimes = 2
	MaxResponsesStreamErrorRetryTimes     = 5

	DefaultChannelPurityInspectionIntervalMinutes = 5
	MinChannelPurityInspectionIntervalMinutes     = 5
	MaxChannelPurityInspectionIntervalMinutes     = 10

	DefaultContextRequestLogRetentionDays = 0
	MaxContextRequestLogRetentionDays     = 3650
)

type QiqiSetting struct {
	ContextRequestLoggingEnabled              bool `json:"context_request_logging_enabled"`
	ZeroReplyAutoRefundEnabled                bool `json:"zero_reply_auto_refund_enabled"`
	ContextRequestLogRetentionDays            int  `json:"context_request_log_retention_days"`
	ResponsesMissingReasoningItemRetryEnabled bool `json:"responses_missing_reasoning_item_retry_enabled"`
	AzureResponsesResourceAffinityEnabled     bool `json:"azure_responses_resource_affinity_enabled"`
	ResponsesStreamErrorRetryEnabled          bool `json:"responses_stream_error_retry_enabled"`
	ResponsesStreamErrorRetryTimes            int  `json:"responses_stream_error_retry_times"`
	ChannelPurityInspectionEnabled            bool `json:"channel_purity_inspection_enabled"`
	ChannelPurityInspectionIntervalMinutes    int  `json:"channel_purity_inspection_interval_minutes"`
}

var qiqiSetting = QiqiSetting{
	ContextRequestLoggingEnabled:              false,
	ZeroReplyAutoRefundEnabled:                false,
	ContextRequestLogRetentionDays:            DefaultContextRequestLogRetentionDays,
	ResponsesMissingReasoningItemRetryEnabled: true,
	AzureResponsesResourceAffinityEnabled:     true,
	ResponsesStreamErrorRetryEnabled:          true,
	ResponsesStreamErrorRetryTimes:            DefaultResponsesStreamErrorRetryTimes,
	ChannelPurityInspectionEnabled:            false,
	ChannelPurityInspectionIntervalMinutes:    DefaultChannelPurityInspectionIntervalMinutes,
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

// IsZeroReplyAutoRefundEnabled 返回「0回复自动回退额度」开关状态。
// 开启后，最终结算为消耗类、prompt_tokens>0 且 completion_tokens=0 的请求
// 会在结算时实时退回已扣 quota（详见 service.MaybeAutoRefundZeroReplyQuota）。
func IsZeroReplyAutoRefundEnabled() bool {
	return qiqiSetting.ZeroReplyAutoRefundEnabled
}

func GetContextRequestLogRetentionDays() int {
	days := qiqiSetting.ContextRequestLogRetentionDays
	if days < 0 || days > MaxContextRequestLogRetentionDays {
		return DefaultContextRequestLogRetentionDays
	}
	return days
}

func ValidateContextRequestLogRetentionDays(value string) error {
	days, err := strconv.Atoi(value)
	if err != nil || days < 0 || days > MaxContextRequestLogRetentionDays {
		return fmt.Errorf("Context request log retention days must be an integer from 0 to %d", MaxContextRequestLogRetentionDays)
	}
	return nil
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

func IsChannelPurityInspectionEnabled() bool {
	return qiqiSetting.ChannelPurityInspectionEnabled
}

func GetChannelPurityInspectionIntervalMinutes() int {
	minutes := qiqiSetting.ChannelPurityInspectionIntervalMinutes
	if minutes < MinChannelPurityInspectionIntervalMinutes || minutes > MaxChannelPurityInspectionIntervalMinutes {
		return DefaultChannelPurityInspectionIntervalMinutes
	}
	return minutes
}

func ValidateChannelPurityInspectionIntervalMinutes(value string) error {
	minutes, err := strconv.Atoi(value)
	if err != nil || minutes < MinChannelPurityInspectionIntervalMinutes || minutes > MaxChannelPurityInspectionIntervalMinutes {
		return fmt.Errorf("Channel purity inspection interval must be an integer from %d to %d minutes", MinChannelPurityInspectionIntervalMinutes, MaxChannelPurityInspectionIntervalMinutes)
	}
	return nil
}
