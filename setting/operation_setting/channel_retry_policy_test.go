package operation_setting

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestChannelRetryPolicyValidationAndFirstMatch(t *testing.T) {
	previous := channelRetryPolicy.Load()
	t.Cleanup(func() { channelRetryPolicy.Store(previous) })
	valid := `[{"channel_id":7,"status":429,"error_code":"RATE_LIMIT","message_contains":"Busy now","action":"stop"},{"status":429,"action":"retry"}]`
	require.NoError(t, SetChannelRetryPolicy(valid))
	check := func(channel, status int, code, message string, wantRetry, wantMatched bool) {
		t.Helper()
		retry, matched := MatchChannelRetryPolicy(channel, status, code, message)
		require.Equal(t, wantRetry, retry)
		require.Equal(t, wantMatched, matched)
	}
	check(7, 429, "rate_limit", "Server is BUSY NOW", false, true)
	check(8, 429, "rate_limit", "Server is BUSY NOW", true, true)
	check(7, 429, "other", "Server is BUSY NOW", true, true)
	check(7, 429, "rate_limit", "Server is merely busy", true, true)
	check(7, 500, "rate_limit", "Server is BUSY NOW", false, false)

	for _, invalid := range []string{
		`null`, `{}`, `[null]`, `[] []`, `[{"status":429}]`,
		`[{"channel_id":1,"action":"retry"}]`, `[{"status":200,"action":"retry"}]`,
		`[{"channel_id":0,"status":429,"action":"retry"}]`,
		`[{"status":429,"error_code":" ","action":"retry"}]`,
		`[{"channel_id":null,"status":429,"action":"retry"}]`,
		`[{"status":"429","action":"retry"}]`, `[{"status":429,"action":"other"}]`,
		`[{"status":429,"action":"retry","typo":true}]`,
		`[{"status":429,"action":"retry"},{}]`,
		`[` + strings.Repeat(`{"status":429,"action":"retry"},`, 128) + `{"status":429,"action":"retry"}]`,
	} {
		require.Error(t, SetChannelRetryPolicy(invalid), invalid)
		check(7, 429, "rate_limit", "Server is BUSY NOW", false, true)
	}
	require.NoError(t, SetChannelRetryPolicy(DefaultChannelRetryPolicy))
	check(7, 429, "rate_limit", "Server is BUSY NOW", false, false)
}

func TestChannelRetryPolicyStreamHedge(t *testing.T) {
	previous := channelRetryPolicy.Load()
	t.Cleanup(func() { channelRetryPolicy.Store(previous) })
	assertHedge := func(delay int, same, configured bool) {
		t.Helper()
		gotDelay, gotSame, _, gotConfigured := GetChannelRetryStreamHedge("claude-opus-5-5")
		require.Equal(t, delay, gotDelay)
		require.Equal(t, same, gotSame)
		require.Equal(t, configured, gotConfigured)
	}
	require.NoError(t, SetChannelRetryPolicy(`[{"status":429,"action":"stop"},{"stream_hedge":{"delay_ms":15000,"retry_same_channel_first":true}},{"status":429,"action":"retry"}]`))
	assertHedge(15000, true, true)
	retry, matched := MatchChannelRetryPolicy(1, 429, "", "")
	require.True(t, matched)
	require.False(t, retry)
	for _, invalid := range []string{
		`[{"stream_hedge":null}]`, `[{"stream_hedge":{}}]`, `[{"stream_hedge":{"delay_ms":null}}]`,
		`[{"stream_hedge":{"delay_ms":"15000"}}]`, `[{"stream_hedge":{"delay_ms":15.1}}]`,
		`[{"stream_hedge":{"delay_ms":-1}}]`, `[{"stream_hedge":{"delay_ms":300001}}]`,
		`[{"stream_hedge":{"delay_ms":15000,"retry_same_channel_first":null}}]`,
		`[{"stream_hedge":{"delay_ms":15000,"retry_same_channel_first":"true"}}]`,
		`[{"stream_hedge":{"delay_ms":15000,"unknown":true}}]`,
		`[{"stream_hedge":{"delay_ms":15000},"status":429}]`,
		`[{"stream_hedge":{"delay_ms":15000}},{"stream_hedge":{"delay_ms":0}}]`,
	} {
		require.Error(t, SetChannelRetryPolicy(invalid), invalid)
		assertHedge(15000, true, true)
	}
	require.NoError(t, SetChannelRetryPolicy(`[{"stream_hedge":{"delay_ms":0}}]`))
	assertHedge(0, false, true)
	require.NoError(t, SetChannelRetryPolicy(`[]`))
	assertHedge(0, false, false)
}

