package openai

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const azureTokenRateLimitSSE = `data: {"error":{"code":"429","message":"Your requests to gpt-6-astra for gpt-6-astra in eastus2 have exceeded token rate limit."}}`

func newChatStreamTestContext(t *testing.T, body string) (*gin.Context, *httptest.ResponseRecorder, *http.Response, *relaycommon.RelayInfo) {
	t.Helper()
	c, rec, resp, info := newResponsesStreamTestContext(t, body)
	info.RelayFormat = types.RelayFormatOpenAI
	return c, rec, resp, info
}

func TestOaiStreamHandlerPreservesAuditBeforeError(t *testing.T) {
	enableResponsesStreamRetryForTest(t)
	body := strings.Join([]string{
		`data: {"choices":[],"created":0,"id":"","model":"","object":"","prompt_filter_results":[{"prompt_index":0}]}`,
		``,
		`data: {"id":"c1","object":"chat.completion.chunk","created":1,"model":"gpt-6-astra","choices":[{"index":0,"delta":{"role":"assistant","content":""},"finish_reason":null}]}`,
		``,
		azureTokenRateLimitSSE,
		``,
		azureTokenRateLimitSSE,
		``,
	}, "\n")
	c, recorder, resp, info := newChatStreamTestContext(t, body)

	usage, newAPIError := OaiStreamHandler(c, info, resp)

	require.NotNil(t, usage)
	require.Nil(t, newAPIError)
	assert.False(t, info.ResponsesStreamErrorBeforeCommit)
	assert.Equal(t, 1, strings.Count(recorder.Body.String(), `"prompt_filter_results":[{"prompt_index":0}]`))
}

func TestOaiStreamHandlerKeepsPartialStreamWhenErrorAfterOutput(t *testing.T) {
	enableResponsesStreamRetryForTest(t)
	body := strings.Join([]string{
		`data: {"id":"c1","object":"chat.completion.chunk","created":1,"model":"gpt-6-astra","choices":[{"index":0,"delta":{"content":"partial"},"finish_reason":null}]}`,
		``,
		azureTokenRateLimitSSE,
		``,
	}, "\n")
	c, recorder, resp, info := newChatStreamTestContext(t, body)

	usage, newAPIError := OaiStreamHandler(c, info, resp)

	require.Nil(t, newAPIError)
	require.NotNil(t, usage)
	assert.False(t, info.ResponsesStreamErrorBeforeCommit)
	assert.Contains(t, recorder.Body.String(), "partial")
	assert.Equal(t, 1, info.StreamStatus.ErrorCount)
}

func TestOaiStreamHandlerDisabledRuleKeepsOriginalPassThrough(t *testing.T) {
	enableResponsesStreamRetryForTest(t)
	disableResponsesStreamRetryForTest(t)
	body := strings.Join([]string{
		`data: {"id":"c1","object":"chat.completion.chunk","created":1,"model":"gpt-6-astra","choices":[{"index":0,"delta":{"role":"assistant","content":""},"finish_reason":null}]}`,
		``,
		azureTokenRateLimitSSE,
		``,
	}, "\n")
	c, recorder, resp, info := newChatStreamTestContext(t, body)

	_, newAPIError := OaiStreamHandler(c, info, resp)

	require.Nil(t, newAPIError)
	assert.False(t, info.ResponsesStreamErrorBeforeCommit)
	assert.Contains(t, recorder.Body.String(), "chat.completion.chunk")
}

