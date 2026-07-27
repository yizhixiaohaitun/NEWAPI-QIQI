package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestMigrateContextRequestLogOnSharedMainDB(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.Exec("CREATE TABLE qiqi_context_request_logs (id integer PRIMARY KEY)").Error)

	oldDB := DB
	oldMainType := common.MainDatabaseType()
	DB = db
	common.SetMainDatabaseType(common.DatabaseTypeSQLite)
	t.Setenv("LOG_SQL_DSN", "")
	t.Cleanup(func() {
		DB = oldDB
		common.SetMainDatabaseType(oldMainType)
	})

	require.NoError(t, migrateContextRequestLogOnMainDB())
	require.True(t, db.Migrator().HasColumn(&ContextRequestLog{}, "RuleId"))
	require.True(t, db.Migrator().HasColumn(&ContextRequestLog{}, "DecisionSource"))
	require.True(t, db.Migrator().HasColumn(&ContextRequestLog{}, "RequestBodyTruncated"))
}

func TestMigrateContextRequestLogOnMainDBSkipsIndependentLogDB(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.Exec("CREATE TABLE qiqi_context_request_logs (id integer PRIMARY KEY)").Error)

	oldDB := DB
	DB = db
	t.Setenv("LOG_SQL_DSN", "postgres://independent-log-db")
	t.Cleanup(func() { DB = oldDB })

	require.NoError(t, migrateContextRequestLogOnMainDB())
	require.False(t, db.Migrator().HasColumn(&ContextRequestLog{}, "RuleId"))
}
