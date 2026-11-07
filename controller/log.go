package controller

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"github.com/gin-gonic/gin"
)

func asString(v interface{}) string { s, _ := v.(string); return s }

// sanitizeAdminLogIdentity only changes the response object, never the stored log.
func sanitizeAdminLogIdentity(log *model.Log) {
	names := map[string]string{}
	if log.Username != "" {
		names[log.Username] = strconv.Itoa(log.UserId)
	}
	var other interface{}
	valid := log.Other == "" || json.Unmarshal([]byte(log.Other), &other) == nil
	var collect func(interface{})
	collect = func(v interface{}) {
		switch x := v.(type) {
		case map[string]interface{}:
			for k, value := range x {
				key := strings.ToLower(k)
				if s, ok := value.(string); ok && s != "" && (key == "username" || key == "display_name" || key == "admin_username" || key == "operator_username") {
					replacement := ""
					for _, idKey := range []string{"target_user_id", "id", "admin_id", "user_id"} {
						if id, ok := x[idKey].(float64); ok {
							replacement = strconv.FormatInt(int64(id), 10)
							break
						}
					}
					names[s] = replacement
				}
				collect(value)
			}
		case []interface{}:
			for _, value := range x {
				collect(value)
			}
		}
	}
	collect(other)
	replace := func(s string) string {
		for name, id := range names {
			s = strings.ReplaceAll(s, name, id)
		}
		return s
	}
	var clean func(interface{}) interface{}
	clean = func(v interface{}) interface{} {
		switch x := v.(type) {
		case string:
			return x
		case map[string]interface{}:
			for k, value := range x {
				if _, ok := names[asString(value)]; ok && (strings.EqualFold(k, "username") || strings.EqualFold(k, "display_name") || strings.EqualFold(k, "admin_username") || strings.EqualFold(k, "operator_username")) {
					x[k] = names[asString(value)]
				} else {
					x[k] = clean(value)
				}
			}
		case []interface{}:
			for i, value := range x {
				x[i] = clean(value)
			}
		}
		return v
	}
	log.Username = strconv.Itoa(log.UserId)
	// Only locally generated audit templates may be reconstructed; upstream errors
	// and arbitrary content are deliberately preserved verbatim.
	if log.Type == model.LogTypeManage {
		if obj, ok := other.(map[string]interface{}); ok {
			if op, ok := obj["op"].(map[string]interface{}); ok {
				if action, ok := op["action"].(string); ok {
					if params, ok := op["params"].(map[string]interface{}); ok && strings.HasPrefix(action, "user.") {
						if _, known := auditContentTemplates[action]; known && log.Content == auditContentEN(action, params) {
							log.Content = replace(log.Content)
						}
					}
				}
			}
		}
	}
	if !valid {
		log.Other = ""
	} else if log.Other != "" {
		b, err := json.Marshal(clean(other))
		if err != nil {
			log.Other = ""
		} else {
			log.Other = string(b)
		}
	}
}

// parseZeroReplyQuery 解析「0回复」伪类型筛选参数（zero_reply=1/true）。
// 它不是真实的 log type 枚举值：前端选中「0回复」时额外携带该参数，
// 后端据此把结果限定为 type=消耗 且 prompt_tokens>0 且 completion_tokens=0 的日志。
func parseZeroReplyQuery(c *gin.Context) bool {
	v := c.Query("zero_reply")
	return v == "1" || v == "true"
}

func adminLogUserFilter(c *gin.Context, username string) (string, []int) {
	if c.GetInt("role") == common.RoleRootUser || username == "" {
		return username, nil
	}
	id, err := strconv.Atoi(username)
	if err != nil || id <= 0 {
		id = -1
	}
	return "", []int{id}
}

func GetAllLogs(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)
	logType, _ := strconv.Atoi(c.Query("type"))
	startTimestamp, _ := strconv.ParseInt(c.Query("start_timestamp"), 10, 64)
	endTimestamp, _ := strconv.ParseInt(c.Query("end_timestamp"), 10, 64)
	username, userID := adminLogUserFilter(c, c.Query("username"))
	tokenName := c.Query("token_name")
	modelName := c.Query("model_name")
	channel, _ := strconv.Atoi(c.Query("channel"))
	group := c.Query("group")
	requestId := c.Query("request_id")
	upstreamRequestId := c.Query("upstream_request_id")
	zeroReply := parseZeroReplyQuery(c)
	logs, total, err := model.GetAllLogs(logType, startTimestamp, endTimestamp, modelName, username, tokenName, pageInfo.GetStartIdx(), pageInfo.GetPageSize(), channel, group, requestId, upstreamRequestId, zeroReply, userID...)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if c.GetInt("role") != common.RoleRootUser {
		for _, log := range logs {
			sanitizeAdminLogIdentity(log)
		}
	}
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(logs)
	common.ApiSuccess(c, pageInfo)
	return
}

