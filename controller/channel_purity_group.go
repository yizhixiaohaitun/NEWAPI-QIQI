package controller

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	channelpurity "github.com/QuantumNous/new-api/service/channel_purity"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func purityGroupFromRequest(r dto.ChannelPurityGroupRequest) *model.ChannelPurityGroup {
	g := &model.ChannelPurityGroup{
		Name: r.Name, Enabled: r.Enabled, IntervalMinutes: r.IntervalMinutes,
		RandomPairingEnabled: r.RandomPairingEnabled,
		WindowMinutes:        r.Sampling.WindowMinutes, MinimumSamples: r.Sampling.MinimumSamples,
		MaxSamplesPerWindow: r.Sampling.MaxSamplesPerWindow,
		SuspectThreshold:    r.Policy.SuspectThreshold, AlertThreshold: r.Policy.AlertThreshold,
		AlertWindows: r.Policy.AlertWindows, RecoveryWindows: r.Policy.RecoveryWindows,
		RetentionWindows: r.Retention.MaxWindowsPerTargetModel,
	}
	members := r.Members
	if len(members) == 0 {
		members = make([]dto.ChannelPurityGroupMemberRequest, 0, len(r.ChannelIDs))
		for _, channelID := range r.ChannelIDs {
			members = append(members, dto.ChannelPurityGroupMemberRequest{ChannelID: channelID, IsBaseline: channelID == r.BaselineChannelID})
		}
	}
	g.Members = make([]model.ChannelPurityMember, len(members))
	for i, member := range members {
		g.Members[i] = model.ChannelPurityMember{ChannelID: member.ChannelID, IsBaseline: member.IsBaseline}
	}
	g.ModelComparisons = make([]model.ChannelPurityModelComparison, len(r.ModelComparisons))
	for i, comparison := range r.ModelComparisons {
		g.ModelComparisons[i] = model.ChannelPurityModelComparison{BaselineModel: comparison.BaselineModel, TargetModel: comparison.TargetModel}
	}
	return g
}

func validatePurityGroupChannelsEnabled(group *model.ChannelPurityGroup) error {
	if err := model.ValidateChannelPurityGroup(group); err != nil {
		return err
	}
	disabledIDs := make([]int, 0)
	missingIDs := make([]int, 0)
	for _, member := range group.Members {
		channel, err := model.GetChannelById(member.ChannelID, true)
		if err != nil || channel == nil {
			missingIDs = append(missingIDs, member.ChannelID)
			continue
		}
		if channel.Status != common.ChannelStatusEnabled {
			disabledIDs = append(disabledIDs, member.ChannelID)
		}
	}
	if len(missingIDs) > 0 {
		return fmt.Errorf("purity group channels do not exist: %v", missingIDs)
	}
	if len(disabledIDs) > 0 {
		return fmt.Errorf("purity group channels are disabled: %v", disabledIDs)
	}
	if len(group.ModelComparisons) == 0 {
		return fmt.Errorf("at least one model comparison is required")
	}
	var baseline *model.Channel
	targets := make([]*model.Channel, 0, len(group.Members)-1)
	for _, member := range group.Members {
		channel, _ := model.GetChannelById(member.ChannelID, true)
		if member.IsBaseline {
			baseline = channel
		} else {
			targets = append(targets, channel)
		}
	}
	baselineModels := channelModelSet(baseline)
	for _, comparison := range group.ModelComparisons {
		if !baselineModels[comparison.BaselineModel] {
			return fmt.Errorf("baseline model %q is not available on baseline channel %d", comparison.BaselineModel, baseline.Id)
		}
		for _, target := range targets {
			if !channelModelSet(target)[comparison.TargetModel] {
				return fmt.Errorf("target model %q is not available on target channel %d", comparison.TargetModel, target.Id)
			}
		}
	}
	return nil
}

