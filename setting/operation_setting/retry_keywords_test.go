package operation_setting

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAutomaticRetryKeywords(t *testing.T) {
	original := AutomaticRetryKeywordsToString()
	t.Cleanup(func() { AutomaticRetryKeywordsFromString(original) })

	flagged := "Invalid prompt: your prompt was flagged as potentially violating our usage policy. Please try again with a different prompt: https://example.com/x"
	assert.True(t, ShouldRetryByErrorMessage(flagged), "default list must cover the flagged-prompt 400")
	assert.False(t, ShouldRetryByErrorMessage("invalid model"))

	AutomaticRetryKeywordsFromString("  Foo Bar \r\n\n foo bar\nBAZ ")
	assert.Equal(t, "foo bar\nbaz", AutomaticRetryKeywordsToString())
	assert.True(t, ShouldRetryByErrorMessage("", "xx FOO BAR yy"))
	assert.True(t, ShouldRetryByErrorMessage("baz"))
	assert.False(t, ShouldRetryByErrorMessage(flagged), "replacing the list drops the default")

	AutomaticRetryKeywordsFromString("")
	assert.False(t, ShouldRetryByErrorMessage("anything"))
}
