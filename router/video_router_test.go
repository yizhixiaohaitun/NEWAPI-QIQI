package router

import (
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestSetVideoRouterRegistersAsyncTaskRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	SetVideoRouter(engine)

	routes := map[string]bool{}
	for _, route := range engine.Routes() {
		routes[route.Method+" "+route.Path] = true
	}
	assert.True(t, routes["POST /async/tasks"])
	assert.True(t, routes["GET /async/tasks/:task_id"])
	assert.True(t, routes["POST /v1/video/assets"])
	assert.True(t, routes["GET /v1/video/assets/:asset_id"])
	assert.True(t, routes["POST /v1/videos"], "MiniMax-H3 route must remain registered")
	assert.True(t, routes["POST /v1/video/generations"], "existing Doubao route must remain registered")
}
