package controller

import (
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestAdminLogIdentityFilterAndTemplates(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.Log{}))
	require.NoError(t, db.Create(&model.Log{UserId: 42, Username: "secret", Type: model.LogTypeManage, Content: "Updated user target (ID: 43)", Other: `{"op":{"action":"user.update","params":{"username":"target","id":43}}}`}).Error)
	for _, tc := range []struct {
		role    int
		query   string
		found   bool
		content string
	}{
		{common.RoleAdminUser, "42", true, "Updated user 43 (ID: 43)"},
		{common.RoleAdminUser, "secret", false, ""},
		{common.RoleRootUser, "secret", true, "Updated user target (ID: 43)"},
	} {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Set("role", tc.role)
		c.Request = httptest.NewRequest("GET", "/api/log/?username="+tc.query, nil)
		GetAllLogs(c)
		if tc.found {
			require.Contains(t, w.Body.String(), tc.content)
		} else {
			require.NotContains(t, w.Body.String(), "Updated user")
		}
	}
}
