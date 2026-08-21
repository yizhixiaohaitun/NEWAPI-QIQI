package relay

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSummarizeTaskUpstreamErrorHidesSPAHTML(t *testing.T) {
	summary := summarizeTaskUpstreamError(
		"text/html; charset=utf-8",
		[]byte(`<!doctype html><html><body><script>large frontend bundle</script></body></html>`),
	)

	assert.Equal(t, "HTML page (possible wrong API path or SPA fallback)", summary)
	assert.NotContains(t, summary, "frontend bundle")
}

func TestSummarizeTaskUpstreamErrorTruncatesLargeBody(t *testing.T) {
	summary := summarizeTaskUpstreamError("application/json", []byte(strings.Repeat("x", 300)))

	assert.Len(t, summary, 243)
	assert.True(t, strings.HasSuffix(summary, "..."))
}
