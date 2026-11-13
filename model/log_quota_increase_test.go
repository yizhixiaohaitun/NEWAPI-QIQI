package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupQuotaIncreaseLogDB(t *testing.T) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:quota-increase-log-"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&User{}, &Log{}))

	oldDB, oldLogDB := DB, LOG_DB
	oldLogType := common.LogDatabaseType()
	DB, LOG_DB = db, db
	common.SetLogDatabaseType(common.DatabaseTypeSQLite)
	t.Cleanup(func() {
		DB, LOG_DB = oldDB, oldLogDB
		common.SetLogDatabaseType(oldLogType)
	})
}

func TestQuotaIncreasePostgreSQLPredicateAvoidsUnsafeJSONCast(t *testing.T) {
	oldLogType := common.LogDatabaseType()
	t.Cleanup(func() { common.SetLogDatabaseType(oldLogType) })
	common.SetLogDatabaseType(common.DatabaseTypePostgreSQL)

	expression := logActionExpression("logs.other")
	assert.Contains(t, expression, "substring")
	assert.Contains(t, expression, "COALESCE(logs.other, '')")
	assert.NotContains(t, expression, "::jsonb", "invalid historical TEXT must not be cast directly")
	predicate, args := quotaIncreaseLogPredicate("logs.")
	assert.Contains(t, predicate, "logs.content LIKE ?")
	assert.Len(t, args, 2)
}

func TestQuotaIncreaseAuditTypeAndHistoricalFilters(t *testing.T) {
	setupQuotaIncreaseLogDB(t)
	require.NoError(t, DB.Create(&User{Id: 1, Username: "operator", AffCode: "operator-code"}).Error)
	require.NoError(t, DB.Create(&User{Id: 2, Username: "target", AffCode: "target-code"}).Error)

	adminInfo := map[string]interface{}{"admin_id": 1, "admin_username": "operator"}
	auditInfo := map[string]interface{}{"status": 200, "success": true, "request_id": "audit-request"}
	RecordOperationAuditLog(1, "Increased user quota by 10", "192.0.2.1", "user.quota_add", map[string]interface{}{
		"quota":          "10",
		"target_user_id": 2,
	}, adminInfo, auditInfo)
	RecordOperationAuditLog(1, "Decreased user quota by 5", "192.0.2.1", "user.quota_subtract", map[string]interface{}{"quota": "5", "target_user_id": 2}, adminInfo, auditInfo)
	RecordOperationAuditLog(1, "Overrode user quota from 10 to 20", "192.0.2.1", "user.quota_override", map[string]interface{}{"from": "10", "to": "20", "target_user_id": 2}, adminInfo, auditInfo)
	RecordOperationAuditLog(1, "Updated user target (ID: 2)", "192.0.2.1", "user.update", map[string]interface{}{"id": 2, "target_user_id": 2}, adminInfo, auditInfo)

	legacyAction := common.MapToJsonStr(map[string]interface{}{"op": map[string]interface{}{
		"action": "user.quota_add",
		"params": map[string]interface{}{"quota": "11", "target_user_id": 2},
	}})
	require.NoError(t, LOG_DB.Create(&Log{UserId: 1, Username: "operator", Type: LogTypeManage, Content: "Increased user quota by 11", Other: legacyAction}).Error)
	require.NoError(t, LOG_DB.Create(&Log{UserId: 1, Username: "operator", Type: LogTypeManage, Content: "Increased user quota by 12"}).Error)
	// A contradictory structured action must remain a management log even if
	// its old fallback text resembles a quota increase.
	contradictory := common.MapToJsonStr(map[string]interface{}{"op": map[string]interface{}{"action": "user.quota_subtract"}})
	require.NoError(t, LOG_DB.Create(&Log{UserId: 1, Username: "operator", Type: LogTypeManage, Content: "Increased user quota by 13", Other: contradictory}).Error)
	ordinaryAction := common.MapToJsonStr(map[string]interface{}{"op": map[string]interface{}{"action": "user.manage"}})
	require.NoError(t, LOG_DB.Create(&Log{UserId: 1, Username: "operator", Type: LogTypeManage, Content: "Performed disable on user target (ID: 2)", Other: ordinaryAction}).Error)
	// Historical malformed/empty JSON must not make either type filter fail.
	require.NoError(t, LOG_DB.Create(&Log{UserId: 1, Username: "operator", Type: LogTypeManage, Content: "Updated a user", Other: "not-json"}).Error)
	require.NoError(t, LOG_DB.Exec("INSERT INTO logs (user_id, username, type, content, other) VALUES (?, ?, ?, ?, NULL)", 1, "operator", LogTypeManage, "Updated another user").Error)
	require.NoError(t, LOG_DB.Create(&Log{UserId: 2, Username: "target", Type: LogTypeQuotaIncrease, Content: "Increased user quota by 99", Other: legacyAction}).Error)

	var createdQuotaLog Log
	require.NoError(t, LOG_DB.Where("type = ? AND content = ?", LogTypeQuotaIncrease, "Increased user quota by 10").First(&createdQuotaLog).Error)
	assert.NotEmpty(t, createdQuotaLog.RequestId, "audit logs keep a request id")
	assert.Contains(t, createdQuotaLog.Other, `"target_user_id":2`)

	logs, total, err := GetAllLogs(LogTypeQuotaIncrease, 0, 0, "", "", "", 0, 2, 0, "", "", "", false)
	require.NoError(t, err)
	assert.Equal(t, int64(4), total)
	assert.Len(t, logs, 2)
	logs, total, err = GetAllLogs(LogTypeQuotaIncrease, 0, 0, "", "", "", 2, 2, 0, "", "", "", false)
	require.NoError(t, err)
	assert.Equal(t, int64(4), total)
	assert.Len(t, logs, 2)

	_, total, err = GetAllLogs(LogTypeManage, 0, 0, "", "", "", 0, 100, 0, "", "", "", false)
	require.NoError(t, err)
	assert.Equal(t, int64(7), total, "management filter excludes current and both legacy quota-add rows")

	userLogs, userTotal, err := GetUserLogs(1, LogTypeQuotaIncrease, 0, 0, "", "", 0, 100, "", "", "", false)
	require.NoError(t, err)
	assert.Equal(t, int64(3), userTotal, "self log query excludes another user's quota increase")
	assert.Len(t, userLogs, 3)
	for _, log := range userLogs {
		assert.NotContains(t, log.Other, "admin_info")
		assert.NotContains(t, log.Other, "audit_info")
		if log.Other != "null" {
			assert.Contains(t, log.Other, `"op"`)
		}
	}

	// Management quota logs must not enter consumption accounting, regardless
	// of their type or stored quota value.
	require.NoError(t, LOG_DB.Model(&Log{}).Where("type = ?", LogTypeQuotaIncrease).Update("quota", 999).Error)
	stat, err := SumUsedQuota(LogTypeQuotaIncrease, 0, 0, "", "", "", 0, "", false)
	require.NoError(t, err)
	assert.Zero(t, stat.Quota)
}
