package controller

import (
	"fmt"
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
)

func GetAllTask(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)

	startTimestamp, _ := strconv.ParseInt(c.Query("start_timestamp"), 10, 64)
	endTimestamp, _ := strconv.ParseInt(c.Query("end_timestamp"), 10, 64)
	// 解析其他查询参数
	queryParams := model.SyncTaskQueryParams{
		Platform:       constant.TaskPlatform(c.Query("platform")),
		TaskID:         c.Query("task_id"),
		Status:         c.Query("status"),
		Action:         c.Query("action"),
		StartTimestamp: startTimestamp,
		EndTimestamp:   endTimestamp,
		ChannelID:      c.Query("channel_id"),
	}

	items := model.TaskGetAllTasks(pageInfo.GetStartIdx(), pageInfo.GetPageSize(), queryParams)
	total := model.TaskCountAllTasks(queryParams)
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(tasksToDto(items, true))
	common.ApiSuccess(c, pageInfo)
}

func GetUserTask(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)

	userId := c.GetInt("id")

	startTimestamp, _ := strconv.ParseInt(c.Query("start_timestamp"), 10, 64)
	endTimestamp, _ := strconv.ParseInt(c.Query("end_timestamp"), 10, 64)

	queryParams := model.SyncTaskQueryParams{
		Platform:       constant.TaskPlatform(c.Query("platform")),
		TaskID:         c.Query("task_id"),
		Status:         c.Query("status"),
		Action:         c.Query("action"),
		StartTimestamp: startTimestamp,
		EndTimestamp:   endTimestamp,
	}

	items := model.TaskGetAllUserTask(userId, pageInfo.GetStartIdx(), pageInfo.GetPageSize(), queryParams)
	total := model.TaskCountAllUserTask(userId, queryParams)
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(tasksToDto(items, false))
	common.ApiSuccess(c, pageInfo)
}

func GetAllTaskDetail(c *gin.Context) {
	task, exists, err := model.GetByOnlyTaskId(c.Param("task_id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if !exists {
		common.ApiError(c, fmt.Errorf("task not found"))
		return
	}
	if user, userErr := model.GetUserCache(task.UserId); userErr == nil {
		task.Username = user.Username
	}
	common.ApiSuccess(c, taskToDetailDto(task, true))
}

func GetUserTaskDetail(c *gin.Context) {
	task, exists, err := model.GetByTaskId(c.GetInt("id"), c.Param("task_id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if !exists {
		common.ApiError(c, fmt.Errorf("task not found"))
		return
	}
	common.ApiSuccess(c, taskToDetailDto(task, false))
}

func taskToDetailDto(task *model.Task, includeChannel bool) *dto.TaskDetailDto {
	base := relay.TaskModel2Dto(task)
	if !includeChannel {
		base.ChannelId = 0
	}
	base.FailReason = model.SanitizeTaskDetailText(base.FailReason)
	base.ResultURL = model.SanitizeTaskDetailText(base.ResultURL)
	base.Data = model.SanitizeTaskDetailJSON(base.Data)

	properties := task.Properties
	properties.ReferenceResources = append([]string(nil), task.Properties.ReferenceResources...)
	properties.RequestSnapshot = append([]byte(nil), task.Properties.RequestSnapshot...)
	properties.Input = model.SanitizeTaskDetailText(properties.Input)
	for i, resource := range properties.ReferenceResources {
		properties.ReferenceResources[i] = model.SanitizeTaskDetailText(resource)
	}
	properties.RequestSnapshot = model.SanitizeTaskDetailJSON(properties.RequestSnapshot)
	base.Properties = properties

	detail := &dto.TaskDetailDto{
		TaskDto:         base,
		DetailSource:    "normalized_upstream_request",
		RequestSnapshot: properties.RequestSnapshot,
	}
	if len(detail.RequestSnapshot) > 0 {
		return detail
	}

	legacy := map[string]any{}
	if properties.OriginModelName != "" {
		legacy["model"] = properties.OriginModelName
	} else if properties.UpstreamModelName != "" {
		legacy["model"] = properties.UpstreamModelName
	}
	if properties.Input != "" {
		legacy["prompt"] = properties.Input
	}
	if len(properties.ReferenceResources) > 0 {
		legacy["reference_resources"] = properties.ReferenceResources
	}
	if len(legacy) > 0 {
		snapshot, _ := common.Marshal(legacy)
		detail.RequestSnapshot = snapshot
		detail.DetailSource = "legacy_partial"
		detail.MissingFields = []string{"normalized_request_snapshot"}
		return detail
	}

	detail.DetailSource = "unavailable"
	detail.MissingFields = []string{"request_snapshot", "prompt", "reference_resources"}
	return detail
}

func tasksToDto(tasks []*model.Task, fillUser bool) []*dto.TaskDto {
	var userIdMap map[int]*model.UserBase
	if fillUser {
		userIdMap = make(map[int]*model.UserBase)
		userIds := types.NewSet[int]()
		for _, task := range tasks {
			userIds.Add(task.UserId)
		}
		for _, userId := range userIds.Items() {
			cacheUser, err := model.GetUserCache(userId)
			if err == nil {
				userIdMap[userId] = cacheUser
			}
		}
	}
	result := make([]*dto.TaskDto, len(tasks))
	for i, task := range tasks {
		if fillUser {
			if user, ok := userIdMap[task.UserId]; ok {
				task.Username = user.Username
			}
		}
		result[i] = relay.TaskModel2Dto(task)
	}
	return result
}
