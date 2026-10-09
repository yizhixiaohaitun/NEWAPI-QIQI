package service

import (
	"errors"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/gin-gonic/gin"
)

type RetryParam struct {
	Ctx         *gin.Context
	TokenGroup  string
	ModelName   string
	RequestPath string
	Retry       *int
}

func (p *RetryParam) GetRetry() int {
	if p.Retry == nil {
		return 0
	}
	return *p.Retry
}

func (p *RetryParam) SetRetry(retry int) {
	p.Retry = &retry
}

// MarkChannelFailed prevents a request retry from selecting the same failed
// single-key upstream channel again, including through an overlapping auto group.
func MarkChannelFailed(c *gin.Context, channelID int) {
	if c == nil || channelID <= 0 {
		return
	}
	failed := make(map[int]bool)
	if value, exists := common.GetContextKey(c, constant.ContextKeyAutoGroupFailedChannels); exists {
		if existing, ok := value.(map[int]bool); ok {
			for id, isFailed := range existing {
				failed[id] = isFailed
			}
		}
	}
	failed[channelID] = true
	common.SetContextKey(c, constant.ContextKeyAutoGroupFailedChannels, failed)
}

func (p *RetryParam) IncreaseRetry() bool {
	if p.Retry == nil {
		p.Retry = new(int)
	}
	if *p.Retry == int(^uint(0)>>1) {
		return false
	}
	*p.Retry++
	return true
}

// CacheGetRandomSatisfiedChannel selects a channel for the requested group.
// For an auto group, the first selection follows the configured group order.
// With cross-group retry enabled, each later attempt starts at the next
// untried group and uses that group's normal first-choice priority. Groups with
// no matching channel are skipped without consuming another controller retry.
func CacheGetRandomSatisfiedChannel(param *RetryParam) (*model.Channel, string, error) {
	selectGroup := param.TokenGroup
	failedChannels := make(map[int]bool)
	if value, exists := common.GetContextKey(param.Ctx, constant.ContextKeyAutoGroupFailedChannels); exists {
		if failed, ok := value.(map[int]bool); ok {
			for channelID, isFailed := range failed {
				failedChannels[channelID] = isFailed
			}
		}
	}
	if param.TokenGroup != "auto" {
		channel, err := model.GetRandomSatisfiedChannelExcluding(param.TokenGroup, param.ModelName, param.GetRetry(), param.RequestPath, failedChannels)
		if err != nil {
			return nil, param.TokenGroup, err
		}
		return channel, selectGroup, nil
	}

	if len(setting.GetAutoGroups()) == 0 {
		return nil, selectGroup, errors.New("auto groups is not enabled")
	}
	userGroup := common.GetContextKeyString(param.Ctx, constant.ContextKeyUserGroup)
	autoGroups := GetUserAutoGroup(userGroup)
	startGroupIndex := 0
	if value, exists := common.GetContextKey(param.Ctx, constant.ContextKeyAutoGroupIndex); exists {
		if index, ok := value.(int); ok && index >= 0 && index < len(autoGroups) {
			startGroupIndex = index
		}
	}

	crossGroupRetry := common.GetContextKeyBool(param.Ctx, constant.ContextKeyTokenCrossGroupRetry)
	visitedGroups := make(map[string]bool)
	if value, exists := common.GetContextKey(param.Ctx, constant.ContextKeyAutoGroupVisited); exists {
		if visited, ok := value.(map[string]bool); ok {
			for group, used := range visited {
				visitedGroups[group] = used
			}
		}
	}
	if crossGroupRetry && param.GetRetry() > 0 {
		// Retry from the configured head and skip every group already used by
		// this request. This also handles affinity starting in the middle or at
		// the end without omitting earlier configured groups.
		startGroupIndex = 0
	}

	for i := startGroupIndex; i < len(autoGroups); i++ {
		autoGroup := autoGroups[i]
		if crossGroupRetry && param.GetRetry() > 0 && visitedGroups[autoGroup] {
			continue
		}
		priorityRetry := param.GetRetry()
		if crossGroupRetry || i > startGroupIndex {
			priorityRetry = 0
		}
		logger.LogDebug(param.Ctx, "Auto selecting group: %s, priorityRetry: %d", autoGroup, priorityRetry)

		channel, _ := model.GetRandomSatisfiedChannelExcluding(autoGroup, param.ModelName, priorityRetry, param.RequestPath, failedChannels)
		if channel == nil {
			logger.LogDebug(param.Ctx, "No available channel in group %s for model %s, trying next group", autoGroup, param.ModelName)
			continue
		}

		visitedGroups[autoGroup] = true
		common.SetContextKey(param.Ctx, constant.ContextKeyAutoGroupVisited, visitedGroups)
		common.SetContextKey(param.Ctx, constant.ContextKeyAutoGroupIndex, i)
		common.SetContextKey(param.Ctx, constant.ContextKeyAutoGroup, autoGroup)
		logger.LogDebug(param.Ctx, "Auto selected group: %s", autoGroup)
		return channel, autoGroup, nil
	}

	// Preserve the exhausted position so another call cannot wrap around and
	// hit a previously failed group.
	common.SetContextKey(param.Ctx, constant.ContextKeyAutoGroupIndex, len(autoGroups))
	return nil, selectGroup, nil
}
