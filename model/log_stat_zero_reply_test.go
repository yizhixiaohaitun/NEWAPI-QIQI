package model

import (
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupZeroReplyStatDB(t *testing.T) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file::memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&Log{}))
	oldDB := LOG_DB
	oldType := common.LogDatabaseType()
	LOG_DB = db
	common.SetLogDatabaseType(common.DatabaseTypeSQLite)
	t.Cleanup(func() { LOG_DB = oldDB; common.SetLogDatabaseType(oldType) })
}

func TestSumUsedQuotaCountsZeroReplyLogs(t *testing.T) {
	setupZeroReplyStatDB(t)

	now := time.Now().Unix()
	logs := []*Log{
		// Zero-reply consume logs: huge prompt, no completion, still billed.
		{Type: LogTypeConsume, CreatedAt: now, ModelName: "claude-3", Username: "alice", Quota: 5000, PromptTokens: 3191361, CompletionTokens: 0},
		{Type: LogTypeConsume, CreatedAt: now, ModelName: "claude-3", Username: "alice", Quota: 300, PromptTokens: 1200, CompletionTokens: 0},
		// Normal consume log: has completion tokens, must not be counted.
		{Type: LogTypeConsume, CreatedAt: now, ModelName: "claude-3", Username: "alice", Quota: 100, PromptTokens: 50, CompletionTokens: 20},
		// Zero prompt + zero completion (e.g. cached/free) must not be counted.
		{Type: LogTypeConsume, CreatedAt: now, ModelName: "claude-3", Username: "alice", Quota: 10, PromptTokens: 0, CompletionTokens: 0},
		// Non-consume log types must not be counted even when tokens match.
		{Type: LogTypeError, CreatedAt: now, ModelName: "claude-3", Username: "alice", Quota: 999, PromptTokens: 888, CompletionTokens: 0},
	}
	for _, l := range logs {
		require.NoError(t, LOG_DB.Create(l).Error)
	}

	stat, err := SumUsedQuota(LogTypeUnknown, 0, 0, "", "", "", 0, "")
	require.NoError(t, err)
	assert.Equal(t, 5410, stat.Quota, "quota should still sum all consume logs")
	assert.Equal(t, 2, stat.ZeroReplyCount)
	assert.Equal(t, 5300, stat.ZeroReplyQuota)
}

func TestSumUsedQuotaZeroReplyRespectsFilters(t *testing.T) {
	setupZeroReplyStatDB(t)

	now := time.Now().Unix()
	logs := []*Log{
		{Type: LogTypeConsume, CreatedAt: now, ModelName: "claude-3", Username: "alice", Quota: 500, PromptTokens: 1000, CompletionTokens: 0},
		{Type: LogTypeConsume, CreatedAt: now, ModelName: "gpt-4o", Username: "bob", Quota: 700, PromptTokens: 2000, CompletionTokens: 0},
	}
	for _, l := range logs {
		require.NoError(t, LOG_DB.Create(l).Error)
	}

	stat, err := SumUsedQuota(LogTypeUnknown, 0, 0, "claude-3", "alice", "", 0, "")
	require.NoError(t, err)
	assert.Equal(t, 500, stat.Quota)
	assert.Equal(t, 1, stat.ZeroReplyCount)
	assert.Equal(t, 500, stat.ZeroReplyQuota)
}

func TestSumUsedQuotaZeroReplyEmptyResult(t *testing.T) {
	setupZeroReplyStatDB(t)

	stat, err := SumUsedQuota(LogTypeUnknown, 0, 0, "", "", "", 0, "")
	require.NoError(t, err)
	assert.Equal(t, 0, stat.ZeroReplyCount)
	assert.Equal(t, 0, stat.ZeroReplyQuota)
}
