package controller

import (
	"context"
	"fmt"
	"net/http"
	"runtime/debug"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
)

const channelPurityBatchConcurrency = 2

type channelPurityInspectionHandler struct{}

type channelPurityInspectionPayload struct {
	CreatedBy int  `json:"created_by,omitempty"`
	Manual    bool `json:"manual,omitempty"`
}

type channelPurityInspectionItem struct {
	channel *model.Channel
	model   string
}

type channelPurityInspectionSummary struct {
	Total     int `json:"total"`
	Completed int `json:"completed"`
	Failed    int `json:"failed"`
	Skipped   int `json:"skipped"`
}

func (channelPurityInspectionHandler) Type() string { return model.SystemTaskTypeChannelPurity }

func (channelPurityInspectionHandler) Enabled() bool {
	return operation_setting.IsChannelPurityInspectionEnabled()
}

func (channelPurityInspectionHandler) Interval() time.Duration {
	return time.Duration(operation_setting.GetChannelPurityInspectionIntervalMinutes()) * time.Minute
}

func (channelPurityInspectionHandler) NewPayload() any {
	return channelPurityInspectionPayload{}
}

func (channelPurityInspectionHandler) Run(ctx context.Context, task *model.SystemTask, runnerID string) {
	finished := false
	defer func() {
		if recovered := recover(); recovered != nil {
			common.SysError(fmt.Sprintf("channel purity inspection panic: %v\n%s", recovered, debug.Stack()))
			if !finished {
				finishSystemTaskHandler(task, runnerID, model.SystemTaskStatusFailed, nil, fmt.Errorf("channel purity inspection failed internally"))
			}
		}
	}()

	payload := channelPurityInspectionPayload{}
	if err := task.DecodePayload(&payload); err != nil {
		finishSystemTaskHandler(task, runnerID, model.SystemTaskStatusFailed, nil, err)
		finished = true
		return
	}
	summary, err := runChannelPurityInspection(ctx, payload.CreatedBy, service.NewSystemTaskProgressReporter(task, runnerID))
	if err != nil {
		finishSystemTaskHandler(task, runnerID, model.SystemTaskStatusFailed, summary, err)
		finished = true
		return
	}
	finishSystemTaskHandler(task, runnerID, model.SystemTaskStatusSucceeded, summary, nil)
	finished = true
}

func StartChannelPurityInspection(c *gin.Context) {
	task, created, err := service.EnqueueSystemTask(model.SystemTaskTypeChannelPurity, channelPurityInspectionPayload{
		CreatedBy: c.GetInt("id"),
		Manual:    true,
	})
	if err != nil {
		common.ApiError(c, err)
		return
	}
	status := http.StatusAccepted
	if !created {
		status = http.StatusConflict
	}
	c.JSON(status, gin.H{
		"success": created,
		"message": map[bool]string{true: "", false: "a channel purity inspection is already pending or running"}[created],
		"data":    task.ToResponse(),
	})
}

func GetChannelPurityInspectionStatus(c *gin.Context) {
	active, err := model.GetActiveSystemTask(model.SystemTaskTypeChannelPurity)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	latest, err := model.GetLatestSystemTask(model.SystemTaskTypeChannelPurity)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	lastFinished, err := model.GetLatestFinishedSystemTask(model.SystemTaskTypeChannelPurity)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	settings := currentChannelPurityInspectionSettings()
	channels, err := model.ListEnabledChannelsForPurityInspection()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	items, _ := buildChannelPurityInspectionItems(channels)
	status := dto.ChannelPurityInspectionStatus{
		Enabled: settings.Enabled, IntervalMinutes: settings.IntervalMinutes, Running: active != nil,
		EnabledChannels: len(channels), ModelCombinations: len(items),
	}
	if lastFinished != nil {
		status.LastRunAt = lastFinished.UpdatedAt
	}
	if settings.Enabled && active == nil {
		if latest == nil {
			status.NextRunAt = time.Now().Unix()
		} else {
			status.NextRunAt = latest.UpdatedAt + int64(settings.IntervalMinutes*60)
		}
	}
	if active != nil {
		status.Task = active.ToResponse()
	} else if latest != nil {
		status.Task = latest.ToResponse()
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": status})
}

func GetChannelPurityInspectionSettings(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"success": true, "data": currentChannelPurityInspectionSettings()})
}