func CreateChannelPurityGroup(c *gin.Context) {
	var request dto.ChannelPurityGroupRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid group"})
		return
	}
	group := purityGroupFromRequest(request)
	now := time.Now().Unix()
	group.CreatedAt, group.UpdatedAt = now, now
	if group.Enabled {
		group.NextRunAt = now
	}
	if err := validatePurityGroupChannelsEnabled(group); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": err.Error()})
		return
	}
	if err := model.CreatePurityGroup(group); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": err.Error()})
		return
	}
	response, err := buildPurityGroupResponse(group)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"success": true, "data": response})
}

func ListChannelPurityGroups(c *gin.Context) {
	groups, err := model.ListPurityGroups()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	responses := make([]gin.H, 0, len(groups))
	for i := range groups {
		response, buildErr := buildPurityGroupResponse(&groups[i])
		if buildErr != nil {
			common.ApiError(c, buildErr)
			return
		}
		responses = append(responses, response)
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": responses})
}

func GetChannelPurityGroup(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("group_id"), 10, 64)
	if err != nil || id == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid group id"})
		return
	}
	group, err := model.GetPurityGroup(uint(id))
	if err != nil {
		purityGroupLookup(c, err)
		return
	}
	response, err := buildPurityGroupResponse(group)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": response})
}

func UpdateChannelPurityGroup(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("group_id"), 10, 64)
	if err != nil || id == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid group id"})
		return
	}
	var request dto.ChannelPurityGroupRequest
	if err = c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid group"})
		return
	}
	group := purityGroupFromRequest(request)
	group.ID = uint(id)
	group.UpdatedAt = time.Now().Unix()
	if group.Enabled {
		group.NextRunAt = group.UpdatedAt
	}
	if err = validatePurityGroupChannelsEnabled(group); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": err.Error()})
		return
	}
	if err = model.UpdatePurityGroup(group); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": err.Error()})
		return
	}
	group, err = model.GetPurityGroup(group.ID)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	response, err := buildPurityGroupResponse(group)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": response})
}

func DeleteChannelPurityGroup(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("group_id"), 10, 64)
	if err != nil || id == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid group id"})
		return
	}
	if err = model.DeletePurityGroup(uint(id)); err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

func CreateChannelPuritySample(c *gin.Context) {
	var request dto.ChannelPuritySampleRequest
	if c.ShouldBindJSON(&request) != nil || request.GroupID == 0 || request.ChannelID <= 0 || strings.TrimSpace(request.ActualModel) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "group_id, channel_id and actual_model are required"})
		return
	}
	group, err := model.GetPurityGroup(request.GroupID)
	if err != nil {
		purityGroupLookup(c, err)
		return
	}
	member := false
	for _, value := range group.Members {
		if value.ChannelID == request.ChannelID {
			member = true
			break
		}
	}
	if !member {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "channel is not a member of group"})
		return
	}
	if request.ObservedAt == 0 {
		request.ObservedAt = time.Now().Unix()
	}
	profileJSON := ""
	if len(request.StructureProfile) > 0 {
		profile := make([]dto.ChannelPurityFieldProfile, 0, min(len(request.StructureProfile), 200))
		seen := map[string]bool{}
		for _, field := range request.StructureProfile {
			field.Path, field.Type = strings.TrimSpace(field.Path), strings.TrimSpace(field.Type)
			if field.Path == "" || len(field.Path) > 256 || len(field.Type) > 64 || seen[field.Path+"\x00"+field.Type] {
				continue
			}
			seen[field.Path+"\x00"+field.Type] = true
			profile = append(profile, field)
			if len(profile) == 200 {
				break
			}
		}
		if encoded, marshalErr := common.Marshal(profile); marshalErr == nil {
			profileJSON = string(encoded)
		}
	}
	sample := &model.ChannelPuritySample{
		GroupID: request.GroupID, ChannelID: request.ChannelID, ActualModel: strings.TrimSpace(request.ActualModel),
		RunKey: fmt.Sprintf("external-%d", request.ObservedAt), Protocol: "external", StructureSignature: request.StructureSignature,
		StructureProfileJSON: profileJSON,
		PromptTokens:         request.PromptTokens, CompletionTokens: request.CompletionTokens, TotalTokens: request.TotalTokens,
		Valid: request.Valid, ErrorClass: request.ErrorClass, ObservedAt: request.ObservedAt,
	}
	if err = model.CreatePuritySample(sample); err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"success": true, "data": sample})
}

