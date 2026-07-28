package model

import (
	"context"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestDeleteContextRequestLogsBeforeBatchOnlyDeletesOldContextLogs(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&ContextRequestLog{}, &Log{}))
	original := LOG_DB
	LOG_DB = db
	t.Cleanup(func() { LOG_DB = original })

	cutoff := int64(1_000)
	require.NoError(t, db.Create(&ContextRequestLog{CreatedAt: 100, RequestId: "old-1"}).Error)
	require.NoError(t, db.Create(&ContextRequestLog{CreatedAt: 200, RequestId: "old-2"}).Error)
	require.NoError(t, db.Create(&ContextRequestLog{CreatedAt: cutoff, RequestId: "boundary"}).Error)
	require.NoError(t, db.Create(&ContextRequestLog{CreatedAt: 1_100, RequestId: "new"}).Error)
	require.NoError(t, db.Create(&Log{CreatedAt: 100, Content: "ordinary log"}).Error)

	deleted, err := DeleteContextRequestLogsBeforeBatch(context.Background(), cutoff, 1)
	require.NoError(t, err)
	assert.EqualValues(t, 1, deleted)
	remainingOld, err := CountContextRequestLogsBefore(context.Background(), cutoff)
	require.NoError(t, err)
	assert.EqualValues(t, 1, remainingOld)

	deleted, err = DeleteContextRequestLogsBeforeBatch(context.Background(), cutoff, 10)
	require.NoError(t, err)
	assert.EqualValues(t, 1, deleted)
	var requestIDs []string
	require.NoError(t, db.Model(&ContextRequestLog{}).Order("created_at").Pluck("request_id", &requestIDs).Error)
	assert.Equal(t, []string{"boundary", "new"}, requestIDs)
	var ordinaryLogs int64
	require.NoError(t, db.Model(&Log{}).Count(&ordinaryLogs).Error)
	assert.EqualValues(t, 1, ordinaryLogs)
}
