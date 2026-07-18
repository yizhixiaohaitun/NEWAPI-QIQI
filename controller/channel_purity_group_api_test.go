package controller

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupPurityAPITestDB(t *testing.T) {
	t.Helper()
	initModelListColumnNames(t)
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	old := model.DB
	dsn := fmt.Sprintf("file:purity-api-%d?mode=memory&cache=shared", time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.Exec("PRAGMA foreign_keys = ON").Error)
	require.NoError(t, db.AutoMigrate(
		&model.Channel{}, &model.ChannelPurityGroup{}, &model.ChannelPurityMember{},
		&model.ChannelPuritySample{}, &model.ChannelPurityPairRun{},
		&model.ChannelPurityAssessment{}, &model.ChannelPurityAlert{},
		&model.SystemTask{}, &model.SystemTaskLock{},
	))
	model.DB = db
	t.Cleanup(func() { model.DB = old })
}

func purityRequest(t *testing.T, method, target, body string, handler gin.HandlerFunc, params ...gin.Param) *httptest.ResponseRecorder {
	t.Helper()
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(method, target, bytes.NewBufferString(body))
	ctx.Request.Header.Set("Content-Type", "application/json")
	ctx.Params = params
	handler(ctx)
	return recorder
}

func decodeEnvelope(t *testing.T, recorder *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var response map[string]any
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response), recorder.Body.String())
	return response
}

func TestChannelPurityGroupCRUDAndListContract(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupPurityAPITestDB(t)
	require.NoError(t, model.DB.Create(&model.Channel{Id: 101, Name: "baseline", Key: "secret-a", Models: "gpt-4o", Status: common.ChannelStatusEnabled}).Error)
	require.NoError(t, model.DB.Create(&model.Channel{Id: 102, Name: "target", Key: "secret-b", Models: "gpt-4o", Status: common.ChannelStatusEnabled}).Error)

	create := purityRequest(t, http.MethodPost, "/api/channel/purity/groups", `{
		"name":"api-acceptance","enabled":true,"channel_ids":[101,102],"baseline_channel_id":101,
		"interval_minutes":5,"random_pairing_enabled":true,
		"sampling":{"window_minutes":30,"minimum_samples":2,"max_samples_per_window":20}
	}`, CreateChannelPurityGroup)
	require.Equal(t, http.StatusCreated, create.Code, create.Body.String())
	created := decodeEnvelope(t, create)
	assert.Equal(t, true, created["success"])
	data := created["data"].(map[string]any)
	groupID := uint(data["id"].(float64))
	assert.Equal(t, float64(101), data["baseline_channel_id"])
	assert.Equal(t, "baseline", data["baseline_channel_name"])
	assert.Equal(t, true, data["random_pairing_enabled"])
	assert.Contains(t, data, "results")
	assert.Contains(t, data, "last_run_at")
	assert.Contains(t, data, "next_run_at")
	assert.Contains(t, data, "last_error")

	run := &model.ChannelPurityPairRun{
		GroupID: groupID, BaselineChannelID: 101, TargetChannelID: 102, ActualModel: "gpt-4o",
		WindowStartedAt: 100, WindowEndedAt: 200, BaselineSampleCount: 3, TargetSampleCount: 3,
		PairedSampleCount: 3, StructureSimilarity: 0.9, TokenSimilarity: 0.8,
		BaselineTokenMin: 10, BaselineTokenMax: 20, TargetTokenMin: 11, TargetTokenMax: 22,
		TokenDeviationRate: 0.1, AnomalyEvidenceJSON: `["token_interval_shift"]`, Confidence: 0.75,
		State: model.ChannelPurityStateSuspect, CreatedAt: 200,
	}
	require.NoError(t, model.DB.Create(run).Error)
	require.NoError(t, model.DB.Create(&model.ChannelPurityAssessment{
		GroupID: groupID, TargetChannelID: 102, ActualModel: "gpt-4o", LatestPairRunID: run.ID,
		State: model.ChannelPurityStateSuspect, Confidence: 0.75, FirstSeenAt: 200, UpdatedAt: 200,
	}).Error)

	list := purityRequest(t, http.MethodGet, "/api/channel/purity/groups", "", ListChannelPurityGroups)
	require.Equal(t, http.StatusOK, list.Code, list.Body.String())
	listed := decodeEnvelope(t, list)["data"].([]any)
	require.Len(t, listed, 1)
	listedGroup := listed[0].(map[string]any)
	assert.Equal(t, "api-acceptance", listedGroup["name"])
	results := listedGroup["results"].([]any)
	require.Len(t, results, 1)
	result := results[0].(map[string]any)
	for _, field := range []string{"target_channel_id", "target_channel_name", "baseline_channel_id", "baseline_channel_name", "model", "status", "samples", "field_similarity", "token_similarity", "confidence", "baseline_token_range", "target_token_range", "deviation_rate", "evidence", "alerts", "trend", "updated_at"} {
		assert.Contains(t, result, field)
	}
	assert.Equal(t, "target", result["target_channel_name"])
	assert.Equal(t, float64(3), result["samples"])

	update := purityRequest(t, http.MethodPut, fmt.Sprintf("/api/channel/purity/groups/%d", groupID), `{
		"name":"api-acceptance-updated","enabled":true,
		"members":[{"channel_id":101,"is_baseline":true},{"channel_id":102,"is_baseline":false}],
		"interval_minutes":10,"sampling":{"window_minutes":30,"minimum_samples":3,"max_samples_per_window":30}
	}`, UpdateChannelPurityGroup, gin.Param{Key: "group_id", Value: fmt.Sprint(groupID)})
	require.Equal(t, http.StatusOK, update.Code, update.Body.String())
	updated := decodeEnvelope(t, update)["data"].(map[string]any)
	assert.Equal(t, "api-acceptance-updated", updated["name"])
	assert.Equal(t, float64(10), updated["interval_minutes"])

	invalid := purityRequest(t, http.MethodPost, "/api/channel/purity/groups", `{
		"name":"invalid","enabled":true,"channel_ids":[101,102],"baseline_channel_id":999,
		"interval_minutes":5,"sampling":{"window_minutes":30,"minimum_samples":2,"max_samples_per_window":20}
	}`, CreateChannelPurityGroup)
	assert.Equal(t, http.StatusBadRequest, invalid.Code)
	assert.Equal(t, false, decodeEnvelope(t, invalid)["success"])

	deleted := purityRequest(t, http.MethodDelete, fmt.Sprintf("/api/channel/purity/groups/%d", groupID), "", DeleteChannelPurityGroup, gin.Param{Key: "group_id", Value: fmt.Sprint(groupID)})
	require.Equal(t, http.StatusOK, deleted.Code, deleted.Body.String())
	missing := purityRequest(t, http.MethodGet, fmt.Sprintf("/api/channel/purity/groups/%d", groupID), "", GetChannelPurityGroup, gin.Param{Key: "group_id", Value: fmt.Sprint(groupID)})
	assert.Equal(t, http.StatusNotFound, missing.Code)
}

