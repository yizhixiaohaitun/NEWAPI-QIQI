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
	assert.Contains(t, sanitized, `"message":"无权访问请求的上游分组"`)
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
	task := &model.Midjourney{FailReason: rawReason}

	response := coverMidjourneyTaskDto(ctx, task)

	assert.Equal(t, "请求的上游分组无可用渠道", response.FailReason)
	assert.Equal(t, rawReason, task.FailReason)
}
