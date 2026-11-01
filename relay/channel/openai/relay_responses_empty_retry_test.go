package openai

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestResponsesStreamEmptyEventCommitBoundary(t *testing.T) {
	for _, tc := range []struct {
		data    string
		commits bool
	}{
		{`{"type":"response.output_text.delta","delta":""}`, false},
		{`{"type":"response.output_text.done","text":""}`, false},
		{`{"type":"response.content_part.done","part":{"type":"output_text","text":"","annotations":[]}}`, false},
		{`{"type":"response.output_item.done","item":{"type":"message","content":[]}}`, false},
		{`{"type":"response.output_item.done","item":{"type":"reasoning","summary":[]}}`, false},
		{`{"type":"response.completed","response":{"output":[],"usage":{"input_tokens":0,"output_tokens":0}}}`, false},
		{`{"type":"response.incomplete","response":{"output":[],"incomplete_details":{"reason":"max_output_tokens"}}}`, false},
		{`{"type":"response.completed","response":{"output":[{"type":"message","content":[{"type":"output_text","text":"hello"}]}]}}`, true},
		{`{"type":"response.output_text.done","text":"hello"}`, true},
		{`{"type":"response.output_item.done","item":{"type":"reasoning","summary":[{"type":"summary_text","text":"thinking"}]}}`, true},
		{`{"type":"response.output_item.done","item":{"type":"reasoning","encrypted_content":"opaque"}}`, true},
		{`{"type":"response.output_item.done","item":{"type":"message","content":[{"type":"refusal","refusal":"declined"}]}}`, true},
		{`{"type":"response.content_part.added","part":{"type":"refusal","refusal":"declined"}}`, true},
		{`{"type":"response.refusal.delta","delta":"declined"}`, true},
		{`{"type":"response.function_call_arguments.delta","delta":"{}"}`, true},
		{`{"type":"response.function_call_arguments.done","arguments":""}`, true},
		{`{"type":"response.output_item.done","item":{"type":"function_call","call_id":"call_1","name":"lookup","arguments":""}}`, true},
		{`{"type":"response.output_item.added","item":{"type":"custom_tool_call","input":""}}`, false},
		{`{"type":"response.output_item.done","item":{"type":"custom_tool_call","input":"command"}}`, true},
		{`{"type":"response.output_audio.delta","delta":"base64"}`, true},
		{`{"type":"response.output_item.done","item":{"type":"image_generation_call","result":"base64"}}`, true},
		{`{"type":"response.future_tool_event","payload":"unknown"}`, true},
	} {
		t.Run(tc.data, func(t *testing.T) {
			var event dto.ResponsesStreamResponse
			require.NoError(t, common.UnmarshalJsonStr(tc.data, &event))
			require.Equal(t, tc.commits, responsesStreamDataCommitsOutput(&event, tc.data))
		})
	}
}

func TestResponsesStreamBufferLimitKeepsNoReplayBoundary(t *testing.T) {
	enableResponsesStreamRetryForTest(t)
	// Large prelude still obeys the existing memory limit: once it has been
	// sent, no later failure may cause the request to be replayed.
	body := "data: {\"type\":\"response.created\",\"padding\":\"" + strings.Repeat("x", upstreamStreamRetryBufferLimit) + "\"}\n\n" +
		"data: {\"type\":\"error\",\"message\":\"failure after buffer commit\"}\n\n"
	c, recorder, resp, info := newResponsesStreamTestContext(t, body)
	_, err := OaiResponsesStreamHandler(c, info, resp)
	require.Nil(t, err)
	require.False(t, info.ResponsesStreamErrorBeforeCommit)
	require.Contains(t, recorder.Body.String(), "failure after buffer commit")
}

func TestResponsesStreamEmptyFailureKeepsOriginalFlatError(t *testing.T) {
	enableResponsesStreamRetryForTest(t)
	c, recorder, resp, info := newResponsesStreamTestContext(t, "data: {\"type\":\"error\",\"code\":\"context_length_exceeded\",\"message\":\"Your input exceeds the context window of this model. Please adjust your input and try again.\"}\n\n")
	_, err := OaiResponsesStreamHandler(c, info, resp)
	require.NotNil(t, err)
	require.Equal(t, "Your input exceeds the context window of this model. Please adjust your input and try again.", err.ToOpenAIError().Message)
	require.Equal(t, "context_length_exceeded", err.ToOpenAIError().Code)
	require.Equal(t, 400, err.StatusCode)
	require.Empty(t, recorder.Body.String())
	require.True(t, c.GetBool("upstream_stream_error_before_commit"))
}

func TestStreamRetryWriterDiscardsUncommittedFrames(t *testing.T) {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	writer := newUpstreamStreamRetryWriter(c.Writer)
	_, err := writer.WriteString("prelude")
	require.NoError(t, err)
	writer.Flush()
	require.False(t, writer.Written())
	require.Empty(t, recorder.Body.String())
}
