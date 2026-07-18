package controller

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	channelpurity "github.com/QuantumNous/new-api/service/channel_purity"
	"github.com/gin-gonic/gin"
)

const purityGroupSchedulerInterval = 5 * time.Minute

type channelPurityGroupDetectionHandler struct{}

type channelPurityGroupDetectionPayload struct {
	GroupID uint `json:"group_id,omitempty"`
	Manual  bool `json:"manual,omitempty"`
}

type channelPurityGroupDetectionSummary struct {
	Groups  int `json:"groups"`
	Pairs   int `json:"pairs"`
	Failed  int `json:"failed"`
	Skipped int `json:"skipped"`
}

func (channelPurityGroupDetectionHandler) Type() string {
	return model.SystemTaskTypeChannelPurityAggregate
}
func (channelPurityGroupDetectionHandler) Enabled() bool { return model.HasEnabledPurityGroups() }
func (channelPurityGroupDetectionHandler) Interval() time.Duration {
	return purityGroupSchedulerInterval
}
func (channelPurityGroupDetectionHandler) NewPayload() any {
	return channelPurityGroupDetectionPayload{}
}

func (channelPurityGroupDetectionHandler) Run(ctx context.Context, task *model.SystemTask, runnerID string) {
	payload := channelPurityGroupDetectionPayload{}
	if err := task.DecodePayload(&payload); err != nil {
		finishSystemTaskHandler(task, runnerID, model.SystemTaskStatusFailed, nil, err)
		return
	}
	summary, err := runDuePurityGroups(ctx, payload.GroupID, payload.Manual, service.NewSystemTaskProgressReporter(task, runnerID))
	status := model.SystemTaskStatusSucceeded
	if err != nil {
		status = model.SystemTaskStatusFailed
	}
	finishSystemTaskHandler(task, runnerID, status, summary, err)
}

func StartChannelPurityGroupDetection(c *gin.Context) {
	groupID, err := strconvParseGroupID(c.Param("group_id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid group id"})
		return
	}
	group, err := model.GetPurityGroup(groupID)
	if err != nil {
		purityGroupLookup(c, err)
		return
	}
	if !group.Enabled {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "group is disabled"})
		return
	}
	task, created, err := service.EnqueueSystemTask(model.SystemTaskTypeChannelPurityAggregate, channelPurityGroupDetectionPayload{GroupID: groupID, Manual: true})
	if err != nil {
		common.ApiError(c, err)
		return
	}
	status := http.StatusAccepted
	if !created {
		status = http.StatusConflict
	}
	c.JSON(status, gin.H{"success": created, "data": task.ToResponse(), "message": map[bool]string{true: "", false: "a grouped purity detection task is already pending or running"}[created]})
}

func strconvParseGroupID(raw string) (uint, error) {
	var value uint64
	_, err := fmt.Sscan(strings.TrimSpace(raw), &value)
	if err != nil || value == 0 {
		return 0, fmt.Errorf("invalid group id")
	}
	return uint(value), nil
}

func runDuePurityGroups(ctx context.Context, requestedGroupID uint, manual bool, report func(processed, total int)) (channelPurityGroupDetectionSummary, error) {
	now := time.Now().Unix()
	var groups []model.ChannelPurityGroup
	if requestedGroupID != 0 {
		group, err := model.GetPurityGroup(requestedGroupID)
		if err != nil {
			return channelPurityGroupDetectionSummary{}, err
		}
		groups = []model.ChannelPurityGroup{*group}
	} else {
		var err error
		groups, err = model.ListDuePurityGroups(now)
		if err != nil {
			return channelPurityGroupDetectionSummary{}, err
		}
	}
	summary := channelPurityGroupDetectionSummary{Groups: len(groups)}
	if report != nil {
		report(0, len(groups))
	}
	if len(groups) == 0 {
		return summary, nil
	}
	userID, err := resolveChannelTestUserID(nil)
	if err != nil {
		return summary, err
	}
	for index := range groups {
		if ctx.Err() != nil {
			return summary, ctx.Err()
		}
		group := &groups[index]
		if !group.Enabled {
			summary.Skipped++
			continue
		}
		pairs, runErr := runPurityGroupDetection(ctx, group, userID)
		summary.Pairs += pairs
		lastError := ""
		if runErr != nil {
			summary.Failed++
			lastError = runErr.Error()
			if len(lastError) > 500 {
				lastError = lastError[:500]
			}
		}
		finishedAt := time.Now().Unix()
		nextRunAt := finishedAt + int64(group.IntervalMinutes*60)
		if manual && group.NextRunAt > finishedAt {
			nextRunAt = group.NextRunAt
		}
		if markErr := model.MarkPurityGroupRun(group.ID, finishedAt, nextRunAt, lastError); markErr != nil && runErr == nil {
			runErr = markErr
			summary.Failed++
		}
		if report != nil {
			report(index+1, len(groups))
		}
	}
	return summary, nil
}

