package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestAdminLogIdentityPrivacy(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.Log{}))
	require.NoError(t, db.Create(&model.Log{UserId: 42, Username: "secret-user", Type: model.LogTypeError, Content: "secret-user failed", Other: `{"admin_info":{"admin_username":"secret-admin"},"op":{"params":{"username":"secret-target","id":43}}}`}).Error)
	for _, tc := range []struct {
		role   int
		hidden bool
	}{{common.RoleAdminUser, true}, {common.RoleRootUser, false}} {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Set("role", tc.role)
		c.Request = httptest.NewRequest(http.MethodGet, "/api/log/?p=1&page_size=10", nil)
		GetAllLogs(c)
		require.Equal(t, 200, w.Code)
		if tc.hidden {
			require.NotContains(t, w.Body.String(), "secret-user")
			require.NotContains(t, w.Body.String(), "secret-admin")
			require.NotContains(t, w.Body.String(), "secret-target")
			require.Contains(t, w.Body.String(), `"user_id":42`)
		} else {
			require.Contains(t, w.Body.String(), "secret-user")
			require.Contains(t, w.Body.String(), "secret-admin")
			require.Contains(t, w.Body.String(), "secret-target")
		}
	}
	var saved model.Log
	require.NoError(t, db.First(&saved).Error)
	require.Equal(t, "secret-user", saved.Username)
}