func RunChannelPurityQuickProbe(c *gin.Context) {
	var request struct {
		ChannelID int    `json:"channel_id"`
		Model     string `json:"model"`
	}
	if c.ShouldBindJSON(&request) != nil || request.ChannelID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "channel_id is required"})
		return
	}
	channel, err := model.GetChannelById(request.ChannelID, true)
	if err != nil {
		writeChannelPurityLookupError(c, err, "channel not found")
		return
	}
	if channel.Status != common.ChannelStatusEnabled {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": fmt.Sprintf("channel %d is disabled and cannot run Quick Probe", request.ChannelID)})
		return
	}
	modelName := strings.TrimSpace(request.Model)
	if modelName == "" {
		models := channel.GetModels()
		if len(models) > 0 {
			modelName = strings.TrimSpace(models[0])
		}
	}
	if modelName == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "channel has no configured model"})
		return
	}
	userID, err := resolveChannelTestUserID(c)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 60*time.Second)
	defer cancel()
	probe := testChannelWithOptions(ctx, channel, userID, modelName, purityEndpointType(channel, modelName), shouldUseStreamForAutomaticChannelTest(channel), channelTestOptions{
		recordConsumeLog: false, captureResponse: true, allowMissingUsage: true, purityDetectionRequest: true,
	})
	ok := probe.localErr == nil && probe.newAPIError == nil
	message := "Connectivity probe succeeded; it is excluded from formal purity evidence"
	if !ok {
		message = "Connectivity probe failed"
		if probe.localErr != nil {
			message += ": " + probe.localErr.Error()
		}
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"ok": ok, "latency_ms": probe.latencyMS, "message": message, "checked_at": time.Now().Unix()}})
}

func GetLatestChannelPurityAssessment(c *gin.Context) {
	groupID, err := strconv.ParseUint(c.Param("group_id"), 10, 64)
	targetID, targetErr := strconv.Atoi(c.Query("target_channel_id"))
	actualModel := strings.TrimSpace(c.Query("actual_model"))
	if err != nil || targetErr != nil || groupID == 0 || targetID <= 0 || actualModel == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "group_id, target_channel_id and actual_model are required"})
		return
	}
	value, err := model.GetLatestPurityAssessment(uint(groupID), targetID, actualModel)
	if err != nil {
		purityGroupLookup(c, err)
		return
	}
	run, err := model.GetPurityPairRun(value.LatestPairRunID)
	if err != nil {
		purityGroupLookup(c, err)
		return
	}
	structureDetail, err := channelpurity.DecodeStructureSimilarityDetail(run)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	group, err := model.GetPurityGroup(uint(groupID))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	alerts, _ := model.ListOpenPurityAlerts(value.ID)
	incidents := make([]gin.H, 0, len(alerts))
	for i := range alerts {
		incidents = append(incidents, purityIncidentResponse(&alerts[i]))
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{
		"id": value.ID, "group_id": value.GroupID, "target_channel_id": value.TargetChannelID,
		"actual_model": value.ActualModel, "latest_pair_run_id": value.LatestPairRunID,
		"state": value.State, "confidence": value.Confidence, "updated_at": value.UpdatedAt,
		"pair_run":                    purityPairRunResponse(run, structureDetail),
		"window_started_at":           run.WindowStartedAt,
		"window_ended_at":             run.WindowEndedAt,
		"structure_similarity":        run.StructureSimilarity,
		"structure_similarity_detail": structureDetail,
		"evidence":                    purityPairRunEvidence(run),
		"explanation":                 purityStatusExplanation(value, run, group),
		"incidents":                   incidents,
	}})
}

