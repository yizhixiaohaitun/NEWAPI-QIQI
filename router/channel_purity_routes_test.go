package router

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/controller"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/service/authz"
	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestChannelPurityRouteContracts(t *testing.T) {
	expected := map[string]authz.Permission{
		http.MethodPost + " /purity/groups":                                    authz.ChannelWrite,
		http.MethodGet + " /purity/groups":                                     authz.ChannelRead,
		http.MethodGet + " /purity/groups/:group_id":                           authz.ChannelRead,
		http.MethodPut + " /purity/groups/:group_id":                           authz.ChannelWrite,
		http.MethodDelete + " /purity/groups/:group_id":                        authz.ChannelSensitiveWrite,
		http.MethodPost + " /purity/groups/:group_id/run":                      authz.ChannelOperate,
		http.MethodGet + " /purity/groups/:group_id/run/:task_id":              authz.ChannelRead,
		http.MethodPost + " /purity/quick-probe":                               authz.ChannelOperate,
		http.MethodGet + " /purity/groups/:group_id/latest":                    authz.ChannelRead,
		http.MethodGet + " /purity/history":                                    authz.ChannelRead,
		http.MethodGet + " /purity/groups/:group_id/history":                   authz.ChannelRead,
		http.MethodGet + " /purity/groups/:group_id/history/preview":           authz.ChannelRead,
		http.MethodDelete + " /purity/groups/:group_id/history":                authz.ChannelSensitiveWrite,
		http.MethodPost + " /purity/groups/:group_id/alerts/:alert_id/actions": authz.ChannelOperate,
	}
	found := map[string]permissionRoute{}
	for _, route := range channelPermissionRoutes {
		key := route.method + " " + route.path
		if _, ok := expected[key]; ok {
			found[key] = route
		}
	}
	require.Len(t, found, len(expected))
	for key, permission := range expected {
		assert.Equal(t, permission, found[key].permission, key)
		require.NotNil(t, found[key].handler, key)
	}
	assert.NotNil(t, controller.ListChannelPurityGroups)
}

func TestChannelPurityUnauthenticatedErrorIsVisible(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(sessions.Sessions("acceptance", cookie.NewStore([]byte("acceptance-secret"))))
	engine.GET("/api/channel/purity/groups", middleware.AdminAuth(), controller.ListChannelPurityGroups)
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/channel/purity/groups", nil))

	assert.Equal(t, http.StatusUnauthorized, recorder.Code)
	var response map[string]any
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	assert.Equal(t, false, response["success"])
	assert.NotEmpty(t, response["message"])
}

func TestChannelOptionsSearchRouteContract(t *testing.T) {
	for _, route := range channelPermissionRoutes {
		if route.method == http.MethodGet && route.path == "/search" {
			assert.Equal(t, authz.ChannelRead, route.permission)
			require.NotNil(t, route.handler)
			return
		}
	}
	t.Fatal("GET /api/channel/search route is not registered")
}
