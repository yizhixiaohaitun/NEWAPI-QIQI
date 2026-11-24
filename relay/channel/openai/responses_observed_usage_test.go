package openai

import (
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/service"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResponsesDeliveredTextUsage(t *testing.T) {
	service.InitTokenEncoders()
	const model = "gpt-4o"
	const args = `{"unit":"local fixture"}`
	const argsDelta = `{"type":"response.function_call_arguments.delta","item_id":"tool","output_index":0,"delta":"{\"unit\":"}`
	const argsDone = `{"type":"response.function_call_arguments.done","item_id":"tool","output_index":0,"arguments":"{\"unit\":\"local fixture\"}"}`
	const textDone = `{"type":"response.output_text.done","item_id":"msg","output_index":1,"content_index":0,"text":"first delivered text"}`
	const item = `{"type":"function_call","id":"tool","arguments":"{\"unit\":\"local fixture\"}"}`
	for _, tc := range []struct {
		name                          string
		frames, parts                 []string
		prompt, upstreamOutput, cache int
		estimated                     bool
	}{
		{name: "arguments_delta_only", frames: []string{argsDelta}, parts: []string{`{"unit":`}, prompt: 10, estimated: true},
		{name: "arguments_full_replaces_prefix_and_repeated_item", frames: []string{argsDelta, argsDone, `{"type":"response.output_item.done","output_index":0,"item":` + item + `}`, `{"type":"response.completed","response":{"output":[` + item + `]}}`}, parts: []string{args}, prompt: 10, estimated: true},
		{name: "complete_text_without_delta", frames: []string{textDone}, parts: []string{"first delivered text"}, prompt: 10, estimated: true},
		{name: "interleaved_slots_and_parts", frames: []string{`{"type":"response.output_text.delta","output_index":1,"content_index":0,"delta":"first"}`, argsDelta, `{"type":"response.output_text.delta","item_id":"msg","content_index":1,"delta":"second"}`, textDone, argsDone, `{"type":"response.content_part.done","output_index":1,"content_index":1,"part":{"type":"output_text","text":"second text"}}`}, parts: []string{args, "first delivered text", "second text"}, prompt: 10, estimated: true},
		{name: "refusal_reasoning_and_custom_input", frames: []string{`{"type":"response.refusal.delta","item_id":"m","delta":"refusal"}`, `{"type":"response.refusal.done","item_id":"m","refusal":"refusal"}`, `{"type":"response.reasoning_summary_part.done","item_id":"r","summary_index":0,"part":{"type":"summary_text","text":"short summary"}}`, `{"type":"response.custom_tool_call_input.done","item_id":"tool","input":"plain input"}`}, parts: []string{"refusal", "short summary", "plain input"}, prompt: 10, estimated: true},
		{name: "no_text_from_media_or_metadata", frames: []string{`{"type":"response.audio.delta","delta":"bWVkaWE="}`, `{"type":"response.output_item.done","item":{"type":"image_generation_call","result":"bWVkaWE="}}`}},
		{name: "metered_report_wins", frames: []string{argsDone, `{"type":"response.completed","response":{"usage":{"input_tokens":12,"output_tokens":7,"total_tokens":19,"input_tokens_details":{"cached_tokens":4}}}}`}, prompt: 12, upstreamOutput: 7, cache: 4},
		{name: "explicit_zero_output_is_not_missing", frames: []string{textDone, `{"type":"response.completed","response":{"usage":{"input_tokens":12,"output_tokens":0,"total_tokens":12}}}`}, prompt: 12},
		{name: "explicit_zero_input_is_not_missing", frames: []string{textDone, `{"type":"response.completed","response":{"usage":{"input_tokens":0,"output_tokens":3,"total_tokens":3}}}`}, upstreamOutput: 3},
		{name: "partial_report_keeps_input_and_cache", frames: []string{textDone, `{"type":"response.incomplete","response":{"usage":{"input_tokens":12,"input_tokens_details":{"cached_tokens":4}}}}`}, prompt: 12, parts: []string{"first delivered text"}, cache: 4, estimated: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			enableResponsesStreamRetryForTest(t)
			var stream strings.Builder
			for _, f := range tc.frames {
				stream.WriteString("data: " + f + "\n\n")
			}
			c, writer, response, info := newResponsesStreamTestContext(t, stream.String())
			info.UpstreamModelName = model
			usage, apiErr := OaiResponsesStreamHandler(c, info, response)
			require.Nil(t, apiErr)
			require.NotNil(t, usage)
			require.True(t, writer.Flushed)
			want := tc.upstreamOutput
			for _, s := range tc.parts {
				n := service.CountTextToken(s, model)
				require.Positive(t, n)
				want += n
			}
			assert.Equal(t, tc.prompt, usage.PromptTokens)
			assert.Equal(t, want, usage.CompletionTokens)
			assert.Equal(t, tc.prompt+want, usage.TotalTokens)
			assert.Equal(t, tc.cache, usage.PromptTokensDetails.CachedTokens)
			if tc.estimated {
				require.NotNil(t, usage.BillingUsage)
				require.True(t, usage.BillingUsage.Estimated)
				require.NotNil(t, usage.BillingUsage.OpenAIUsage)
				assert.Equal(t, want, usage.BillingUsage.OpenAIUsage.CompletionTokens)
			} else if usage.BillingUsage != nil {
				assert.False(t, usage.BillingUsage.Estimated)
			}
			assert.NotContains(t, writer.Body.String(), `"billing_usage"`)
		})
	}
}