func purityPairRunEvidence(run *model.ChannelPurityPairRun) []gin.H {
	var reasons []string
	_ = common.Unmarshal([]byte(run.AnomalyEvidenceJSON), &reasons)
	items := make([]gin.H, 0, len(reasons))
	for i, reason := range reasons {
		items = append(items, gin.H{"id": fmt.Sprintf("%d-%d", run.ID, i), "occurred_at": run.CreatedAt, "kind": reason, "summary": purityEvidenceSummary(reason)})
	}
	return items
}

func purityPairRunResponse(run *model.ChannelPurityPairRun, detail *channelpurity.StructureSimilarityDetail) gin.H {
	return gin.H{
		"id": run.ID, "group_id": run.GroupID, "baseline_channel_id": run.BaselineChannelID, "target_channel_id": run.TargetChannelID,
		"actual_model": run.ActualModel, "baseline_model": run.BaselineModel, "target_model": run.TargetModel,
		"window_started_at": run.WindowStartedAt, "window_ended_at": run.WindowEndedAt,
		"baseline_sample_count": run.BaselineSampleCount, "target_sample_count": run.TargetSampleCount, "paired_sample_count": run.PairedSampleCount,
		"baseline_invalid_count": run.BaselineInvalidCount, "target_invalid_count": run.TargetInvalidCount,
		"unmatched_baseline_count": run.UnmatchedBaselineCount, "unmatched_target_count": run.UnmatchedTargetCount,
		"structure_similarity": run.StructureSimilarity, "structure_similarity_detail": detail,
		"token_similarity": run.TokenSimilarity, "baseline_token_min": run.BaselineTokenMin, "baseline_token_max": run.BaselineTokenMax,
		"target_token_min": run.TargetTokenMin, "target_token_max": run.TargetTokenMax, "token_deviation_rate": run.TokenDeviationRate,
		"confidence": run.Confidence, "state": run.State, "error_class": run.ErrorClass, "evidence": purityPairRunEvidence(run), "created_at": run.CreatedAt,
	}
}

func ListChannelPurityHistory(c *gin.Context) {
	groupID, err := strconv.ParseUint(c.Param("group_id"), 10, 64)
	targetID, targetErr := strconv.Atoi(c.Query("target_channel_id"))
	actualModel := strings.TrimSpace(c.Query("actual_model"))
	if err != nil || targetErr != nil || groupID == 0 || targetID <= 0 || actualModel == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "group_id, target_channel_id and actual_model are required"})
		return
	}
	page := common.GetPageQuery(c)
	values, total, err := model.ListPurityPairRuns(uint(groupID), targetID, actualModel, page.GetStartIdx(), page.GetPageSize())
	if err != nil {
		common.ApiError(c, err)
		return
	}
	items := make([]gin.H, 0, len(values))
	for i := range values {
		detail, decodeErr := channelpurity.DecodeStructureSimilarityDetail(&values[i])
		if decodeErr != nil {
			common.ApiError(c, decodeErr)
			return
		}
		items = append(items, purityPairRunResponse(&values[i], detail))
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"items": items, "total": total, "page": page.GetPage(), "page_size": page.GetPageSize()}})
}

