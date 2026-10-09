package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/require"
)

func TestChannelValidateSettingsRejectsNegativeInputTokenDeduction(t *testing.T) {
	for _, setting := range []string{
		`{"input_token_deduction":-1}`,
		`{"input_token_deduction_by_group":{"default":-1}}`,
	} {
		channel := &Channel{Setting: &setting}
		require.Error(t, channel.ValidateSettings())
	}
}

func TestFormatUserLogsStripsInputTokenDeductionAudit(t *testing.T) {
	logs := []*Log{{Other: common.MapToJsonStr(map[string]interface{}{
		"model_ratio": 1,
		"admin_info": map[string]interface{}{
			"input_token_deduction_original_input_tokens": 500,
			"input_token_deduction_applied":               490,
		},
	})}}

	formatUserLogs(logs, 0)

	parsed, err := common.StrToMap(logs[0].Other)
	require.NoError(t, err)
	require.NotContains(t, parsed, "admin_info")
	require.Contains(t, parsed, "model_ratio")
}

func TestChannelValidateSettingsAcceptsZeroAndPositiveInputTokenDeduction(t *testing.T) {
	setting := `{"input_token_deduction":490,"input_token_deduction_by_group":{"default":0,"vip":200}}`
	channel := &Channel{Setting: &setting}
	require.NoError(t, channel.ValidateSettings())

	parsed := channel.GetSetting()
	require.Equal(t, 490, parsed.InputTokenDeduction)
	require.Equal(t, 0, parsed.InputTokenDeductionByGroup["default"])
	require.Equal(t, 200, parsed.InputTokenDeductionByGroup["vip"])
}
