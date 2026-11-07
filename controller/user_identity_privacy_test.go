package controller

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestManagedUserIdentityPrivacyHTTP(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.Create(&model.User{Id: 42, Username: "secretusername", DisplayName: "secretdisplay", Password: "password123", Role: common.RoleCommonUser}).Error)
	role := common.RoleAdminUser
	r := gin.New()
	r.Use(func(c *gin.Context) { c.Set("role", role); c.Set("id", 99) })
	r.GET("/users", GetAllUsers)
	r.GET("/users/search", SearchUsers)
	r.GET("/users/:id", GetUser)
	r.PUT("/users", UpdateUser)
	request := func(method, path, body string) string {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		r.ServeHTTP(w, req)
		require.Equal(t, http.StatusOK, w.Code)
		return w.Body.String()
	}
	for _, path := range []string{"/users", "/users/search?keyword=42", "/users/42"} {
		body := request("GET", path, "")
		assert.Contains(t, body, `"success":true`)
		assert.Contains(t, body, `"username":"42"`)
		assert.Contains(t, body, `"display_name":"42"`)
		assert.NotContains(t, body, "secretusername")
		assert.NotContains(t, body, "secretdisplay")
	}
	body := request("PUT", "/users", `{"id":42,"username":"42","display_name":"42","group":"default"}`)
	assert.Contains(t, body, `"success":true`)
	var saved model.User
	require.NoError(t, db.First(&saved, 42).Error)
	assert.Equal(t, "secretusername", saved.Username)
	assert.Equal(t, "secretdisplay", saved.DisplayName)
	role = common.RoleRootUser
	for _, path := range []string{"/users", "/users/search?keyword=42", "/users/42"} {
		body := request("GET", path, "")
		assert.Contains(t, body, `"username":"secretusername"`)
		assert.Contains(t, body, `"display_name":"secretdisplay"`)
	}
}
