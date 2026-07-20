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
		&model.Channel{}, &model.ChannelPurityGroup{}, &model.ChannelPurityMember{}, &model.ChannelPurityModelComparison{},
		&model.ChannelPuritySample{}, &model.ChannelPurityPairRun{},
		&model.ChannelPurityAssessment{}, &model.ChannelPurityAlert{}, &model.ChannelPurityAlertAudit{},
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
		"model_comparisons":[{"baseline_model":"gpt-4o","target_model":"gpt-4o"}],
		"sampling":{"window_minutes":30,"minimum_samples":2,"max_samples_per_window":20},
		"policy":{"suspect_threshold":0.8,"alert_threshold":0.6,"alert_windows":4,"recovery_windows":3},
		"retention":{"max_windows_per_target_model":120,"policy":"latest_windows"}
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
	policy := data["policy"].(map[string]any)
	assert.Equal(t, 0.8, policy["suspect_threshold"])
	assert.Equal(t, 0.6, policy["alert_threshold"])
	assert.Equal(t, float64(4), policy["alert_windows"])
	assert.Equal(t, float64(3), policy["recovery_windows"])
	retention := data["retention"].(map[string]any)
	assert.Equal(t, float64(120), retention["max_windows_per_target_model"])
	assert.Equal(t, "latest_windows", retention["policy"])

	run := &model.ChannelPurityPairRun{
		GroupID: groupID, BaselineChannelID: 101, TargetChannelID: 102, ActualModel: "gpt-4o",
		WindowStartedAt: 100, WindowEndedAt: 200, BaselineSampleCount: 3, TargetSampleCount: 3,
		PairedSampleCount: 3, StructureSimilarity: 0.5,
		StructureSimilarityDetail: `{"version":"structure_similarity.v1","method":"multiset_jaccard","window_started_at":100,"window_ended_at":200,"paired_sample_count":3,"matched_count":2,"baseline_only_count":1,"target_only_count":1,"intersection_count":2,"union_count":4,"differences":[{"signature":"shared","baseline_count":2,"target_count":2,"matched_count":2},{"signature":"baseline-only","baseline_count":1,"target_count":0,"matched_count":0},{"signature":"target-only","baseline_count":0,"target_count":1,"matched_count":0}],"field_paths_available":false,"limitation":"Only anonymous structure-signature hashes are retained."}`,
		TokenSimilarity:           0.8,
		BaselineTokenMin:          10, BaselineTokenMax: 20, TargetTokenMin: 11, TargetTokenMax: 22,
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

	latest := purityRequest(t, http.MethodGet, fmt.Sprintf("/api/channel/purity/groups/%d/latest?target_channel_id=102&actual_model=gpt-4o", groupID), "", GetLatestChannelPurityAssessment, gin.Param{Key: "group_id", Value: fmt.Sprint(groupID)})
	require.Equal(t, http.StatusOK, latest.Code, latest.Body.String())
	latestData := decodeEnvelope(t, latest)["data"].(map[string]any)
	assert.Equal(t, 0.5, latestData["structure_similarity"])
	assert.Equal(t, float64(100), latestData["window_started_at"])
	assert.Equal(t, float64(200), latestData["window_ended_at"])
	detail := latestData["structure_similarity_detail"].(map[string]any)
	assert.Equal(t, "structure_similarity.v1", detail["version"])
	assert.Equal(t, float64(3), detail["paired_sample_count"])
	assert.Equal(t, float64(2), detail["matched_count"])
	assert.Equal(t, float64(1), detail["baseline_only_count"])
	assert.Equal(t, float64(1), detail["target_only_count"])
	assert.Equal(t, false, detail["field_paths_available"])

	update := purityRequest(t, http.MethodPut, fmt.Sprintf("/api/channel/purity/groups/%d", groupID), `{
		"name":"api-acceptance-updated","enabled":true,
		"members":[{"channel_id":101,"is_baseline":true},{"channel_id":102,"is_baseline":false}],
		"interval_minutes":10,"model_comparisons":[{"baseline_model":"gpt-4o","target_model":"gpt-4o"}],
		"sampling":{"window_minutes":30,"minimum_samples":3,"max_samples_per_window":30},
		"policy":{"suspect_threshold":0.75,"alert_threshold":0.5,"alert_windows":2,"recovery_windows":4},
		"retention":{"max_windows_per_target_model":80,"policy":"latest_windows"}
	}`, UpdateChannelPurityGroup, gin.Param{Key: "group_id", Value: fmt.Sprint(groupID)})
	require.Equal(t, http.StatusOK, update.Code, update.Body.String())
	updated := decodeEnvelope(t, update)["data"].(map[string]any)
	assert.Equal(t, "api-acceptance-updated", updated["name"])
	assert.Equal(t, float64(10), updated["interval_minutes"])
	updatedPolicy := updated["policy"].(map[string]any)
	assert.Equal(t, 0.75, updatedPolicy["suspect_threshold"])
	assert.Equal(t, float64(2), updatedPolicy["alert_windows"])
	assert.Equal(t, float64(80), updated["retention"].(map[string]any)["max_windows_per_target_model"])

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

func TestClearChannelPurityHistoryKeepsGroupAndRejectsActiveTask(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupPurityAPITestDB(t)
	group := &model.ChannelPurityGroup{Name: "clear-group", Enabled: true, IntervalMinutes: 5, WindowMinutes: 30, MinimumSamples: 1, MaxSamplesPerWindow: 10, Members: []model.ChannelPurityMember{{ChannelID: 1, IsBaseline: true}, {ChannelID: 2}}}
	require.NoError(t, model.CreatePurityGroup(group))
	run := &model.ChannelPurityPairRun{GroupID: group.ID, TargetChannelID: 2, ActualModel: "model", State: model.ChannelPurityStateHealthy, AnomalyEvidenceJSON: "[]", CreatedAt: 1}
	require.NoError(t, model.DB.Create(run).Error)
	require.NoError(t, model.DB.Create(&model.ChannelPurityAssessment{GroupID: group.ID, TargetChannelID: 2, ActualModel: "model", LatestPairRunID: run.ID, State: model.ChannelPurityStateHealthy, FirstSeenAt: 1, UpdatedAt: 1}).Error)

	active, err := model.CreateSystemTask(model.SystemTaskTypeChannelPurityAggregate, channelPurityGroupDetectionPayload{GroupID: group.ID}, nil)
	require.NoError(t, err)
	blocked := purityRequest(t, http.MethodDelete, fmt.Sprintf("/api/channel/purity/groups/%d/history", group.ID), "", ClearChannelPurityHistory, gin.Param{Key: "group_id", Value: fmt.Sprint(group.ID)})
	assert.Equal(t, http.StatusConflict, blocked.Code)
	require.NoError(t, model.DB.Model(active).Updates(map[string]any{"status": model.SystemTaskStatusFailed, "active_key": nil}).Error)

	cleared := purityRequest(t, http.MethodDelete, fmt.Sprintf("/api/channel/purity/groups/%d/history", group.ID), "", ClearChannelPurityHistory, gin.Param{Key: "group_id", Value: fmt.Sprint(group.ID)})
	require.Equal(t, http.StatusOK, cleared.Code, cleared.Body.String())
	_, err = model.GetPurityGroup(group.ID)
	require.NoError(t, err)
	var runs, assessments int64
	require.NoError(t, model.DB.Model(&model.ChannelPurityPairRun{}).Where("group_id = ?", group.ID).Count(&runs).Error)
	require.NoError(t, model.DB.Model(&model.ChannelPurityAssessment{}).Where("group_id = ?", group.ID).Count(&assessments).Error)
	assert.Zero(t, runs)
	assert.Zero(t, assessments)
}

func TestChannelPurityHistorySearchPreviewAndIncidentLifecycle(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupPurityAPITestDB(t)
	require.NoError(t, model.DB.Create(&[]model.Channel{
		{Id: 701, Name: "baseline-api", Models: "gpt-4o", Status: common.ChannelStatusEnabled},
		{Id: 702, Name: "target-api", Models: "gpt-4o-mini", Status: common.ChannelStatusEnabled},
	}).Error)
	group := &model.ChannelPurityGroup{
		Name: "incident-group", Enabled: true, IntervalMinutes: 5, WindowMinutes: 30, MinimumSamples: 2, MaxSamplesPerWindow: 20,
		Members:          []model.ChannelPurityMember{{ChannelID: 701, IsBaseline: true}, {ChannelID: 702}},
		ModelComparisons: []model.ChannelPurityModelComparison{{BaselineModel: "gpt-4o", TargetModel: "gpt-4o-mini"}},
	}
	require.NoError(t, model.CreatePurityGroup(group))
	require.NoError(t, model.DB.Create(&model.ChannelPuritySample{GroupID: group.ID, ChannelID: 702, ActualModel: "gpt-4o-mini", RunKey: "sample", Protocol: "responses", StructureSignature: "sig", Valid: true, ObservedAt: 100}).Error)
	run := &model.ChannelPurityPairRun{
		GroupID: group.ID, BaselineChannelID: 701, TargetChannelID: 702, ActualModel: "gpt-4o-mini",
		BaselineModel: "gpt-4o", TargetModel: "gpt-4o-mini", WindowStartedAt: 100, WindowEndedAt: 200,
		BaselineSampleCount: 3, TargetSampleCount: 3, PairedSampleCount: 2, StructureSimilarity: 0.4,
		TokenSimilarity: 0.5, Confidence: 0.9, State: model.ChannelPurityStateAlert, AnomalyEvidenceJSON: `["structure_distribution_shift"]`, CreatedAt: 200,
	}
	require.NoError(t, model.DB.Create(run).Error)
	assessment := &model.ChannelPurityAssessment{
		GroupID: group.ID, TargetChannelID: 702, ActualModel: "gpt-4o-mini", LatestPairRunID: run.ID,
		State: model.ChannelPurityStateAlert, ConsecutiveAnomalies: 3, Confidence: 0.9, FirstSeenAt: 100, UpdatedAt: 200,
	}
	require.NoError(t, model.DB.Create(assessment).Error)
	alert := &model.ChannelPurityAlert{AssessmentID: assessment.ID, PairRunID: run.ID, Status: "OPEN", EvidenceJSON: `[]`, OpenedAt: 200, UpdatedAt: 200}
	require.NoError(t, model.DB.Create(alert).Error)

	preview := purityRequest(t, http.MethodGet, fmt.Sprintf("/api/channel/purity/groups/%d/history/preview", group.ID), "", GetChannelPurityHistoryPreview, gin.Param{Key: "group_id", Value: fmt.Sprint(group.ID)})
	require.Equal(t, http.StatusOK, preview.Code, preview.Body.String())
	previewData := decodeEnvelope(t, preview)["data"].(map[string]any)
	assert.Equal(t, float64(1), previewData["samples"])
	assert.Equal(t, float64(1), previewData["pair_runs"])
	assert.Equal(t, float64(1), previewData["assessments"])
	assert.Equal(t, float64(1), previewData["alerts"])
	assert.Equal(t, float64(0), previewData["audits"])

	history := purityRequest(t, http.MethodGet, "/api/channel/purity/history?query=target-api&p=1&page_size=20", "", ListAllChannelPurityHistory)
	require.Equal(t, http.StatusOK, history.Code, history.Body.String())
	historyData := decodeEnvelope(t, history)["data"].(map[string]any)
	assert.Equal(t, float64(1), historyData["total"])
	items := historyData["items"].([]any)
	require.Len(t, items, 1)
	assert.Equal(t, "incident-group", items[0].(map[string]any)["group_name"])
	assert.Equal(t, "target-api", items[0].(map[string]any)["target_channel_name"])

	note := purityRequest(t, http.MethodPost, fmt.Sprintf("/api/channel/purity/groups/%d/alerts/%d/actions", group.ID, alert.ID), `{"action":"note","note":"checked upstream mapping"}`, UpdateChannelPurityIncident,
		gin.Param{Key: "group_id", Value: fmt.Sprint(group.ID)}, gin.Param{Key: "alert_id", Value: fmt.Sprint(alert.ID)})
	require.Equal(t, http.StatusOK, note.Code, note.Body.String())
	assert.Equal(t, "checked upstream mapping", decodeEnvelope(t, note)["data"].(map[string]any)["note"])

	acknowledge := purityRequest(t, http.MethodPost, fmt.Sprintf("/api/channel/purity/groups/%d/alerts/%d/actions", group.ID, alert.ID), `{"action":"acknowledge"}`, UpdateChannelPurityIncident,
		gin.Param{Key: "group_id", Value: fmt.Sprint(group.ID)}, gin.Param{Key: "alert_id", Value: fmt.Sprint(alert.ID)})
	require.Equal(t, http.StatusOK, acknowledge.Code, acknowledge.Body.String())
	acknowledged := decodeEnvelope(t, acknowledge)["data"].(map[string]any)
	assert.Equal(t, "ACKNOWLEDGED", acknowledged["status"])
	require.Len(t, acknowledged["audit"].([]any), 2)

	preview = purityRequest(t, http.MethodGet, fmt.Sprintf("/api/channel/purity/groups/%d/history/preview", group.ID), "", GetChannelPurityHistoryPreview, gin.Param{Key: "group_id", Value: fmt.Sprint(group.ID)})
	require.Equal(t, http.StatusOK, preview.Code, preview.Body.String())
	assert.Equal(t, float64(2), decodeEnvelope(t, preview)["data"].(map[string]any)["audits"])
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

func TestChannelPurityFormalDetectionRejectsDisabledBaselineAndTarget(t *testing.T) {
	setupPurityAPITestDB(t)
	channels := []model.Channel{
		{Id: 301, Name: "baseline", Models: "gpt-4o", Status: common.ChannelStatusEnabled},
		{Id: 302, Name: "target", Models: "gpt-4o", Status: common.ChannelStatusEnabled},
	}
	require.NoError(t, model.DB.Create(&channels).Error)
	group := &model.ChannelPurityGroup{ID: 41, Members: []model.ChannelPurityMember{
		{ChannelID: 301, IsBaseline: true}, {ChannelID: 302},
	}}

	require.NoError(t, model.DB.Model(&model.Channel{}).Where("id = ?", 301).Update("status", common.ChannelStatusManuallyDisabled).Error)
	pairs, err := runPurityGroupDetection(context.Background(), group, 1)
	require.Error(t, err)
	assert.Zero(t, pairs)
	assert.Contains(t, err.Error(), "baseline channel 301 is disabled")

	require.NoError(t, model.DB.Model(&model.Channel{}).Where("id = ?", 301).Update("status", common.ChannelStatusEnabled).Error)
	require.NoError(t, model.DB.Model(&model.Channel{}).Where("id = ?", 302).Update("status", common.ChannelStatusAutoDisabled).Error)
	pairs, err = runPurityGroupDetection(context.Background(), group, 1)
	require.Error(t, err)
	assert.Zero(t, pairs)
	assert.Contains(t, err.Error(), "target channels are disabled: [302]")
}

func TestChannelPurityQuickProbeRejectsDisabledChannel(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupPurityAPITestDB(t)
	require.NoError(t, model.DB.Create(&model.Channel{Id: 401, Name: "disabled", Models: "gpt-4o", Status: common.ChannelStatusManuallyDisabled}).Error)

	recorder := purityRequest(t, http.MethodPost, "/api/channel/purity/quick-probe", `{"channel_id":401}`, RunChannelPurityQuickProbe)
	require.Equal(t, http.StatusBadRequest, recorder.Code, recorder.Body.String())
	response := decodeEnvelope(t, recorder)
	assert.Equal(t, false, response["success"])
	assert.Contains(t, response["message"], "disabled")
}

func TestChannelPurityOldGroupWithDisabledOrMissingMembersRemainsReadable(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupPurityAPITestDB(t)
	require.NoError(t, model.DB.Create(&model.Channel{Id: 501, Name: "old-disabled", Models: "gpt-4o", Status: common.ChannelStatusManuallyDisabled}).Error)
	group := &model.ChannelPurityGroup{
		Name: "legacy-visible", Enabled: true, IntervalMinutes: 5, WindowMinutes: 30, MinimumSamples: 1, MaxSamplesPerWindow: 10,
		Members: []model.ChannelPurityMember{{ChannelID: 501, IsBaseline: true}, {ChannelID: 599}},
	}
	require.NoError(t, model.CreatePurityGroup(group))

	recorder := purityRequest(t, http.MethodGet, fmt.Sprintf("/api/channel/purity/groups/%d", group.ID), "", GetChannelPurityGroup, gin.Param{Key: "group_id", Value: fmt.Sprint(group.ID)})
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	data := decodeEnvelope(t, recorder)["data"].(map[string]any)
	assert.ElementsMatch(t, []any{float64(501), float64(599)}, data["channel_ids"].([]any))
	assert.Equal(t, "old-disabled", data["baseline_channel_name"])
}

func TestChannelPurityCreateRejectsDisabledMember(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupPurityAPITestDB(t)
	require.NoError(t, model.DB.Create(&[]model.Channel{
		{Id: 601, Name: "enabled", Models: "gpt-4o", Status: common.ChannelStatusEnabled},
		{Id: 602, Name: "disabled", Models: "gpt-4o", Status: common.ChannelStatusManuallyDisabled},
	}).Error)

	recorder := purityRequest(t, http.MethodPost, "/api/channel/purity/groups", `{
		"name":"invalid-disabled","enabled":true,"channel_ids":[601,602],"baseline_channel_id":601,
		"interval_minutes":5,"sampling":{"window_minutes":30,"minimum_samples":1,"max_samples_per_window":10}
	}`, CreateChannelPurityGroup)
	require.Equal(t, http.StatusBadRequest, recorder.Code, recorder.Body.String())
	assert.Contains(t, decodeEnvelope(t, recorder)["message"], "disabled")
}