func TestOaiResponsesStreamHandlerRetriesRateLimitAfterEmptyOutputContainers(t *testing.T) {
	enableResponsesStreamRetryForTest(t)
	body := strings.Join([]string{
		`data: {"type":"response.created","response":{"id":"resp_1","status":"in_progress"}}`,
		``,
		`data: {"type":"response.output_item.added","output_index":0,"item":{"type":"reasoning","id":"rs_1","summary":[]}}`,
		``,
		`data: {"type":"response.output_item.added","output_index":1,"item":{"type":"message","id":"msg_1","role":"assistant","content":[]}}`,
		``,
		`data: {"type":"response.content_part.added","item_id":"msg_1","output_index":1,"content_index":0,"part":{"type":"output_text","text":""}}`,
		``,
		`data: {"type":"error","error":{"code":"429","message":"Your requests to gpt-6-astra for gpt-6-astra in eastus2 have exceeded token rate limit."}}`,
		``,
	}, "\n")
	c, recorder, resp, info := newResponsesStreamTestContext(t, body)

	usage, newAPIError := OaiResponsesStreamHandler(c, info, resp)

	require.Nil(t, usage)
	require.NotNil(t, newAPIError)
	assert.Equal(t, http.StatusTooManyRequests, newAPIError.StatusCode)
	assert.True(t, info.ResponsesStreamErrorBeforeCommit)
	assert.Empty(t, recorder.Body.String())
}

func TestResponsesOutputItemWithToolArgumentsCommits(t *testing.T) {
	assert.False(t, chatStreamDataCommitsOutput(`{"choices":[{"index":0,"delta":{"role":"assistant"}}]}`))
	assert.True(t, chatStreamDataCommitsOutput(`{"choices":[{"index":0,"delta":{"tool_calls":[{"index":0}]}}]}`))
	assert.True(t, chatStreamDataCommitsOutput(`{"choices":[{"index":0,"delta":{"reasoning_content":"x"}}]}`))
	assert.True(t, chatStreamDataCommitsOutput(`{"choices":[],"usage":{"prompt_tokens":1}}`))
}

func disableResponsesStreamRetryForTest(t *testing.T) {
	t.Helper()
	operation_setting.GetQiqiSetting().ResponsesStreamErrorRetryEnabled = false
}

func TestChatAuditFrameIsFlushedUnchangedBeforeFailure(t *testing.T) {
	enableResponsesStreamRetryForTest(t)
	audit := `{"choices":[],"prompt_filter_results":[{"content_filter_results":{"hate":{"filtered":false}},"unknown":"keep me"}]}`
	for _, force := range []bool{false, true} {
		t.Run(fmt.Sprint(force), func(t *testing.T) {
			c, recorder, resp, info := newChatStreamTestContext(t, "data: "+audit+string([]byte{10, 10})+azureTokenRateLimitSSE+string([]byte{10, 10}))
			info.ChannelSetting.ForceFormat = force
			_, err := OaiStreamHandler(c, info, resp)
			require.Nil(t, err)
			require.Equal(t, 1, strings.Count(recorder.Body.String(), "data: "+audit+string([]byte{10, 10})))
			require.False(t, info.ResponsesStreamErrorBeforeCommit)
		})
	}
}

func TestStreamDataContainsModerationMetadata(t *testing.T) {
	for _, tc := range []struct {
		name string
		data string
		want bool
	}{
		{"top_level", `{"choices":[],"prompt_filter_results":[]}`, true},
		{"choice_nested", `{"choices":[{"content_filter_results":{"hate":{"filtered":false}}}]}`, true},
		{"offsets_nested", `{"choices":[{"content_filter_offsets":null}]}`, true},
		{"null_value_without_field", `{"choices":[{"content_filter_results":null}]}`, true},
		{"ordinary_empty", `{"choices":[],"metadata":{"value":null}}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, streamDataContainsModerationMetadata(tc.data))
		})
	}
}

func TestChatAuditPreservesPriorFrameOrder(t *testing.T) {
	enableResponsesStreamRetryForTest(t)
	preamble := `{"choices":[{"delta":{"role":"assistant"}}]}`
	audit := `{"choices":[],"prompt_filter_results":[{"id":9007199254740993}]}`
	c, recorder, resp, info := newChatStreamTestContext(t, "data: "+preamble+"\n\ndata: "+audit+"\n\n"+azureTokenRateLimitSSE+"\n\n")
	_, err := OaiStreamHandler(c, info, resp)
	require.Nil(t, err)
	body := recorder.Body.String()
	require.Contains(t, body, audit)
	require.Contains(t, body, preamble)
	require.Less(t, strings.Index(body, preamble), strings.Index(body, audit))
	require.Equal(t, 1, strings.Count(body, preamble))
}
