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
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	channelpurity "github.com/QuantumNous/new-api/service/channel_purity"
	"github.com/gin-gonic/gin"
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
	defer func() {
		<-channelPuritySlots
		if recovered := recover(); recovered != nil {
			common.SysError(fmt.Sprintf("channel purity scan panic: %v\n%s", recovered, debug.Stack()))
			now := time.Now().Unix()
			scan.Status = model.ChannelPurityStatusFailed
			scan.Conclusion = model.ChannelPurityConclusionUnknown
			scan.Risk = model.ChannelPurityRiskUnknown
			scan.Summary = "Quick probe failed internally; conclusion unknown"
			scan.ErrorClass = "internal_panic"
			scan.CompletedAt = now
			if err := model.FinishChannelPurityScan(scan, nil); err != nil {
				common.SysError("failed to persist channel purity panic state: " + err.Error())
			}
		}
	}()

	scan.StartedAt = time.Now().Unix()
	if err := model.MarkChannelPurityScanRunning(scan.ID, scan.StartedAt); err != nil {
		common.SysError("failed to mark channel purity scan running: " + err.Error())
	}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Second)
	defer cancel()
	outcome := channelpurity.RunQuickProbe(ctx, channel, scan.RequestedModel)
	scan.Status = outcome.Status
	scan.Conclusion = outcome.Conclusion
	scan.Risk = outcome.Risk
	scan.Coverage = outcome.Coverage
	scan.Summary = outcome.Summary
	scan.ErrorClass = outcome.ErrorClass
	scan.CompletedAt = time.Now().Unix()
	if err := model.FinishChannelPurityScan(scan, outcome.Result); err != nil {
		common.SysError("failed to finish channel purity scan: " + err.Error())
	}
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