func ListAllChannelPurityHistory(c *gin.Context) {
	page := common.GetPageQuery(c)
	var groupID uint
	if raw := strings.TrimSpace(c.Query("group_id")); raw != "" {
		parsed, err := strconvParseGroupID(raw)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid group id"})
			return
		}
		groupID = parsed
	}
	values, total, err := model.ListPurityHistory(groupID, strings.TrimSpace(c.Query("status")), strings.TrimSpace(c.Query("query")), page.GetStartIdx(), page.GetPageSize())
	if err != nil {
		common.ApiError(c, err)
		return
	}
	items := make([]gin.H, 0, len(values))
	groups := map[uint]string{}
	channels := map[int]string{}
	for i := range values {
		run := &values[i]
		if _, ok := groups[run.GroupID]; !ok {
			if group, e := model.GetPurityGroup(run.GroupID); e == nil {
				groups[run.GroupID] = group.Name
			}
		}
		if _, ok := channels[run.TargetChannelID]; !ok {
			if channel, e := model.GetChannelById(run.TargetChannelID, true); e == nil && channel != nil {
				channels[run.TargetChannelID] = channel.Name
			}
		}
		items = append(items, gin.H{"id": run.ID, "group_id": run.GroupID, "group_name": fallbackChannelName(map[int]string{int(run.GroupID): groups[run.GroupID]}, int(run.GroupID)),
			"target_channel_id": run.TargetChannelID, "target_channel_name": fallbackChannelName(channels, run.TargetChannelID),
			"baseline_model": run.BaselineModel, "target_model": run.TargetModel, "state": run.State,
			"paired_sample_count": run.PairedSampleCount, "structure_similarity": run.StructureSimilarity,
			"token_similarity": run.TokenSimilarity, "confidence": run.Confidence, "window_ended_at": run.WindowEndedAt})
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"items": items, "total": total, "page": page.GetPage(), "page_size": page.GetPageSize()}})
}

func GetChannelPurityHistoryPreview(c *gin.Context) {
	groupID, err := strconvParseGroupID(c.Param("group_id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid group id"})
		return
	}
	if _, err = model.GetPurityGroup(groupID); err != nil {
		purityGroupLookup(c, err)
		return
	}
	samples, runs, assessments, alerts, audits, err := model.PurityHistoryPreview(groupID)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"samples": samples, "pair_runs": runs, "assessments": assessments, "alerts": alerts, "audits": audits}})
}

func UpdateChannelPurityIncident(c *gin.Context) {
	groupID, err := strconvParseGroupID(c.Param("group_id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid group id"})
		return
	}
	alertID64, err := strconv.ParseUint(c.Param("alert_id"), 10, 64)
	if err != nil || alertID64 == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid alert id"})
		return
	}
	var request struct {
		Action       string `json:"action"`
		Note         string `json:"note"`
		SilenceUntil int64  `json:"silence_until"`
	}
	if c.ShouldBindJSON(&request) != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid action"})
		return
	}
	request.Action, request.Note = strings.ToLower(strings.TrimSpace(request.Action)), strings.TrimSpace(request.Note)
	alert, err := model.GetPurityAlertForGroup(groupID, uint(alertID64))
	if err != nil {
		purityGroupLookup(c, err)
		return
	}
	now := time.Now().Unix()
	switch request.Action {
	case "acknowledge":
		alert.Status, alert.AcknowledgedAt = "ACKNOWLEDGED", now
	case "silence":
		if request.SilenceUntil <= now || request.SilenceUntil > now+30*86400 {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "silence_until must be within the next 30 days"})
			return
		}
		alert.Status, alert.SilenceUntil = "SILENCED", request.SilenceUntil
	case "note":
		if request.Note == "" {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "note is required"})
			return
		}
		alert.Note = request.Note
	case "false_positive":
		alert.Status, alert.FalsePositiveAt, alert.ResolvedAt = "FALSE_POSITIVE", now, now
		if request.Note != "" {
			alert.Note = request.Note
		}
	case "resolve":
		alert.Status, alert.ResolvedAt = "RESOLVED", now
	default:
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "unsupported incident action"})
		return
	}
	alert.UpdatedAt = now
	if err = model.UpdatePurityAlertAction(alert, request.Action, request.Note, now); err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": purityIncidentResponse(alert)})
}

func ClearChannelPurityHistory(c *gin.Context) {
	groupID, err := strconvParseGroupID(c.Param("group_id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid group id"})
		return
	}
	if err := model.ClearPurityGroupHistory(groupID); err != nil {
		if errors.Is(err, model.ErrPurityGroupDetectionRunning) {
			c.JSON(http.StatusConflict, gin.H{"success": false, "message": err.Error()})
			return
		}
		purityGroupLookup(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "channel purity history cleared"})
}

