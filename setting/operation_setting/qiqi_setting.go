package operation_setting

import "github.com/QuantumNous/new-api/setting/config"

type QiqiSetting struct {
	ContextRequestLoggingEnabled              bool `json:"context_request_logging_enabled"`
	ResponsesMissingReasoningItemRetryEnabled bool `json:"responses_missing_reasoning_item_retry_enabled"`
}

var qiqiSetting = QiqiSetting{
	ContextRequestLoggingEnabled:              false,
	ResponsesMissingReasoningItemRetryEnabled: true,
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
