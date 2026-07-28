package model

import (
	"context"
	"errors"
	"sync/atomic"
	"time"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

type ContextRequestLog struct {
	Id                    int    `json:"id" gorm:"index:idx_qiqi_context_request_logs_created_at_id,priority:2;index:idx_qiqi_context_request_logs_user_id_id,priority:2"`
	UserId                int    `json:"user_id" gorm:"index;index:idx_qiqi_context_request_logs_user_id_id,priority:1"`
	CreatedAt             int64  `json:"created_at" gorm:"bigint;index:idx_qiqi_context_request_logs_created_at_id,priority:1"`
	RequestId             string `json:"request_id" gorm:"type:varchar(64);index;default:''"`
	Method                string `json:"method" gorm:"type:varchar(16);default:''"`
	Path                  string `json:"path"`
	Ip                    string `json:"ip" gorm:"index;default:''"`
	UserAgent             string `json:"user_agent"`
	Username              string `json:"username" gorm:"index;default:''"`
	TokenId               int    `json:"token_id" gorm:"default:0;index"`
	TokenName             string `json:"token_name" gorm:"index;default:''"`
	ModelName             string `json:"model_name" gorm:"index;default:''"`
	Group                 string `json:"group" gorm:"index"`
	IsStream              bool   `json:"is_stream"`
	StatusCode            int    `json:"status_code" gorm:"index;default:0"`
	LatencyMs             int64  `json:"latency_ms" gorm:"default:0"`
	Error                 string `json:"error" gorm:"default:''"`
	ChannelId             int    `json:"channel_id" gorm:"index;default:0"`
	ChannelName           string `json:"channel_name" gorm:"default:''"`
	ChannelType           int    `json:"channel_type" gorm:"index;default:0"`
	NodeName              string `json:"node_name" gorm:"index;default:''"`
	RuleId                int    `json:"rule_id" gorm:"index;default:0"`
	RuleName              string `json:"rule_name" gorm:"default:''"`
	DecisionSource        string `json:"decision_source" gorm:"type:varchar(32);default:''"`
	RequestHeaders        string `json:"request_headers"`
	ResponseHeaders       string `json:"response_headers"`
	RequestBody           string `json:"request_body"`
	RequestBodyEncoding   string `json:"request_body_encoding" gorm:"type:varchar(16);default:''"`
	RequestBodySize       int64  `json:"request_body_size" gorm:"default:0"`
	RequestBodyTruncated  bool   `json:"request_body_truncated" gorm:"default:false"`
	ResponseBody          string `json:"response_body"`
	ResponseBodyEncoding  string `json:"response_body_encoding" gorm:"type:varchar(16);default:''"`
	ResponseBodySize      int64  `json:"response_body_size" gorm:"default:0"`
	ResponseBodyTruncated bool   `json:"response_body_truncated" gorm:"default:false"`
}

func (ContextRequestLog) TableName() string { return "qiqi_context_request_logs" }

var contextRequestLogIdSequence atomic.Int64

func ensureContextRequestLogRequestId(log *ContextRequestLog) {
	if log == nil {
		return
	}
	if log.RequestId == "" {
		log.RequestId = common.NewRequestId()
	}
	if log.Id == 0 && common.UsingLogDatabase(common.DatabaseTypeClickHouse) {
		// ClickHouse has no auto increment. Millisecond time plus a process-local
		// sequence provides a stable detail/delete identity for dashboard rows.
		log.Id = int(time.Now().UnixMilli()*1000 + contextRequestLogIdSequence.Add(1)%1000)
	}
}
func RecordContextRequestLog(log *ContextRequestLog) error {
	if log == nil || LOG_DB == nil {
		return nil
	}
	ensureContextRequestLogRequestId(log)
	return LOG_DB.Create(log).Error
}

type ContextRequestLogFilter struct {
	UserId                     int
	Username, Model, RequestId string
	ChannelId, StatusCode      int
	StartTime, EndTime         int64
}

// ContextRequestLogListItem deliberately excludes headers and bodies.
type ContextRequestLogListItem struct {
	Id                    int    `json:"id"`
	UserId                int    `json:"user_id"`
	CreatedAt             int64  `json:"created_at"`
	RequestId             string `json:"request_id"`
	Method                string `json:"method"`
	Path                  string `json:"path"`
	Username              string `json:"username"`
	ModelName             string `json:"model_name"`
	IsStream              bool   `json:"is_stream"`
	StatusCode            int    `json:"status_code"`
	LatencyMs             int64  `json:"latency_ms"`
	Error                 string `json:"error"`
	ChannelId             int    `json:"channel_id"`
	ChannelName           string `json:"channel_name"`
	ChannelType           int    `json:"channel_type"`
	RuleId                int    `json:"rule_id"`
	RuleName              string `json:"rule_name"`
	DecisionSource        string `json:"decision_source"`
	RequestBodySize       int64  `json:"request_body_size"`
	RequestBodyTruncated  bool   `json:"request_body_truncated"`
	ResponseBodySize      int64  `json:"response_body_size"`
	ResponseBodyTruncated bool   `json:"response_body_truncated"`
}

func contextRequestLogFilteredQuery(filter ContextRequestLogFilter) *gorm.DB {
	q := LOG_DB.Model(&ContextRequestLog{})
	if filter.UserId > 0 {
		q = q.Where("user_id = ?", filter.UserId)
	}
	if filter.Username != "" {
		q = q.Where("username = ?", filter.Username)
	}
	if filter.Model != "" {
		q = q.Where("model_name = ?", filter.Model)
	}
	if filter.ChannelId > 0 {
		q = q.Where("channel_id = ?", filter.ChannelId)
	}
	if filter.StatusCode > 0 {
		q = q.Where("status_code = ?", filter.StatusCode)
	}
	if filter.RequestId != "" {
		q = q.Where("request_id = ?", filter.RequestId)
	}
	if filter.StartTime > 0 {
		q = q.Where("created_at >= ?", filter.StartTime)
	}
	if filter.EndTime > 0 {
		q = q.Where("created_at <= ?", filter.EndTime)
	}
	return q
}

func ListContextRequestLogs(filter ContextRequestLogFilter, offset, limit int) ([]ContextRequestLogListItem, int64, error) {
	var total int64
	q := contextRequestLogFilteredQuery(filter)
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var items []ContextRequestLogListItem
	order := "created_at desc, id desc"
	if common.UsingLogDatabase(common.DatabaseTypeClickHouse) {
		order = "created_at desc, request_id desc"
	}
	err := q.Select("id,user_id,created_at,request_id,method,path,username,model_name,is_stream,status_code,latency_ms,error,channel_id,channel_name,channel_type,rule_id,rule_name,decision_source,request_body_size,request_body_truncated,response_body_size,response_body_truncated").Order(order).Offset(offset).Limit(limit).Scan(&items).Error
	return items, total, err
}

func GetContextRequestLog(id int) (*ContextRequestLog, error) {
	var item ContextRequestLog
	var err error
	if common.UsingLogDatabase(common.DatabaseTypeClickHouse) {
		err = LOG_DB.Where("id = ?", id).Order("created_at desc").Limit(1).Take(&item).Error
	} else {
		err = LOG_DB.First(&item, id).Error
	}
	return &item, err
}

func CountContextRequestLogsBefore(ctx context.Context, cutoff int64) (int64, error) {
	if LOG_DB == nil {
		return 0, errors.New("log database is not initialized")
	}
	var count int64
	err := LOG_DB.WithContext(ctx).Model(&ContextRequestLog{}).Where("created_at < ?", cutoff).Count(&count).Error
	return count, err
}

func DeleteContextRequestLogsBeforeBatch(ctx context.Context, cutoff int64, limit int) (int64, error) {
	if LOG_DB == nil {
		return 0, errors.New("log database is not initialized")
	}
	if limit <= 0 {
		return 0, errors.New("batch size must be positive")
	}
	if common.UsingLogDatabase(common.DatabaseTypeClickHouse) {
		return 0, errors.New("ClickHouse context log retention must be applied with TTL")
	}

	// Select IDs first instead of deleting through a self-referencing subquery:
	// MySQL rejects some DELETE ... IN (SELECT ... FROM same_table) forms. The
	// bounded ID list is portable across SQLite, MySQL, and PostgreSQL. The
	// threshold is strict so rows exactly at the retention boundary are kept.
	var ids []int
	if err := LOG_DB.WithContext(ctx).Model(&ContextRequestLog{}).
		Where("created_at < ?", cutoff).
		Order("created_at asc, id asc").
		Limit(limit).
		Pluck("id", &ids).Error; err != nil {
		return 0, err
	}
	if len(ids) == 0 {
		return 0, nil
	}
	result := LOG_DB.WithContext(ctx).Where("id IN ?", ids).Delete(&ContextRequestLog{})
	return result.RowsAffected, result.Error
}

func DeleteContextRequestLogs(ids []int) (int64, error) {
	if common.UsingLogDatabase(common.DatabaseTypeClickHouse) {
		return 0, errors.New("ClickHouse 日志库不支持上下文日志逐条删除，请使用 TTL 生命周期管理")
	}
	if len(ids) == 0 {
		return 0, errors.New("至少选择一条日志")
	}
	result := LOG_DB.Where("id IN ?", ids).Delete(&ContextRequestLog{})
	return result.RowsAffected, result.Error
}