func TestChannelPurityManualRunEnqueuesFormalTask(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupPurityAPITestDB(t)
	group := &model.ChannelPurityGroup{Name: "run-group", Enabled: true, IntervalMinutes: 5, WindowMinutes: 30, MinimumSamples: 1, MaxSamplesPerWindow: 10, Members: []model.ChannelPurityMember{{ChannelID: 1, IsBaseline: true}, {ChannelID: 2}}}
	require.NoError(t, model.CreatePurityGroup(group))

	run := purityRequest(t, http.MethodPost, fmt.Sprintf("/api/channel/purity/groups/%d/run", group.ID), "", StartChannelPurityGroupDetection, gin.Param{Key: "group_id", Value: fmt.Sprint(group.ID)})
	require.Equal(t, http.StatusAccepted, run.Code, run.Body.String())
	response := decodeEnvelope(t, run)
	assert.Equal(t, true, response["success"])
	data := response["data"].(map[string]any)
	assert.Equal(t, model.SystemTaskTypeChannelPurityAggregate, data["type"])
	assert.Equal(t, string(model.SystemTaskStatusPending), data["status"])

	conflict := purityRequest(t, http.MethodPost, fmt.Sprintf("/api/channel/purity/groups/%d/run", group.ID), "", StartChannelPurityGroupDetection, gin.Param{Key: "group_id", Value: fmt.Sprint(group.ID)})
	assert.Equal(t, http.StatusConflict, conflict.Code)
	assert.Equal(t, false, decodeEnvelope(t, conflict)["success"])
}

func TestChannelPurityScheduledHandlerExecutesDueEmptyPass(t *testing.T) {
	setupPurityAPITestDB(t)
	handler := channelPurityGroupDetectionHandler{}
	assert.Equal(t, model.SystemTaskTypeChannelPurityAggregate, handler.Type())
	assert.Equal(t, 5*time.Minute, handler.Interval())
	assert.False(t, handler.Enabled())

	task, err := model.CreateSystemTask(handler.Type(), channelPurityGroupDetectionPayload{}, nil)
	require.NoError(t, err)
	claimed, ok, err := model.ClaimSystemTask(task.ID, task.Type, "acceptance-runner", time.Now().Add(time.Minute).Unix())
	require.NoError(t, err)
	require.True(t, ok)
	handler.Run(context.Background(), claimed, "acceptance-runner")
	finished, err := model.GetSystemTaskByTaskID(task.TaskID)
	require.NoError(t, err)
	require.NotNil(t, finished)
	assert.Equal(t, model.SystemTaskStatusSucceeded, finished.Status)
	assert.Contains(t, finished.Result, `"groups":0`)
}

func TestChannelOptionsSearchContractForPurityUI(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupPurityAPITestDB(t)
	require.NoError(t, model.DB.Create(&model.Channel{Id: 201, Name: "option-a", Key: "must-not-leak", Models: "gpt-4o,gpt-4o-mini", Status: common.ChannelStatusEnabled}).Error)

	recorder := purityRequest(t, http.MethodGet, "/api/channel/search?p=1&page_size=1000", "", SearchChannels)
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	response := decodeEnvelope(t, recorder)
	data := response["data"].(map[string]any)
	items := data["items"].([]any)
	require.Len(t, items, 1)
	option := items[0].(map[string]any)
	assert.Equal(t, float64(201), option["id"])
	assert.Equal(t, "option-a", option["name"])
	assert.Equal(t, "gpt-4o,gpt-4o-mini", option["models"])
	assert.Empty(t, option["key"], "channel option response must not expose the stored credential")
}

func TestChannelPurityQuickProbeValidationIsVisible(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := purityRequest(t, http.MethodPost, "/api/channel/purity/quick-probe", `{}`, RunChannelPurityQuickProbe)
	assert.Equal(t, http.StatusBadRequest, recorder.Code)
	response := decodeEnvelope(t, recorder)
	assert.Equal(t, false, response["success"])
	assert.Contains(t, response["message"], "channel_id")
}
