package controller

import (
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestAdminLogIdentityKeepsUpstreamAndExactNumbers(t *testing.T) {
	log := model.Log{UserId: 42, Username: "user", Type: model.LogTypeError, Content: "user: provider original", Other: `{"request_number":9007199254740993,"upstream_error":"user original","username":"user"}`}
	sanitizeAdminLogIdentity(&log)
	require.Equal(t, "42", log.Username)
	require.Equal(t, "user: provider original", log.Content)
	require.Contains(t, log.Other, `9007199254740993`)
	require.Contains(t, log.Other, `"upstream_error":"user original"`)
	require.Contains(t, log.Other, `"username":"42"`)
}
func TestAdminLogIdentityRendersOnlyLocalTemplate(t *testing.T) {
	log := model.Log{UserId: 42, Username: "user", Type: model.LogTypeManage, Content: "Updated user user (ID: 43)", Other: `{"op":{"action":"user.update","params":{"username":"user","id":43}}}`}
	sanitizeAdminLogIdentity(&log)
	require.Equal(t, "Updated user 43 (ID: 43)", log.Content)
	require.Contains(t, log.Other, `"username":"43"`)
}
