package controller

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"runtime/debug"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
	"gorm.io/gorm"
)

const channelPurityMaxConcurrent = 2

var channelPuritySlots = make(chan struct{}, channelPurityMaxConcurrent)

func StartChannelPurityScan(c *gin.Context) {
	var request dto.ChannelPurityQuickScanRequest
	if err := c.ShouldBindJSON(&request); err != nil || request.ChannelID <= 0 || strings.TrimSpace(request.Model) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "channel_id and model are required"})
		return
	}
	request.Model = strings.TrimSpace(request.Model)
	channel, err := model.GetChannelById(request.ChannelID, true)
	if err != nil {
		writeChannelPurityLookupError(c, err, "channel not found")
		return
	}
	if !channelHasModel(channel.Models, request.Model) {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "model is not configured on this channel"})
		return
	}
	select {
	case channelPuritySlots <- struct{}{}:
	default:
		c.JSON(http.StatusTooManyRequests, gin.H{"success": false, "message": "channel purity scan concurrency limit reached"})
		return
	}

	now := time.Now().Unix()
	scan := &model.ChannelPurityScan{
		ChannelID: channel.Id, ChannelName: channel.Name, RequestedModel: request.Model, Protocol: "openai_chat",
		Status: model.ChannelPurityStatusPending, Conclusion: model.ChannelPurityConclusionUnknown,
		Risk: model.ChannelPurityRiskUnknown, Summary: "Quick probe is queued", CreatedBy: c.GetInt("id"), CreatedAt: now,
	}
	if err := model.CreateChannelPurityScan(scan); err != nil {
		<-channelPuritySlots
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "failed to create scan"})
		return
	}
	go runChannelPurityScan(scan, channel)
	c.JSON(http.StatusAccepted, gin.H{"success": true, "data": toPurityScanResponse(scan)})
}

func runChannelPurityScan(scan *model.ChannelPurityScan, channel *model.Channel) {
	defer func() { <-channelPuritySlots }()
	executeChannelPurityScan(context.Background(), scan, channel, scan.CreatedBy)
}

func executeChannelPurityScan(parent context.Context, scan *model.ChannelPurityScan, channel *model.Channel, testUserID int) {
	defer func() {
		if recovered := recover(); recovered != nil {
			common.SysError(fmt.Sprintf("channel purity scan panic: %v\n%s", recovered, debug.Stack()))
			finishChannelPurityOperationalFailure(scan, "internal_panic", "Probe failed internally; risk could not be determined")
		}
	}()

	scan.StartedAt = time.Now().Unix()
	if err := model.MarkChannelPurityScanRunning(scan.ID, scan.StartedAt); err != nil {
		common.SysError("failed to mark channel purity scan running: " + err.Error())
	}
	if testUserID <= 0 {
		resolved, err := resolveChannelTestUserID(nil)
		if err != nil {
			finishChannelPurityOperationalFailure(scan, "test_user_unavailable", "Inspection identity is unavailable; risk could not be determined")
			return
		}
		testUserID = resolved
	}

	ctx, cancel := context.WithTimeout(parent, 60*time.Second)
	defer cancel()
	endpointType := purityEndpointType(channel, scan.RequestedModel)
	probe := testChannelWithOptions(ctx, channel, testUserID, scan.RequestedModel, endpointType,
		shouldUseStreamForAutomaticChannelTest(channel), channelTestOptions{
			recordConsumeLog:  false,
			captureResponse:   true,
			allowMissingUsage: true,
		})
	applyChannelTestProbe(scan, probe)
	scan.CompletedAt = time.Now().Unix()
	if err := model.FinishChannelPurityScan(scan, probeResult(scan, probe)); err != nil {
		common.SysError("failed to finish channel purity scan: " + err.Error())
	}
}

func finishChannelPurityOperationalFailure(scan *model.ChannelPurityScan, errorClass, summary string) {
	scan.Status = model.ChannelPurityStatusFailed
	scan.Conclusion = model.ChannelPurityConclusionUnknown
	scan.Risk = model.ChannelPurityRiskUnknown
	scan.Coverage = 0
	scan.ErrorClass = errorClass
	scan.Summary = summary
	scan.CompletedAt = time.Now().Unix()
	if err := model.FinishChannelPurityScan(scan, nil); err != nil {
		common.SysError("failed to finish channel purity scan: " + err.Error())
	}
}

