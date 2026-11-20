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
