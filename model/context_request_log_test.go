package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestListContextRequestLogsDoesNotExposeBodies(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&ContextRequestLog{}))
	require.NoError(t, db.Create(&ContextRequestLog{CreatedAt: 1, RequestId: "req", RequestBody: "private request", ResponseBody: "private response", RequestBodySize: 15, ResponseBodySize: 16}).Error)
	oldDB := LOG_DB
	oldType := common.LogDatabaseType()
	LOG_DB = db
	common.SetLogDatabaseType(common.DatabaseTypeSQLite)
	t.Cleanup(func() { LOG_DB = oldDB; common.SetLogDatabaseType(oldType) })

	items, total, err := ListContextRequestLogs(ContextRequestLogFilter{}, 0, 10)
	require.NoError(t, err)
	assert.Equal(t, int64(1), total)
	require.Len(t, items, 1)
	assert.Equal(t, int64(15), items[0].RequestBodySize)
	// Compile-time response type intentionally has no RequestBody/ResponseBody fields.
}