func purityEndpointType(channel *model.Channel, modelName string) string {
	lower := strings.ToLower(strings.TrimSpace(modelName))
	if strings.HasSuffix(modelName, ratio_setting.CompactModelSuffix) {
		return string(constant.EndpointTypeOpenAIResponseCompact)
	}
	if channel != nil && channel.Type == constant.ChannelTypeCodex {
		return string(constant.EndpointTypeOpenAIResponse)
	}
	if strings.Contains(lower, "rerank") || (channel != nil && channel.Type == constant.ChannelTypeJina) {
		return string(constant.EndpointTypeJinaRerank)
	}
	if strings.Contains(lower, "embedding") || strings.Contains(lower, "embed") || strings.HasPrefix(lower, "m3e") || strings.Contains(lower, "bge-") {
		return string(constant.EndpointTypeEmbeddings)
	}
	if common.IsImageGenerationModel(modelName) {
		return string(constant.EndpointTypeImageGeneration)
	}
	if common.IsOpenAIResponseOnlyModel(modelName) {
		return string(constant.EndpointTypeOpenAIResponse)
	}
	if channel != nil {
		switch channel.Type {
		case constant.ChannelTypeAws, constant.ChannelTypeAnthropic:
			return string(constant.EndpointTypeAnthropic)
		case constant.ChannelTypeVertexAi, constant.ChannelTypeGemini:
			return string(constant.EndpointTypeGemini)
		}
	}
	return ""
}

func applyChannelTestProbe(scan *model.ChannelPurityScan, probe testResult) {
	scan.Protocol = string(probe.protocol)
	if scan.Protocol == "" {
		scan.Protocol = "unknown"
	}
	if probe.localErr != nil || probe.newAPIError != nil {
		scan.Status = model.ChannelPurityStatusFailed
		scan.Conclusion = model.ChannelPurityConclusionUnknown
		scan.Risk = model.ChannelPurityRiskUnknown
		scan.Coverage = 0
		scan.ErrorClass = classifyPurityProbeError(probe)
		scan.Summary = "Probe failed operationally; risk could not be determined"
		return
	}

	hasOutput := purityResponseHasOutput(probe.protocol, probe.responseBody)
	scan.Status = model.ChannelPurityStatusCompleted
	scan.Coverage = 100
	if !hasOutput {
		scan.Conclusion = model.ChannelPurityConclusionRisk
		scan.Risk = model.ChannelPurityRiskMedium
		scan.ErrorClass = "missing_output"
		scan.Summary = "Successful response lacks protocol output; structural risk detected"
		return
	}
	declared := purityDeclaredModel(probe.responseBody)
	if declared != "" && probe.mappedModel != "" && !samePurityModelFamily(probe.mappedModel, declared) {
		scan.Conclusion = model.ChannelPurityConclusionUnknown
		scan.Risk = model.ChannelPurityRiskUnknown
		scan.Coverage = 75
		scan.Summary = "Declared model differs from the mapped request; identity remains unproven"
		return
	}
	if declared == "" || !probe.usagePresent {
		scan.Conclusion = model.ChannelPurityConclusionUnknown
		scan.Risk = model.ChannelPurityRiskUnknown
		scan.Coverage = 75
		scan.Summary = "Probe succeeded, but optional identity evidence is incomplete"
		return
	}
	scan.Conclusion = model.ChannelPurityConclusionNoObviousRisk
	scan.Risk = model.ChannelPurityRiskLow
	scan.Summary = "Probe found no obvious structural risk; model identity is not proven"
}

func probeResult(scan *model.ChannelPurityScan, probe testResult) *model.ChannelPurityResult {
	if probe.localErr != nil || probe.newAPIError != nil {
		return nil
	}
	declared := purityDeclaredModel(probe.responseBody)
	result := &model.ChannelPurityResult{
		ChannelID: scan.ChannelID, DeclaredModel: declared,
		HTTPStatus: probe.httpStatus, LatencyMS: probe.latencyMS,
		HasModelField: declared != "", HasUsage: probe.usagePresent,
		HasOutput: purityResponseHasOutput(probe.protocol, probe.responseBody), CreatedAt: time.Now().Unix(),
	}
	result.ProtocolValid = result.HasOutput
	if probe.usage != nil {
		result.PromptTokens = probe.usage.PromptTokens
		result.CompletionTokens = probe.usage.CompletionTokens
		result.TotalTokens = probe.usage.TotalTokens
	}
	evidence := dto.ChannelPurityEvidence{
		HTTPStatus: result.HTTPStatus, ContentType: probe.contentType, DeclaredModel: result.DeclaredModel,
		MappedModel: probe.mappedModel, HasModelField: result.HasModelField, HasUsage: result.HasUsage,
		HasOutput: result.HasOutput, HasChoices: gjson.GetBytes(probe.responseBody, "choices.#").Int() > 0,
		Usage: dto.ChannelPurityUsage{PromptTokens: result.PromptTokens, CompletionTokens: result.CompletionTokens, TotalTokens: result.TotalTokens},
	}
	if result.DeclaredModel != "" && probe.mappedModel != "" && !samePurityModelFamily(probe.mappedModel, result.DeclaredModel) {
		evidence.Warnings = append(evidence.Warnings, "declared_model_differs_from_mapped_request")
	}
	if encoded, err := common.Marshal(evidence); err == nil {
		result.EvidenceJSON = string(encoded)
	} else {
		result.EvidenceJSON = "{}"
	}
	return result
}

func purityResponseHasOutput(protocol types.RelayFormat, body []byte) bool {
	paths := []string{"choices.#", "output.#", "content.#", "candidates.#", "data.#", "results.#"}
	if protocol == types.RelayFormatOpenAIResponsesCompaction {
		paths = append(paths, "compact.#")
	}
	for _, path := range paths {
		if gjson.GetBytes(body, path).Int() > 0 {
			return true
		}
	}
	return strings.TrimSpace(gjson.GetBytes(body, "output_text").String()) != ""
}

