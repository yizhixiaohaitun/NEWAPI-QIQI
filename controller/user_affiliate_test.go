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

func TestRegisterRejectsInvalidAffiliateCode(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	oldRegisterEnabled := common.RegisterEnabled
	oldPasswordRegisterEnabled := common.PasswordRegisterEnabled
	common.RegisterEnabled = true
	common.PasswordRegisterEnabled = true
	t.Cleanup(func() {
		common.RegisterEnabled = oldRegisterEnabled
		common.PasswordRegisterEnabled = oldPasswordRegisterEnabled
	})

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/api/user/register", Register)

	request := httptest.NewRequest(
		http.MethodPost,
		"/api/user/register",
		strings.NewReader(`{"username":"invitee","password":"password123","aff_code":"missing-code"}`),
	)
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	assert.Equal(t, http.StatusOK, response.Code)
	assert.Contains(t, response.Body.String(), `"success":false`)
	assert.Contains(t, response.Body.String(), "user.aff_code_invalid")

	var users int64
	require.NoError(t, db.Model(&model.User{}).Count(&users).Error)
	assert.Zero(t, users)
}
