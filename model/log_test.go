package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFormatUserLogsRemovesCompatibilityAdminInfo(t *testing.T) {
	logs := []*Log{{
		ChannelName: "private-channel",
		Other:       `{"request_path":"/v1/responses","admin_info":{"compatibility_events":[{"rule_id":"QIQI-EC-001"}]}}`,
	}}

	formatUserLogs(logs, 0)

	require.Len(t, logs, 1)
	assert.Empty(t, logs[0].ChannelName)
	other, err := common.StrToMap(logs[0].Other)
	require.NoError(t, err)
	assert.Equal(t, "/v1/responses", other["request_path"])
	assert.NotContains(t, other, "admin_info")
}
