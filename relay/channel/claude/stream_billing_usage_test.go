package claude

import (
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestClaudeStreamFinalBillingUsage(t *testing.T) {
	const visible = "The upstream delivered a visible answer to this request."
	for _, tc := range []struct {
		name       string
		reported   *dto.ClaudeUsage
		text       string
		done       bool
		wantInput  int
		wantOutput int // -1 means the missing output should be estimated.
		estimated  bool
	}{
		{"complete provider usage stays authoritative", &dto.ClaudeUsage{InputTokens: 17, OutputTokens: 1, CacheReadInputTokens: 23}, visible, true, 17, 1, false},
		{"larger partial provider count is not replaced", &dto.ClaudeUsage{InputTokens: 17, OutputTokens: 512}, visible, false, 17, 512, false},
		{"missing usage gets a complete billing snapshot", nil, visible, true, 31, -1, true},
		{"cache-only input stays cache-only", &dto.ClaudeUsage{CacheReadInputTokens: 23}, visible, false, 0, -1, true},
		{"no reply leaves output at zero", &dto.ClaudeUsage{InputTokens: 17}, "", true, 17, 0, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			info := &relaycommon.RelayInfo{
				RelayFormat: types.RelayFormatClaude,
				ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "claude-3-5-sonnet"},
			}
			info.SetEstimatePromptTokens(31)
			stream := &ClaudeResponseInfo{Usage: &dto.Usage{}, Done: tc.done}
			stream.ResponseText.WriteString(tc.text)
			wantCacheRead := 0
			if tc.reported != nil {
				wantCacheRead = tc.reported.CacheReadInputTokens
				stream.Usage.PromptTokens = tc.reported.InputTokens
				stream.Usage.CompletionTokens = tc.reported.OutputTokens
				stream.Usage.PromptTokensDetails.CachedTokens = wantCacheRead
				stream.Usage.BillingUsage = dto.NewClaudeMessagesBillingUsage(tc.reported)
			}

			HandleStreamFinalResponse(c, info, stream)

			billing := stream.Usage.BillingUsage
			require.NotNil(t, billing)
			require.Equal(t, dto.BillingUsageSemanticAnthropic, billing.Semantic)
			require.Equal(t, tc.estimated, billing.Estimated)
			require.Equal(t, tc.wantInput, stream.Usage.PromptTokens)
			require.Equal(t, tc.wantInput, billing.ClaudeUsage.InputTokens)
			require.Equal(t, wantCacheRead, billing.ClaudeUsage.CacheReadInputTokens)
			require.Equal(t, stream.Usage.CompletionTokens, billing.ClaudeUsage.OutputTokens)
			if tc.wantOutput < 0 {
				require.Positive(t, billing.ClaudeUsage.OutputTokens)
			} else {
				require.Equal(t, tc.wantOutput, billing.ClaudeUsage.OutputTokens)
			}
		})
	}
}
