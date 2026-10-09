package relay

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestTaskModel2DtoSanitizesFailureWithoutMutatingTask(t *testing.T) {
	rawReason := "group claude-enterprise has no available channels"
	task := &model.Task{FailReason: rawReason}

	response := TaskModel2Dto(task)

	assert.Equal(t, "请求的上游分组无可用渠道", response.FailReason)
	assert.Equal(t, "请求的上游分组无可用渠道", response.ResultURL)
	assert.Equal(t, rawReason, task.FailReason)
}

func TestSanitizeTaskResponseBodyOnlyChangesExplicitFailureFields(t *testing.T) {
	raw := []byte(`{"code":"success","data":{"status":"failed","fail_reason":"group ClaudeCode_AZ has no available channels","message":"not authorized to access group claude-enterprise","error":{"metadata":{"details":["group ClaudeCode_EU has no available channels"]}},"prompt":"group ClaudeCode_AZ has no available channels"}}`)

	sanitized := string(model.SanitizeTaskPublicErrorJSON(raw))

	assert.NotContains(t, sanitized, `"fail_reason":"group ClaudeCode_AZ`)
	assert.NotContains(t, sanitized, `"message":"not authorized to access group claude-enterprise"`)
	assert.NotContains(t, sanitized, "ClaudeCode_EU")
	assert.Contains(t, sanitized, `"fail_reason":"请求的上游分组无可用渠道"`)
	assert.NotContains(t, sanitized, `"metadata"`)
	assert.NotContains(t, sanitized, `"details"`)
	assert.Contains(t, sanitized, `"prompt":"group ClaudeCode_AZ has no available channels"`)
}

func TestSanitizeTaskResponseBodyPreservesNormalOutput(t *testing.T) {
	raw := []byte(`{"code":"success","data":{"status":"succeeded","output":"group ClaudeCode_AZ has no available channels"}}`)

	assert.Equal(t, raw, []byte(model.SanitizeTaskPublicErrorJSON(raw)))
}

func TestRelayTaskFetchSanitizesConverterFailureAtFinalBoundary(t *testing.T) {
	const testMode = -20261009
	fetchRespBuilders[testMode] = func(*gin.Context) ([]byte, *dto.TaskError) {
		return []byte(`{"id":"task_public","status":"failed","error":{"message":"group ClaudeCode_AZ has no available channels"}}`), nil
	}
	t.Cleanup(func() { delete(fetchRespBuilders, testMode) })

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/v1/videos/task_public", nil)

	taskErr := RelayTaskFetch(ctx, testMode)

	assert.Nil(t, taskErr)
	assert.NotContains(t, recorder.Body.String(), "ClaudeCode_AZ")
	assert.Contains(t, recorder.Body.String(), "请求的上游分组无可用渠道")
}

func TestCoverMidjourneyTaskDtoSanitizesFailureWithoutMutatingTask(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(nil)
	rawReason := "group ClaudeCode_AZ has no available channels"
	rawDescription := "not authorized to access group claude-enterprise"
	rawID := "group ClaudeCode_AZ has no available channels"
	task := &model.Midjourney{
		MjId: rawID, FailReason: rawReason, Description: rawDescription,
		ImageUrl: "https://public.invalid/mj/image/" + rawID,
		VideoUrl: "https://public.invalid/mj/video/" + rawID,
	}

	response := coverMidjourneyTaskDto(ctx, task)

	assert.Equal(t, "请求的上游分组无可用渠道", response.FailReason)
	assert.Equal(t, "无权访问请求的上游分组", response.Description)
	assert.Empty(t, response.MjId)
	assert.Empty(t, response.ImageUrl)
	assert.Empty(t, response.VideoUrl)
	assert.Equal(t, rawReason, task.FailReason)
	assert.Equal(t, rawDescription, task.Description)
	assert.Equal(t, rawID, task.MjId)
}

func TestCoverMidjourneyTaskDtoPreservesSuccessfulFields(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(nil)
	task := &model.Midjourney{MjId: "mj-success", Status: "SUCCESS", Description: "completed", FailReason: ""}

	response := coverMidjourneyTaskDto(ctx, task)

	assert.Equal(t, task.MjId, response.MjId)
	assert.Equal(t, task.Description, response.Description)
	assert.Equal(t, task.FailReason, response.FailReason)
}

func TestSanitizeMidjourneySubmitResponseRebuildsGroupFailure(t *testing.T) {
	raw := []byte(`{"code":4,"description":"not authorized to access group claude-enterprise","result":"claude-enterprise","properties":{"group":"claude-enterprise"}}`)
	response := &dto.MidjourneyResponse{
		Code:        4,
		Description: "not authorized to access group claude-enterprise",
		Result:      "claude-enterprise",
		Properties:  map[string]any{"group": "claude-enterprise"},
	}

	publicBody := sanitizeMidjourneySubmitResponse(raw, response)

	assert.NotContains(t, string(publicBody), "claude-enterprise")
	assert.NotContains(t, string(publicBody), "properties")
	assert.NotContains(t, string(publicBody), "result")
	assert.Contains(t, string(publicBody), "上游分组")
	assert.Equal(t, "claude-enterprise", response.Result)
	assert.Equal(t, "not authorized to access group claude-enterprise", response.Description)
}

func TestSanitizeMidjourneySubmitResponsePreservesSuccessBytes(t *testing.T) {
	raw := []byte(` {"code":1,"description":"ok","result":"group claude-enterprise has no available channels","properties":{"prompt":"ordinary"}} `)
	response := &dto.MidjourneyResponse{Code: 1, Description: "ok", Result: "group claude-enterprise has no available channels"}

	assert.Equal(t, raw, sanitizeMidjourneySubmitResponse(raw, response))
}
