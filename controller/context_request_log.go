package controller

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func ListContextLogRules(c *gin.Context) {
	rules, err := model.ListContextLogRules()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": rules})
}
func bindAndValidateContextLogRule(c *gin.Context, rule *model.ContextRequestLogRule) bool {
	if err := c.ShouldBindJSON(rule); err != nil {
		common.ApiErrorMsg(c, "无效的规则参数: "+err.Error())
		return false
	}
	rule.Name = strings.TrimSpace(rule.Name)
	rule.ModelPattern = strings.TrimSpace(rule.ModelPattern)
	rule.Decision = strings.ToLower(strings.TrimSpace(rule.Decision))
	if rule.Name == "" {
		common.ApiErrorMsg(c, "规则名称不能为空")
		return false
	}
	if rule.Decision != model.ContextLogDecisionCapture && rule.Decision != model.ContextLogDecisionSkip {
		common.ApiErrorMsg(c, "decision 只能是 capture 或 skip")
		return false
	}
	if rule.UserId != nil && *rule.UserId <= 0 {
		common.ApiErrorMsg(c, "user_id 必须为正整数或 null")
		return false
	}
	return true
}
func CreateContextLogRule(c *gin.Context) {
	var rule model.ContextRequestLogRule
	if !bindAndValidateContextLogRule(c, &rule) {
		return
	}
	rule.Id = 0
	if err := model.SaveContextLogRule(&rule); err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": rule})
}
func UpdateContextLogRule(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		common.ApiErrorMsg(c, "无效的规则 ID")
		return
	}
	var existing model.ContextRequestLogRule
	if err = model.DB.First(&existing, id).Error; err != nil {
		common.ApiErrorMsg(c, "规则不存在")
		return
	}
	var rule model.ContextRequestLogRule
	if !bindAndValidateContextLogRule(c, &rule) {
		return
	}
	rule.Id = id
	rule.CreatedAt = existing.CreatedAt
	if err = model.SaveContextLogRule(&rule); err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": rule})
}
func DeleteContextLogRule(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		common.ApiErrorMsg(c, "无效的规则 ID")
		return
	}
	deleted, err := model.DeleteContextLogRule(id)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if !deleted {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "规则不存在"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

func parseIntQuery(c *gin.Context, key string) (int, error) {
	value := strings.TrimSpace(c.Query(key))
	if value == "" {
		return 0, nil
	}
	n, err := strconv.Atoi(value)
	if err != nil || n < 0 {
		return 0, errors.New(key + " 必须是非负整数")
	}
	return n, nil
}
func parseInt64Query(c *gin.Context, key string) (int64, error) {
	value := strings.TrimSpace(c.Query(key))
	if value == "" {
		return 0, nil
	}
	n, err := strconv.ParseInt(value, 10, 64)
	if err != nil || n < 0 {
		return 0, errors.New(key + " 必须是非负时间戳")
	}
	return n, nil
}
func contextLogFilter(c *gin.Context) (model.ContextRequestLogFilter, error) {
	var f model.ContextRequestLogFilter
	var err error
	if f.UserId, err = parseIntQuery(c, "user_id"); err != nil {
		return f, err
	}
	if f.ChannelId, err = parseIntQuery(c, "channel_id"); err != nil {
		return f, err
	}
	if f.StatusCode, err = parseIntQuery(c, "status"); err != nil {
		return f, err
	}
	if f.StartTime, err = parseInt64Query(c, "start_time"); err != nil {
		return f, err
	}
	if f.EndTime, err = parseInt64Query(c, "end_time"); err != nil {
		return f, err
	}
	f.Username = strings.TrimSpace(c.Query("username"))
	f.Model = strings.TrimSpace(c.Query("model"))
	f.RequestId = strings.TrimSpace(c.Query("request_id"))
	if f.StartTime > 0 && f.EndTime > 0 && f.StartTime > f.EndTime {
		return f, errors.New("start_time 不能晚于 end_time")
	}
	return f, nil
}
func ListContextRequestLogs(c *gin.Context) {
	filter, err := contextLogFilter(c)
	if err != nil {
		common.ApiErrorMsg(c, err.Error())
		return
	}
	page := common.GetPageQuery(c)
	items, total, err := model.ListContextRequestLogs(filter, page.GetStartIdx(), page.GetPageSize())
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"items": items, "total": total, "page": page.GetPage(), "page_size": page.GetPageSize()}})
}
func GetContextRequestLog(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		common.ApiErrorMsg(c, "无效的日志 ID")
		return
	}
	item, err := model.GetContextRequestLog(id)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "日志不存在"})
		return
	}
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": item})
}

type deleteContextLogsRequest struct {
	Ids []int `json:"ids"`
}

func DeleteContextRequestLog(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		common.ApiErrorMsg(c, "无效的日志 ID")
		return
	}
	deleteContextRequestLogs(c, []int{id})
}
func DeleteContextRequestLogs(c *gin.Context) {
	var req deleteContextLogsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiErrorMsg(c, "无效的日志 ID 列表")
		return
	}
	deleteContextRequestLogs(c, req.Ids)
}
func deleteContextRequestLogs(c *gin.Context, ids []int) {
	affected, err := model.DeleteContextRequestLogs(ids)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": err.Error()})
		return
	}
	if affected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "日志不存在"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"deleted": affected}})
}