func UpdateChannelPurityInspectionSettings(c *gin.Context) {
	var request dto.ChannelPurityInspectionSettings
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid settings"})
		return
	}
	interval := strconv.Itoa(request.IntervalMinutes)
	if err := operation_setting.ValidateChannelPurityInspectionIntervalMinutes(interval); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": err.Error()})
		return
	}
	if err := model.UpdateOptionsBulk(map[string]string{
		"qiqi_setting.channel_purity_inspection_enabled":          strconv.FormatBool(request.Enabled),
		"qiqi_setting.channel_purity_inspection_interval_minutes": interval,
	}); err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": currentChannelPurityInspectionSettings()})
}

func currentChannelPurityInspectionSettings() dto.ChannelPurityInspectionSettings {
	return dto.ChannelPurityInspectionSettings{
		Enabled:         operation_setting.IsChannelPurityInspectionEnabled(),
		IntervalMinutes: operation_setting.GetChannelPurityInspectionIntervalMinutes(),
	}
}

func runChannelPurityInspection(ctx context.Context, createdBy int, report func(processed, total int)) (channelPurityInspectionSummary, error) {
	channels, err := model.ListEnabledChannelsForPurityInspection()
	if err != nil {
		return channelPurityInspectionSummary{}, err
	}
	items, skipped := buildChannelPurityInspectionItems(channels)
	summary := channelPurityInspectionSummary{Total: len(items), Skipped: skipped}
	if report != nil {
		report(0, len(items))
	}
	if len(items) == 0 {
		return summary, nil
	}

	jobs := make(chan channelPurityInspectionItem)
	results := make(chan bool, len(items))
	var workers sync.WaitGroup
	for range channelPurityBatchConcurrency {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for item := range jobs {
				results <- runChannelPurityInspectionItem(ctx, item, createdBy)
			}
		}()
	}
	go func() {
		defer close(jobs)
		for _, item := range items {
			select {
			case <-ctx.Done():
				return
			case jobs <- item:
			}
		}
	}()
	go func() {
		workers.Wait()
		close(results)
	}()

	for succeeded := range results {
		summary.Completed++
		if !succeeded {
			summary.Failed++
		}
		if report != nil {
			report(summary.Completed, summary.Total)
		}
	}
	if err := ctx.Err(); err != nil {
		return summary, err
	}
	return summary, nil
}

func buildChannelPurityInspectionItems(channels []*model.Channel) ([]channelPurityInspectionItem, int) {
	items := make([]channelPurityInspectionItem, 0)
	skipped := 0
	for _, channel := range channels {
		seen := map[string]struct{}{}
		models := make([]string, 0)
		for _, configuredModel := range strings.Split(channel.Models, ",") {
			name := strings.TrimSpace(configuredModel)
			if name == "" {
				continue
			}
			if _, exists := seen[name]; exists {
				continue
			}
			seen[name] = struct{}{}
			models = append(models, name)
		}
		if len(models) == 0 {
			skipped++
			continue
		}
		sort.Strings(models)
		for _, name := range models {
			items = append(items, channelPurityInspectionItem{channel: channel, model: name})
		}
	}
	return items, skipped
}

func runChannelPurityInspectionItem(ctx context.Context, item channelPurityInspectionItem, createdBy int) bool {
	now := time.Now().Unix()
	scan := &model.ChannelPurityScan{
		ChannelID: item.channel.Id, ChannelName: item.channel.Name, RequestedModel: item.model, Protocol: "pending",
		Status: model.ChannelPurityStatusPending, Conclusion: model.ChannelPurityConclusionUnknown,
		Risk: model.ChannelPurityRiskUnknown, Summary: "Automatic Quick probe is queued", CreatedBy: createdBy, CreatedAt: now,
	}
	if err := model.CreateChannelPurityScan(scan); err != nil {
		common.SysError(fmt.Sprintf("failed to create channel purity scan: channel=%d model=%q err=%v", item.channel.Id, item.model, err))
		return false
	}
	executeChannelPurityScan(ctx, scan, item.channel, createdBy)
	return scan.Status == model.ChannelPurityStatusCompleted
}