func runPurityGroupDetection(ctx context.Context, group *model.ChannelPurityGroup, userID int) (int, error) {
	baselineID := 0
	targetIDs := make([]int, 0, len(group.Members)-1)
	for _, member := range group.Members {
		if member.IsBaseline {
			baselineID = member.ChannelID
		} else {
			targetIDs = append(targetIDs, member.ChannelID)
		}
	}
	if baselineID == 0 || len(targetIDs) == 0 {
		return 0, fmt.Errorf("group requires one baseline and at least one target")
	}
	baseline, err := model.GetChannelById(baselineID, true)
	if err != nil {
		return 0, fmt.Errorf("load baseline channel: %w", err)
	}
	targets := make([]*model.Channel, 0, len(targetIDs))
	for _, targetID := range targetIDs {
		target, targetErr := model.GetChannelById(targetID, true)
		if targetErr != nil {
			return 0, fmt.Errorf("load target channel %d: %w", targetID, targetErr)
		}
		targets = append(targets, target)
	}
	if group.RandomPairingEnabled {
		rand.Shuffle(len(targets), func(i, j int) { targets[i], targets[j] = targets[j], targets[i] })
	}
	baselineModels := channelModelSet(baseline)
	modelsToTargets := map[string][]*model.Channel{}
	for _, target := range targets {
		for modelName := range channelModelSet(target) {
			if baselineModels[modelName] {
				modelsToTargets[modelName] = append(modelsToTargets[modelName], target)
			}
		}
	}
	modelNames := make([]string, 0, len(modelsToTargets))
	for modelName := range modelsToTargets {
		modelNames = append(modelNames, modelName)
	}
	sort.Strings(modelNames)
	if group.RandomPairingEnabled {
		rand.Shuffle(len(modelNames), func(i, j int) { modelNames[i], modelNames[j] = modelNames[j], modelNames[i] })
	}
	if len(modelNames) == 0 {
		return 0, fmt.Errorf("baseline and targets do not share a configured model")
	}
	pairs := 0
	var firstErr error
	for _, modelName := range modelNames {
		if ctx.Err() != nil {
			return pairs, ctx.Err()
		}
		runKey := common.NewRequestId()
		baselineSample := runPuritySample(ctx, group.ID, runKey, baseline, modelName, userID)
		if err = model.CreatePuritySample(&baselineSample); err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		for _, target := range modelsToTargets[modelName] {
			if group.MaxSamplesPerWindow > 0 && pairs >= group.MaxSamplesPerWindow {
				break
			}
			targetSample := runPuritySample(ctx, group.ID, runKey, target, modelName, userID)
			if createErr := model.CreatePuritySample(&targetSample); createErr != nil {
				if firstErr == nil {
					firstErr = createErr
				}
				continue
			}
			windowEnd := time.Now().Unix() + 1
			windowStart := windowEnd - int64(group.WindowMinutes*60)
			policy := channelpurity.DefaultAggregatePolicy()
			policy.MinSamples = group.MinimumSamples
			if _, aggregateErr := channelpurity.AggregatePairWindow(group.ID, target.Id, modelName, windowStart, windowEnd, policy); aggregateErr != nil && firstErr == nil {
				firstErr = aggregateErr
			}
			pairs++
		}
	}
	return pairs, firstErr
}