func purityDeclaredModel(body []byte) string {
	for _, path := range []string{"model", "modelVersion", "model_version"} {
		if value := strings.TrimSpace(gjson.GetBytes(body, path).String()); value != "" {
			return value
		}
	}
	return ""
}

func samePurityModelFamily(expected, actual string) bool {
	normalize := func(value string) string {
		value = strings.ToLower(strings.TrimSpace(value))
		return strings.TrimPrefix(value, "models/")
	}
	expected, actual = normalize(expected), normalize(actual)
	return expected == actual || strings.HasPrefix(actual, expected+"-") || strings.HasPrefix(expected, actual+"-")
}

func classifyPurityProbeError(probe testResult) string {
	status := probe.httpStatus
	switch status {
	case http.StatusUnauthorized, http.StatusForbidden:
		return "authentication_error"
	case http.StatusTooManyRequests:
		return "rate_limited"
	case http.StatusRequestTimeout, http.StatusGatewayTimeout:
		return "timeout"
	}
	if errors.Is(probe.localErr, context.DeadlineExceeded) {
		return "timeout"
	}
	if status >= 500 {
		return "upstream_server_error"
	}
	if status >= 400 {
		return "upstream_request_error"
	}
	return "probe_error"
}

func ListChannelPurityResults(c *gin.Context) {
	page := common.GetPageQuery(c)
	scans, total, err := model.ListChannelPurityResults(page.GetStartIdx(), page.GetPageSize())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "failed to list results"})
		return
	}
	items := make([]dto.ChannelPurityScanResponse, 0, len(scans))
	for i := range scans {
		items = append(items, toPurityScanResponse(&scans[i]))
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"items": items, "total": total, "page": page.GetPage(), "page_size": page.GetPageSize()}})
}

func GetChannelPurityScan(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("scan_id"), 10, 64)
	if err != nil || id == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid scan id"})
		return
	}
	scan, err := model.GetChannelPurityScan(uint(id))
	if err != nil {
		writeChannelPurityLookupError(c, err, "scan not found")
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": toPurityScanResponse(scan)})
}

func GetLatestChannelPurityResult(c *gin.Context) {
	channelID, err := strconv.Atoi(c.Param("id"))
	if err != nil || channelID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid channel id"})
		return
	}
	scan, err := model.GetLatestChannelPurityScan(channelID, strings.TrimSpace(c.Query("model")))
	if err != nil {
		writeChannelPurityLookupError(c, err, "result not found")
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": toPurityScanResponse(scan)})
}

func toPurityScanResponse(scan *model.ChannelPurityScan) dto.ChannelPurityScanResponse {
	response := dto.ChannelPurityScanResponse{
		ID: scan.ID, ChannelID: scan.ChannelID, ChannelName: scan.ChannelName, Model: scan.RequestedModel,
		Protocol: scan.Protocol, Status: scan.Status, Conclusion: scan.Conclusion, Risk: scan.Risk,
		Coverage: scan.Coverage, Summary: scan.Summary, ErrorClass: scan.ErrorClass, CreatedBy: scan.CreatedBy,
		CreatedAt: scan.CreatedAt, StartedAt: scan.StartedAt, CompletedAt: scan.CompletedAt,
	}
	if scan.Result != nil {
		result := toPurityResultResponse(scan)
		response.Result = &result
	}
	return response
}

func toPurityResultResponse(scan *model.ChannelPurityScan) dto.ChannelPurityResultResponse {
	result := scan.Result
	evidence := dto.ChannelPurityEvidence{}
	if result.EvidenceJSON != "" {
		_ = common.UnmarshalJsonStr(result.EvidenceJSON, &evidence)
	}
	return dto.ChannelPurityResultResponse{
		ID: result.ID, ScanID: result.ScanID, ChannelID: result.ChannelID, Model: scan.RequestedModel,
		Protocol: scan.Protocol, Status: scan.Status, Conclusion: scan.Conclusion, Risk: scan.Risk,
		Coverage: scan.Coverage, Summary: scan.Summary, DeclaredModel: result.DeclaredModel,
		LatencyMS: result.LatencyMS, HTTPStatus: result.HTTPStatus, ErrorClass: scan.ErrorClass,
		Usage:    dto.ChannelPurityUsage{PromptTokens: result.PromptTokens, CompletionTokens: result.CompletionTokens, TotalTokens: result.TotalTokens},
		Evidence: evidence, CreatedAt: result.CreatedAt,
	}
}

func channelHasModel(configured, requested string) bool {
	for _, candidate := range strings.Split(configured, ",") {
		if strings.TrimSpace(candidate) == requested {
			return true
		}
	}
	return false
}

func writeChannelPurityLookupError(c *gin.Context, err error, notFoundMessage string) {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "message": notFoundMessage})
		return
	}
	c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "database query failed"})
}
