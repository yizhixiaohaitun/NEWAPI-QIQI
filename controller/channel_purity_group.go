package controller

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func purityGroupFromRequest(r dto.ChannelPurityGroupRequest) *model.ChannelPurityGroup {
	g := &model.ChannelPurityGroup{
		Name: r.Name, Enabled: r.Enabled, IntervalMinutes: r.IntervalMinutes,
		RandomPairingEnabled: r.RandomPairingEnabled,
		WindowMinutes:        r.Sampling.WindowMinutes, MinimumSamples: r.Sampling.MinimumSamples,
		MaxSamplesPerWindow: r.Sampling.MaxSamplesPerWindow,
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
	return g
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
	sample := &model.ChannelPuritySample{
		GroupID: request.GroupID, ChannelID: request.ChannelID, ActualModel: strings.TrimSpace(request.ActualModel),
		RunKey: fmt.Sprintf("external-%d", request.ObservedAt), Protocol: "external", StructureSignature: request.StructureSignature,
		PromptTokens: request.PromptTokens, CompletionTokens: request.CompletionTokens, TotalTokens: request.TotalTokens,
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
	c.JSON(http.StatusOK, gin.H{"success": true, "data": value})
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
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"items": values, "total": total, "page": page.GetPage(), "page_size": page.GetPageSize()}})
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
		for range alerts {
			alertMessages = append(alertMessages, "Repeated independent evidence is outside the baseline interval")
		}
		result := gin.H{
			"id": assessment.ID, "target_channel_id": assessment.TargetChannelID, "target_channel_name": fallbackChannelName(channelNames, assessment.TargetChannelID),
			"baseline_channel_id": baselineID, "baseline_channel_name": fallbackChannelName(channelNames, baselineID), "model": assessment.ActualModel,
			"status": assessment.State, "samples": run.PairedSampleCount, "field_similarity": run.StructureSimilarity,
			"token_similarity": run.TokenSimilarity, "confidence": assessment.Confidence,
			"baseline_token_range": gin.H{"min": run.BaselineTokenMin, "max": run.BaselineTokenMax},
			"target_token_range":   gin.H{"min": run.TargetTokenMin, "max": run.TargetTokenMax}, "deviation_rate": run.TokenDeviationRate,
			"evidence": evidence, "alerts": alertMessages, "trend": trend, "updated_at": assessment.UpdatedAt,
		}
		if len(evidence) > 0 {
			result["latest_evidence"] = evidence[0]
		}
		results = append(results, result)
	}
	return gin.H{
		"id": group.ID, "name": group.Name, "enabled": group.Enabled, "channel_ids": channelIDs, "baseline_channel_id": baselineID,
		"baseline_channel_name": fallbackChannelName(channelNames, baselineID), "interval_minutes": group.IntervalMinutes,
		"random_pairing_enabled": group.RandomPairingEnabled,
		"sampling":               gin.H{"window_minutes": group.WindowMinutes, "minimum_samples": group.MinimumSamples, "max_samples_per_window": group.MaxSamplesPerWindow},
		"results":                results, "last_run_at": group.LastRunAt, "next_run_at": group.NextRunAt, "last_error": group.LastError,
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
