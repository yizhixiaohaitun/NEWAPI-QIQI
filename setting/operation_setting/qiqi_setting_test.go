package operation_setting

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAzureResponsesResourceAffinityEnabledByDefault(t *testing.T) {
	assert.True(t, IsAzureResponsesResourceAffinityEnabled())
}

func TestResponsesStreamErrorRetryTimesValidationAndBounds(t *testing.T) {
	require.NoError(t, ValidateResponsesStreamErrorRetryTimes("0"))
	require.NoError(t, ValidateResponsesStreamErrorRetryTimes("2"))
	require.NoError(t, ValidateResponsesStreamErrorRetryTimes("5"))
	require.Error(t, ValidateResponsesStreamErrorRetryTimes("-1"))
	require.Error(t, ValidateResponsesStreamErrorRetryTimes("6"))
	require.Error(t, ValidateResponsesStreamErrorRetryTimes("two"))

	setting := GetQiqiSetting()
	original := *setting
	t.Cleanup(func() { *setting = original })

	setting.ResponsesStreamErrorRetryTimes = -1
	assert.Zero(t, GetResponsesStreamErrorRetryTimes())
	setting.ResponsesStreamErrorRetryTimes = MaxResponsesStreamErrorRetryTimes + 1
	assert.Equal(t, MaxResponsesStreamErrorRetryTimes, GetResponsesStreamErrorRetryTimes())
}

func TestChannelPurityInspectionIntervalValidationAndFallback(t *testing.T) {
	require.NoError(t, ValidateChannelPurityInspectionIntervalMinutes("15"))
	require.NoError(t, ValidateChannelPurityInspectionIntervalMinutes("360"))
	require.NoError(t, ValidateChannelPurityInspectionIntervalMinutes("10080"))
	require.Error(t, ValidateChannelPurityInspectionIntervalMinutes("14"))
	require.Error(t, ValidateChannelPurityInspectionIntervalMinutes("10081"))
	require.Error(t, ValidateChannelPurityInspectionIntervalMinutes("daily"))

	setting := GetQiqiSetting()
	original := *setting
	t.Cleanup(func() { *setting = original })

	setting.ChannelPurityInspectionIntervalMinutes = 0
	assert.Equal(t, DefaultChannelPurityInspectionIntervalMinutes, GetChannelPurityInspectionIntervalMinutes())
}