func purityStatusExplanation(assessment *model.ChannelPurityAssessment, run *model.ChannelPurityPairRun, group *model.ChannelPurityGroup) gin.H {
	summary, action := "当前窗口正在积累证据", "继续观察后续检测窗口"
	switch assessment.State {
	case model.ChannelPurityStateHealthy:
		summary, action = "当前结构与 Token 分布未发现明显异常", "保持自动检测"
	case model.ChannelPurityStateSuspect:
		summary, action = "相似度已低于可疑阈值，但尚未满足连续告警条件", "查看证据和趋势，并核对目标渠道配置"
	case model.ChannelPurityStateAlert:
		summary, action = "连续异常已达到告警条件", "核查目标渠道上游、模型映射与响应格式"
	case model.ChannelPurityStateLowSample, model.ChannelPurityStateWarmingUp:
		summary, action = "配对样本尚不足以形成稳定判断", "等待更多流量或手动检测"
	case model.ChannelPurityStateNoTraffic:
		summary, action = "当前窗口没有可比较的有效流量", "检查渠道路由和实际调用"
	case model.ChannelPurityStateBaselineUnavailable:
		summary, action = "基准渠道没有可用样本", "优先检查基准渠道"
	case model.ChannelPurityStateDetectorError:
		summary, action = "检测链路执行失败", "查看错误类别后重试"
	}
	return gin.H{"code": assessment.State, "summary": summary, "suggested_action": action,
		"combined_similarity": run.StructureSimilarity*.65 + run.TokenSimilarity*.35,
		"suspect_threshold":   group.SuspectThreshold, "alert_threshold": group.AlertThreshold,
		"consecutive_anomalies": assessment.ConsecutiveAnomalies, "consecutive_healthy": assessment.ConsecutiveHealthy,
		"baseline_available": run.BaselineSampleCount > 0}
}

func purityIncidentResponse(alert *model.ChannelPurityAlert) gin.H {
	audits, _ := model.ListPurityAlertAudits(alert.ID)
	return gin.H{"id": alert.ID, "status": alert.Status, "note": alert.Note, "silence_until": alert.SilenceUntil,
		"opened_at": alert.OpenedAt, "resolved_at": alert.ResolvedAt, "audit": audits}
}

