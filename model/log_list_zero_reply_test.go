package model

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// seedZeroReplyListLogs inserts a mixed set of logs for list-filter tests:
// user 1: 3 zero-reply consume logs (claude-3 x2, gpt-4o x1), 1 normal consume,
//
//	1 error log with zero completion, 1 zero-prompt consume
//
// user 2: 1 zero-reply consume log
func seedZeroReplyListLogs(t *testing.T) {
	t.Helper()
	now := time.Now().Unix()
	logs := []*Log{
		{UserId: 1, Type: LogTypeConsume, CreatedAt: now, ModelName: "claude-3", Username: "alice", TokenName: "tk1", Quota: 5000, PromptTokens: 3191361, CompletionTokens: 0},
		{UserId: 1, Type: LogTypeConsume, CreatedAt: now, ModelName: "claude-3", Username: "alice", TokenName: "tk2", Quota: 300, PromptTokens: 1200, CompletionTokens: 0},
		{UserId: 1, Type: LogTypeConsume, CreatedAt: now, ModelName: "gpt-4o", Username: "alice", TokenName: "tk1", Quota: 200, PromptTokens: 800, CompletionTokens: 0},
		{UserId: 1, Type: LogTypeConsume, CreatedAt: now, ModelName: "claude-3", Username: "alice", TokenName: "tk1", Quota: 100, PromptTokens: 50, CompletionTokens: 20},
		{UserId: 1, Type: LogTypeError, CreatedAt: now, ModelName: "claude-3", Username: "alice", TokenName: "tk1", Quota: 0, PromptTokens: 999, CompletionTokens: 0},
		{UserId: 1, Type: LogTypeConsume, CreatedAt: now, ModelName: "claude-3", Username: "alice", TokenName: "tk1", Quota: 10, PromptTokens: 0, CompletionTokens: 0},
		{UserId: 2, Type: LogTypeConsume, CreatedAt: now, ModelName: "claude-3", Username: "bob", TokenName: "tk3", Quota: 400, PromptTokens: 2000, CompletionTokens: 0},
	}
	for _, l := range logs {
		require.NoError(t, LOG_DB.Create(l).Error)
	}
}

func TestGetAllLogsZeroReplyFilter(t *testing.T) {
	setupZeroReplyStatDB(t)
	seedZeroReplyListLogs(t)

	// zeroReply forces type=consume + prompt>0 + completion=0; the passed
	// logType must be ignored (frontend still sends type=0 for the pseudo type).
	logs, total, err := GetAllLogs(LogTypeUnknown, 0, 0, "", "", "", 0, 100, 0, "", "", "", true)
	require.NoError(t, err)
	assert.Equal(t, int64(4), total)
	assert.Len(t, logs, 4)
	for _, l := range logs {
		assert.Equal(t, LogTypeConsume, l.Type)
		assert.Greater(t, l.PromptTokens, 0)
		assert.Equal(t, 0, l.CompletionTokens)
	}
}

func TestGetAllLogsZeroReplyCombinesWithFilters(t *testing.T) {
	setupZeroReplyStatDB(t)
	seedZeroReplyListLogs(t)

	// model filter
	logs, total, err := GetAllLogs(LogTypeUnknown, 0, 0, "claude-3", "", "", 0, 100, 0, "", "", "", true)
	require.NoError(t, err)
	assert.Equal(t, int64(3), total)
	assert.Len(t, logs, 3)

	// username filter
	_, total, err = GetAllLogs(LogTypeUnknown, 0, 0, "", "bob", "", 0, 100, 0, "", "", "", true)
	require.NoError(t, err)
	assert.Equal(t, int64(1), total)

	// token filter
	_, total, err = GetAllLogs(LogTypeUnknown, 0, 0, "", "", "tk2", 0, 100, 0, "", "", "", true)
	require.NoError(t, err)
	assert.Equal(t, int64(1), total)

	// time range excluding everything
	_, total, err = GetAllLogs(LogTypeUnknown, 1, 2, "", "", "", 0, 100, 0, "", "", "", true)
	require.NoError(t, err)
	assert.Equal(t, int64(0), total)
}

func TestGetAllLogsZeroReplyPagination(t *testing.T) {
	setupZeroReplyStatDB(t)
	seedZeroReplyListLogs(t)

	// page size 3: first page has 3 rows, second page has 1, total stays 4
	logs, total, err := GetAllLogs(LogTypeUnknown, 0, 0, "", "", "", 0, 3, 0, "", "", "", true)
	require.NoError(t, err)
	assert.Equal(t, int64(4), total)
	assert.Len(t, logs, 3)

	logs, total, err = GetAllLogs(LogTypeUnknown, 0, 0, "", "", "", 3, 3, 0, "", "", "", true)
	require.NoError(t, err)
	assert.Equal(t, int64(4), total)
	assert.Len(t, logs, 1)
}

func TestGetAllLogsWithoutZeroReplyUnchanged(t *testing.T) {
	setupZeroReplyStatDB(t)
	seedZeroReplyListLogs(t)

	// zeroReply=false keeps the legacy behaviour: type filter works as before
	_, total, err := GetAllLogs(LogTypeUnknown, 0, 0, "", "", "", 0, 100, 0, "", "", "", false)
	require.NoError(t, err)
	assert.Equal(t, int64(7), total)

	_, total, err = GetAllLogs(LogTypeConsume, 0, 0, "", "", "", 0, 100, 0, "", "", "", false)
	require.NoError(t, err)
	assert.Equal(t, int64(6), total)
}

func TestGetUserLogsZeroReplyFilter(t *testing.T) {
	setupZeroReplyStatDB(t)
	seedZeroReplyListLogs(t)

	// user 1 sees only their own zero-reply logs
	logs, total, err := GetUserLogs(1, LogTypeUnknown, 0, 0, "", "", 0, 100, "", "", "", true)
	require.NoError(t, err)
	assert.Equal(t, int64(3), total)
	assert.Len(t, logs, 3)
	for _, l := range logs {
		assert.Equal(t, LogTypeConsume, l.Type)
		assert.Greater(t, l.PromptTokens, 0)
		assert.Equal(t, 0, l.CompletionTokens)
	}

	// combined with model filter + pagination
	logs, total, err = GetUserLogs(1, LogTypeUnknown, 0, 0, "claude-3", "", 0, 1, "", "", "", true)
	require.NoError(t, err)
	assert.Equal(t, int64(2), total)
	assert.Len(t, logs, 1)

	// user 2
	_, total, err = GetUserLogs(2, LogTypeUnknown, 0, 0, "", "", 0, 100, "", "", "", true)
	require.NoError(t, err)
	assert.Equal(t, int64(1), total)
}