func channelModelSet(channel *model.Channel) map[string]bool {
	values := map[string]bool{}
	if channel == nil {
		return values
	}
	for _, modelName := range channel.GetModels() {
		modelName = strings.TrimSpace(modelName)
		if modelName != "" {
			values[modelName] = true
		}
	}
	return values
}

func runPuritySample(parent context.Context, groupID uint, runKey string, channel *model.Channel, modelName string, userID int) model.ChannelPuritySample {
	observedAt := time.Now().Unix()
	var observation *relaycommon.PurityObservation
	ctx, cancel := context.WithTimeout(parent, 60*time.Second)
	defer cancel()
	probe := testChannelWithOptions(ctx, channel, userID, modelName, purityEndpointType(channel, modelName), shouldUseStreamForAutomaticChannelTest(channel), channelTestOptions{
		recordConsumeLog: false, captureResponse: true, allowMissingUsage: true,
		purityDetectionRequest: true, purityPairID: runKey,
		purityObserver: func(value relaycommon.PurityObservation) { copy := value; observation = &copy },
	})
	sample := model.ChannelPuritySample{GroupID: groupID, ChannelID: channel.Id, ActualModel: modelName, RunKey: runKey, ObservedAt: observedAt}
	if probe.localErr != nil || probe.newAPIError != nil {
		sample.Valid = false
		sample.ErrorClass = classifyPurityProbeError(probe)
		return sample
	}
	if observation == nil {
		header := http.Header{}
		header.Set("Content-Type", probe.contentType)
		features := channelpurity.ExtractAnonymousFeatures(probe.httpStatus, header, probe.responseBody, false)
		observation = &relaycommon.PurityObservation{
			Protocol: features.Protocol, StatusCode: features.StatusCode, ModelFamily: features.ModelFamily,
			FieldPaths: features.FieldPaths, EventSequence: features.EventSequence, FinishReasons: features.FinishReasons,
			ProviderInput: features.ProviderUsage.Input, ProviderOutput: features.ProviderUsage.Output, ProviderTotal: features.ProviderUsage.Total,
			UnifiedTokenCount: features.UnifiedTokenCount, HeaderPresence: features.HeaderPresence, HasSignatureID: features.HasSignatureID, Truncated: features.Truncated,
		}
	}
	sample.Protocol = observation.Protocol
	sample.StructureSignature = purityObservationSignature(*observation)
	sample.PromptTokens = observation.ProviderInput
	sample.CompletionTokens = observation.ProviderOutput
	sample.TotalTokens = observation.ProviderTotal
	if sample.TotalTokens == 0 {
		sample.TotalTokens = observation.UnifiedTokenCount
	}
	if probe.usage != nil {
		if sample.PromptTokens == 0 {
			sample.PromptTokens = probe.usage.PromptTokens
		}
		if sample.CompletionTokens == 0 {
			sample.CompletionTokens = probe.usage.CompletionTokens
		}
		if sample.TotalTokens == 0 {
			sample.TotalTokens = probe.usage.TotalTokens
		}
	}
	sample.Valid = observation.StatusCode >= 200 && observation.StatusCode < 300 && sample.StructureSignature != ""
	if !sample.Valid {
		sample.ErrorClass = "invalid_anonymous_observation"
	}
	return sample
}

func purityObservationSignature(observation relaycommon.PurityObservation) string {
	payload := struct {
		Protocol    string          `json:"protocol"`
		ModelFamily string          `json:"model_family"`
		Fields      []string        `json:"fields"`
		Events      []string        `json:"events"`
		Finish      []string        `json:"finish"`
		Headers     map[string]bool `json:"headers"`
		Signature   bool            `json:"signature"`
	}{observation.Protocol, observation.ModelFamily, observation.FieldPaths, observation.EventSequence, observation.FinishReasons, observation.HeaderPresence, observation.HasSignatureID}
	encoded, _ := json.Marshal(payload)
	hash := sha256.Sum256(encoded)
	return hex.EncodeToString(hash[:])
}