func TestChannelRetryPolicyStreamHedgeModels(t *testing.T) {
	previous := channelRetryPolicy.Load()
	t.Cleanup(func() { channelRetryPolicy.Store(previous) })
	const fable = "claude-fable-5-2"
	const opus = "claude-opus-5-5"
	const policy = `[{"status":429,"action":"stop"},{"stream_hedge":{"delay_ms":15000,"retry_same_channel_first":true,"models":["claude-fable-5-2"]}}]`
	require.NoError(t, SetChannelRetryPolicy(policy))
	delay, same, _, configured := GetChannelRetryStreamHedge(fable)
	require.Equal(t, 15000, delay)
	require.True(t, same)
	require.True(t, configured)
	delay, _, _, configured = GetChannelRetryStreamHedge(opus)
	require.Zero(t, delay)
	require.True(t, configured, "a model excluded by JSON must not use the legacy fallback")

	for _, invalid := range []string{`null`, `"claude-fable-5-2"`, `[null]`, `[" "]`, `[1]`} {
		require.Error(t, SetChannelRetryPolicy(`[{"stream_hedge":{"delay_ms":10,"models":`+invalid+`}}]`))
		delay, _, _, _ = GetChannelRetryStreamHedge(fable)
		require.Equal(t, 15000, delay)
	}
	retry, matched := MatchChannelRetryPolicy(1, 429, "", "")
	require.False(t, retry)
	require.True(t, matched)

	require.NoError(t, SetChannelRetryPolicy(`[{"stream_hedge":{"delay_ms":15000,"models":[]}}]`))
	delay, _, _, configured = GetChannelRetryStreamHedge(fable)
	require.Zero(t, delay)
	require.True(t, configured)

	require.NoError(t, SetChannelRetryPolicy(`[{"stream_hedge":{"delay_ms":15000}}]`))
	for _, model := range []string{fable, opus, "custom-native-claude"} {
		delay, _, _, configured = GetChannelRetryStreamHedge(model)
		require.Equal(t, 15000, delay)
		require.True(t, configured)
	}
}

func TestChannelRetryPolicyStreamHedgeMaxBody(t *testing.T) {
	previous := channelRetryPolicy.Load()
	t.Cleanup(func() { channelRetryPolicy.Store(previous) })
	const defaultBytes = int64(DefaultStreamHedgeMaxBodyMB) << 20
	require.NoError(t, SetChannelRetryPolicy(`[]`))
	require.Equal(t, defaultBytes, GetStreamHedgeMaxBodyBytes())
	require.NoError(t, SetChannelRetryPolicy(`[{"stream_hedge":{"delay_ms":5000,"retry_same_channel_first":true}}]`))
	require.Equal(t, defaultBytes, GetStreamHedgeMaxBodyBytes(), "omitting max_body_mb uses the default")
	require.Greater(t, GetStreamHedgeMaxBodyBytes(), int64(1336357), "the reported 1.3 MB request fits the default")

	const custom = `[{"status":429,"action":"stop"},{"stream_hedge":{"delay_ms":5000,"retry_same_channel_first":true,"max_body_mb":8}}]`
	require.NoError(t, SetChannelRetryPolicy(custom))
	require.Equal(t, int64(8)<<20, GetStreamHedgeMaxBodyBytes())
	for _, invalid := range []string{`0`, `-1`, `129`, `1.5`, `"8"`, `null`, `true`} {
		require.Error(t, SetChannelRetryPolicy(`[{"stream_hedge":{"delay_ms":5000,"max_body_mb":`+invalid+`}}]`), invalid)
		require.Equal(t, int64(8)<<20, GetStreamHedgeMaxBodyBytes(), "an invalid value must keep the saved limit")
	}
	retry, matched := MatchChannelRetryPolicy(1, 429, "", "")
	require.True(t, matched)
	require.False(t, retry)
	require.NoError(t, SetChannelRetryPolicy(`[{"stream_hedge":{"delay_ms":5000,"max_body_mb":128}}]`))
	require.Equal(t, int64(128)<<20, GetStreamHedgeMaxBodyBytes())
	require.NoError(t, SetChannelRetryPolicy(`[{"stream_hedge":{"delay_ms":5000,"max_body_mb":1}}]`))
	require.Equal(t, int64(1)<<20, GetStreamHedgeMaxBodyBytes())
	require.NoError(t, SetChannelRetryPolicy(DefaultChannelRetryPolicy))
	require.Equal(t, defaultBytes, GetStreamHedgeMaxBodyBytes())
}

