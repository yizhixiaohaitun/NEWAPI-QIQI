package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestGetModelProbeReturnsPersistedMetadataWithoutBodies(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.ModelProbeResult{}))
	require.True(t, db.Migrator().HasTable(&model.ModelProbeResult{}))
	for _, column := range []string{"request_model", "declared_model", "actual_model", "expected_tokens", "actual_tokens", "conclusion"} {
		require.True(t, db.Migrator().HasColumn(&model.ModelProbeResult{}, column), "migration must create %s", column)
	}
	for _, forbidden := range []string{"prompt", "request_body", "response_body"} {
		require.False(t, db.Migrator().HasColumn(&model.ModelProbeResult{}, forbidden), "privacy-sensitive column %s must not exist", forbidden)
	}

	originalDB := model.DB
	model.DB = db
	t.Cleanup(func() { model.DB = originalDB })

	expected := 8
	actual := 9
	require.NoError(t, model.CreateModelProbeResult(&model.ModelProbeResult{
		ChannelId:      3,
		ChannelName:    "test channel",
		RequestModel:   "gpt-4o",
		DeclaredModel:  "gpt-4o",
		ActualModel:    "gpt-4o-mini",
		ExpectedTokens: &expected,
		ActualTokens:   &actual,
		IdStatus:       "mismatch",
		TokenStatus:    "match",
		Conclusion:     "suspicious",
	}))

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/channel/model_probe?limit=20", nil)

	GetModelProbe(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	body := recorder.Body.String()
	require.Contains(t, body, `"request_model":"gpt-4o"`)
	require.Contains(t, body, `"actual_model":"gpt-4o-mini"`)
	require.NotContains(t, body, `"prompt"`)
	require.NotContains(t, body, `"request_body"`)
	require.NotContains(t, body, `"response_body"`)
}