func buildPurityGroupResponse(group *model.ChannelPurityGroup) (gin.H, error) {
	channelIDs := make([]int, 0, len(group.Members))
	baselineID := 0
	channelNames := map[int]string{}
	for _, member := range group.Members {
		channelIDs = append(channelIDs, member.ChannelID)
		if member.IsBaseline {
			baselineID = member.ChannelID
		}
		channel, err := model.GetChannelById(member.ChannelID, true)
		if err == nil && channel != nil {
			channelNames[member.ChannelID] = channel.Name
		}
	}
	assessments, err := model.ListPurityAssessments(group.ID)
	if err != nil {
		return nil, err
	}
	results := make([]gin.H, 0, len(assessments))
	for _, assessment := range assessments {
		run, runErr := model.GetPurityPairRun(assessment.LatestPairRunID)
		if runErr != nil {
			continue
		}
		var reasonCodes []string
		_ = common.Unmarshal([]byte(run.AnomalyEvidenceJSON), &reasonCodes)
		evidence := make([]gin.H, 0, len(reasonCodes))
		for i, reason := range reasonCodes {
			evidence = append(evidence, gin.H{"id": fmt.Sprintf("%d-%d", run.ID, i), "occurred_at": run.CreatedAt, "kind": reason, "summary": purityEvidenceSummary(reason)})
		}
		history, _ := model.ListRecentPurityPairRuns(group.ID, assessment.TargetChannelID, assessment.ActualModel, 20)
		trend := make([]gin.H, 0, len(history))
		for i := len(history) - 1; i >= 0; i-- {
			item := history[i]
			trend = append(trend, gin.H{"at": item.WindowEndedAt, "status": item.State, "field_similarity": item.StructureSimilarity, "token_similarity": item.TokenSimilarity, "confidence": item.Confidence})
		}
		alerts, _ := model.ListOpenPurityAlerts(assessment.ID)
		alertMessages := make([]string, 0, len(alerts))
		incidents := make([]gin.H, 0, len(alerts))
		for i := range alerts {
			alertMessages = append(alertMessages, "Repeated independent evidence is outside the baseline interval")
			incidents = append(incidents, purityIncidentResponse(&alerts[i]))
		}
		baselineModel, targetModel := run.BaselineModel, run.TargetModel
		if baselineModel == "" && targetModel == "" {
			baselineModel, targetModel = run.ActualModel, run.ActualModel
		}
		result := gin.H{
			"id": assessment.ID, "target_channel_id": assessment.TargetChannelID, "target_channel_name": fallbackChannelName(channelNames, assessment.TargetChannelID),
			"baseline_channel_id": baselineID, "baseline_channel_name": fallbackChannelName(channelNames, baselineID), "model": assessment.ActualModel,
			"baseline_model": baselineModel, "target_model": targetModel,
			"status": assessment.State, "samples": run.PairedSampleCount, "field_similarity": run.StructureSimilarity,
			"token_similarity": run.TokenSimilarity, "confidence": assessment.Confidence,
			"baseline_token_range": gin.H{"min": run.BaselineTokenMin, "max": run.BaselineTokenMax},
			"target_token_range":   gin.H{"min": run.TargetTokenMin, "max": run.TargetTokenMax}, "deviation_rate": run.TokenDeviationRate,
			"evidence": evidence, "alerts": alertMessages, "incidents": incidents,
			"explanation": purityStatusExplanation(&assessment, run, group), "trend": trend, "updated_at": assessment.UpdatedAt,
		}
		if len(evidence) > 0 {
			result["latest_evidence"] = evidence[0]
		}
		results = append(results, result)
	}
	return gin.H{
		"id": group.ID, "name": group.Name, "enabled": group.Enabled, "channel_ids": channelIDs, "baseline_channel_id": baselineID,
		"baseline_channel_name": fallbackChannelName(channelNames, baselineID), "interval_minutes": group.IntervalMinutes,
		"random_pairing_enabled":     group.RandomPairingEnabled,
		"model_comparisons":          group.ModelComparisons,
		"model_comparisons_required": len(group.ModelComparisons) == 0,
		"sampling":                   gin.H{"window_minutes": group.WindowMinutes, "minimum_samples": group.MinimumSamples, "max_samples_per_window": group.MaxSamplesPerWindow},
		"policy":                     gin.H{"suspect_threshold": group.SuspectThreshold, "alert_threshold": group.AlertThreshold, "alert_windows": group.AlertWindows, "recovery_windows": group.RecoveryWindows},
		"retention":                  gin.H{"max_windows_per_target_model": group.RetentionWindows, "policy": "latest_windows"},
		"results":                    results, "last_run_at": group.LastRunAt, "next_run_at": group.NextRunAt, "last_error": group.LastError,
		"created_at": group.CreatedAt, "updated_at": group.UpdatedAt,
	}, nil
}

func fallbackChannelName(names map[int]string, id int) string {
	if name := names[id]; name != "" {
		return name
	}
	return fmt.Sprintf("#%d", id)
}

func purityEvidenceSummary(code string) string {
	switch code {
	case "structure_distribution_shift":
		return "Anonymous response structure differs repeatedly from the designated baseline"
	case "token_interval_shift":
		return "Paired token usage is outside the baseline-aligned interval"
	case "missing_comparable_samples":
		return "Comparable baseline and target samples are not yet available"
	case "protocol_mismatch":
		return "The upstream response protocol differs from the designated baseline"
	default:
		return code
	}
}

func purityGroupLookup(c *gin.Context, err error) {
	if err == gorm.ErrRecordNotFound {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "not found"})
	} else {
		common.ApiError(c, err)
	}
}