func TestChannelRetryPolicyParallelBackups(t *testing.T) {
	previous := channelRetryPolicy.Load()
	t.Cleanup(func() { channelRetryPolicy.Store(previous) })
	const enabled = `[{"status":429,"action":"stop"},{"stream_hedge":{"delay_ms":15000,"retry_same_channel_first":true,"parallel_backup_models":["claude-fable-5-2"]}}]`
	require.NoError(t, SetChannelRetryPolicy(enabled))
	for _, value := range []string{`null`, `true`, `[null]`, `[" "]`} {
		require.Error(t, SetChannelRetryPolicy(`[{"stream_hedge":{"delay_ms":15000,"retry_same_channel_first":true,"parallel_backup_models":`+value+`}}]`))
	}
	require.Error(t, SetChannelRetryPolicy(`[{"stream_hedge":{"delay_ms":15000,"parallel_backup_models":["claude-fable-5-2"]}}]`))
	require.Error(t, SetChannelRetryPolicy(`[{"stream_hedge":{"delay_ms":15000,"retry_same_channel_first":true,"models":[],"parallel_backup_models":["claude-fable-5-2"]}}]`))
	delay, same, parallel, configured := GetChannelRetryStreamHedge("claude-fable-5-2")
	require.Equal(t, 15000, delay)
	require.True(t, same && configured)
	require.Equal(t, StreamHedgeParallelClaude, parallel)
	delay, same, parallel, configured = GetChannelRetryStreamHedge("claude-opus-5-5")
	require.Equal(t, 15000, delay)
	require.True(t, same && configured)
	require.Equal(t, StreamHedgeParallelOff, parallel)
	retry, matched := MatchChannelRetryPolicy(1, 429, "", "")
	require.True(t, matched)
	require.False(t, retry)
	require.NoError(t, SetChannelRetryPolicy(`[{"stream_hedge":{"delay_ms":15000,"retry_same_channel_first":true,"parallel_backup_models":[]}}]`))
	_, _, parallel, _ = GetChannelRetryStreamHedge("claude-fable-5-2")
	require.Equal(t, StreamHedgeParallelOff, parallel)
}

func TestChannelRetryPolicyHotReloadParallelBackupScope(t *testing.T) {
	previous := channelRetryPolicy.Load()
	t.Cleanup(func() { channelRetryPolicy.Store(previous) })
	const prefix = `[{"status":429,"action":"stop"},{"stream_hedge":{"delay_ms":15000,"retry_same_channel_first":true,`
	const end = `}}]`
	check := func(model string, delay int, scope StreamHedgeParallelScope) {
		t.Helper()
		gotDelay, _, gotScope, configured := GetChannelRetryStreamHedge(model)
		require.True(t, configured)
		require.Equal(t, delay, gotDelay)
		require.Equal(t, scope, gotScope)
	}
	for _, tc := range []struct {
		name string
		mode StreamHedgeParallelScope
	}{
		{"claude", StreamHedgeParallelClaude},
		{"all", StreamHedgeParallelAll},
	} {
		require.NoError(t, SetChannelRetryPolicy(prefix+`"parallel_backup_scope":"`+tc.name+`"`+end))
		check("unlisted-model-or-alias", 15000, tc.mode)
	}
	for _, invalid := range []string{
		prefix + `"parallel_backup_scope":null` + end,
		prefix + `"parallel_backup_scope":true` + end,
		prefix + `"parallel_backup_scope":"*"` + end,
		prefix + `"parallel_backup_scope":""` + end,
		prefix + `"parallel_backup_scope":"all","parallel_backup_models":[]` + end,
		`[{"stream_hedge":{"delay_ms":15000,"parallel_backup_scope":"all"}}]`,
	} {
		require.Error(t, SetChannelRetryPolicy(invalid), invalid)
		check("unlisted-model-or-alias", 15000, StreamHedgeParallelAll)
	}
	retry, matched := MatchChannelRetryPolicy(1, 429, "", "")
	require.True(t, matched)
	require.False(t, retry)
	require.NoError(t, SetChannelRetryPolicy(prefix+`"parallel_backup_scope":"all","models":["a"]`+end))
	check("a", 15000, StreamHedgeParallelAll)
	check("b", 0, StreamHedgeParallelOff)
	require.NoError(t, SetChannelRetryPolicy(`[{"stream_hedge":{"delay_ms":0,"retry_same_channel_first":true,"parallel_backup_scope":"all"}}]`))
	check("a", 0, StreamHedgeParallelAll)
	require.NoError(t, SetChannelRetryPolicy(`[{"stream_hedge":{"delay_ms":15000,"retry_same_channel_first":true}}]`))
	check("a", 15000, StreamHedgeParallelOff)
}