func GetUserLogs(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)
	userId := c.GetInt("id")
	logType, _ := strconv.Atoi(c.Query("type"))
	startTimestamp, _ := strconv.ParseInt(c.Query("start_timestamp"), 10, 64)
	endTimestamp, _ := strconv.ParseInt(c.Query("end_timestamp"), 10, 64)
	tokenName := c.Query("token_name")
	modelName := c.Query("model_name")
	group := c.Query("group")
	requestId := c.Query("request_id")
	upstreamRequestId := c.Query("upstream_request_id")
	zeroReply := parseZeroReplyQuery(c)
	logs, total, err := model.GetUserLogs(userId, logType, startTimestamp, endTimestamp, modelName, tokenName, pageInfo.GetStartIdx(), pageInfo.GetPageSize(), group, requestId, upstreamRequestId, zeroReply)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(logs)
	common.ApiSuccess(c, pageInfo)
	return
}

// Deprecated: SearchAllLogs 已废弃，前端未使用该接口。
func SearchAllLogs(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"success": false,
		"message": "该接口已废弃",
	})
}

// Deprecated: SearchUserLogs 已废弃，前端未使用该接口。
func SearchUserLogs(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"success": false,
		"message": "该接口已废弃",
	})
}

func GetLogByKey(c *gin.Context) {
	tokenId := c.GetInt("token_id")
	if tokenId == 0 {
		c.JSON(200, gin.H{
			"success": false,
			"message": "无效的令牌",
		})
		return
	}
	logs, err := model.GetLogByTokenId(tokenId)
	if err != nil {
		c.JSON(200, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}
	c.JSON(200, gin.H{
		"success": true,
		"message": "",
		"data":    logs,
	})
}

func GetLogsStat(c *gin.Context) {
	logType, _ := strconv.Atoi(c.Query("type"))
	startTimestamp, _ := strconv.ParseInt(c.Query("start_timestamp"), 10, 64)
	endTimestamp, _ := strconv.ParseInt(c.Query("end_timestamp"), 10, 64)
	tokenName := c.Query("token_name")
	username, userID := adminLogUserFilter(c, c.Query("username"))
	modelName := c.Query("model_name")
	channel, _ := strconv.Atoi(c.Query("channel"))
	group := c.Query("group")
	zeroReply := parseZeroReplyQuery(c)
	stat, err := model.SumUsedQuota(logType, startTimestamp, endTimestamp, modelName, username, tokenName, channel, group, zeroReply, userID...)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	//tokenNum := model.SumUsedToken(logType, startTimestamp, endTimestamp, modelName, username, "")
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data": gin.H{
			"quota":            stat.Quota,
			"rpm":              stat.Rpm,
			"tpm":              stat.Tpm,
			"zero_reply_count": stat.ZeroReplyCount,
			"zero_reply_quota": stat.ZeroReplyQuota,
		},
	})
	return
}

func GetLogsSelfStat(c *gin.Context) {
	username := c.GetString("username")
	logType, _ := strconv.Atoi(c.Query("type"))
	startTimestamp, _ := strconv.ParseInt(c.Query("start_timestamp"), 10, 64)
	endTimestamp, _ := strconv.ParseInt(c.Query("end_timestamp"), 10, 64)
	tokenName := c.Query("token_name")
	modelName := c.Query("model_name")
	channel, _ := strconv.Atoi(c.Query("channel"))
	group := c.Query("group")
	zeroReply := parseZeroReplyQuery(c)
	quotaNum, err := model.SumUsedQuota(logType, startTimestamp, endTimestamp, modelName, username, tokenName, channel, group, zeroReply)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	//tokenNum := model.SumUsedToken(logType, startTimestamp, endTimestamp, modelName, username, tokenName)
	c.JSON(200, gin.H{
		"success": true,
		"message": "",
		"data": gin.H{
			"quota":            quotaNum.Quota,
			"rpm":              quotaNum.Rpm,
			"tpm":              quotaNum.Tpm,
			"zero_reply_count": quotaNum.ZeroReplyCount,
			"zero_reply_quota": quotaNum.ZeroReplyQuota,
			//"token": tokenNum,
		},
	})
	return
}

// DeleteHistoryLogs is the legacy synchronous log cleanup endpoint (DELETE /api/log/).
// It deletes directly instead of going through the async system task. It is kept only
// for the classic frontend; the default frontend uses POST /api/system-task/log-cleanup.
// TODO: remove this handler (and its route) once the classic frontend is removed.
func DeleteHistoryLogs(c *gin.Context) {
	targetTimestamp, _ := strconv.ParseInt(c.Query("target_timestamp"), 10, 64)
	if targetTimestamp == 0 {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "target timestamp is required",
		})
		return
	}
	count, err := model.DeleteOldLog(c.Request.Context(), targetTimestamp, 100)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    count,
	})
	return
}
