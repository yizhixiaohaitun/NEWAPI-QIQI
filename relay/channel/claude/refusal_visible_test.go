package claude

import (
	"bufio"
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

const visibleRefusalReason = "无法提供该请求的内容。"
const visibleRefusalStart = `{"type":"message_start","message":{"id":"msg_refusal","type":"message","role":"assistant","model":"claude-3-5-sonnet","content":[],"usage":{"input_tokens":0,"output_tokens":0,"cache_creation_input_tokens":64285,"cache_creation":{"ephemeral_1h_input_tokens":64285}}}}`
const visibleRefusalDelta = `{"type":"message_delta","delta":{"stop_reason":"refusal","stop_details":{"type":"refusal","category":"reasoning_extraction","explanation":"无法提供该请求的内容。"}},"usage":{"input_tokens":0,"output_tokens":0,"cache_creation_input_tokens":64285,"cache_creation":{"ephemeral_1h_input_tokens":64285}}}`

func visibleSSEData(s string) []string {
	var data []string
	scanner := bufio.NewScanner(strings.NewReader(s))
	for scanner.Scan() {
		if line := scanner.Text(); strings.HasPrefix(line, "data: ") {
			data = append(data, strings.TrimPrefix(line, "data: "))
		}
	}
	return data
}

// Exercises the real adapter and optionally exports its bytes for downstream replay.
// REFUSAL_REPLAY_DIR must point outside the repository; no upstream requests are made.
func TestClaudeRefusalVisibleReplay(t *testing.T) {
	cases := []struct {
		name, delta, want string
		blocks            []string
	}{
		{name: "reason", delta: visibleRefusalDelta, want: "[请求被拒绝]（网关提示）\n上游拒绝原因：" + visibleRefusalReason},
		{name: "missing", delta: strings.Replace(visibleRefusalDelta, visibleRefusalReason, "", 1), want: "[请求被拒绝]（网关提示）\n上游未提供拒绝原因。"},
		{name: "ordinary", delta: `{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"input_tokens":0,"output_tokens":4,"cache_creation_input_tokens":64285,"cache_creation":{"ephemeral_1h_input_tokens":64285}}}`, want: "正常文本", blocks: []string{
			`{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`,
			`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"正常文本"}}`,
			`{"type":"content_block_stop","index":0}`}},
		{name: "existing", delta: visibleRefusalDelta, want: "已有拒绝正文", blocks: []string{
			`{"type":"content_block_start","index":0,"content_block":{"type":"text","text":"已有拒绝正文"}}`,
			`{"type":"content_block_stop","index":0}`}},
		{name: "thinking", delta: visibleRefusalDelta, want: "[请求被拒绝]（网关提示）\n上游拒绝原因：" + visibleRefusalReason, blocks: []string{
			`{"type":"content_block_start","index":2,"content_block":{"type":"thinking","thinking":""}}`,
			`{"type":"content_block_delta","index":2,"delta":{"type":"thinking_delta","thinking":"thinking"}}`,
			`{"type":"content_block_stop","index":2}`}},
	}
	for _, tc := range cases {
		for _, native := range []bool{false, true} {
			name := tc.name + "-openai"
			if native {
				name = tc.name + "-claude"
			}
			t.Run(name, func(t *testing.T) {
				_, info := newClaudeRefusalUsageTestContext(t)
				recorder := httptest.NewRecorder()
				ctx, _ := gin.CreateTestContext(recorder)
				if !native {
					info.RelayFormat = types.RelayFormatOpenAI
				}
				info.ShouldIncludeUsage = true
				state := &ClaudeResponseInfo{Usage: &dto.Usage{}}
				events := append([]string{visibleRefusalStart}, tc.blocks...)
				events = append(events, tc.delta, `{"type":"message_stop"}`)
				var upstream strings.Builder
				for _, data := range events {
					var event dto.ClaudeResponse
					require.NoError(t, json.Unmarshal([]byte(data), &event))
					upstream.WriteString("event: " + event.Type + "\ndata: " + data + "\n\n")
				}
				for _, data := range visibleSSEData(upstream.String()) {
					require.Nil(t, HandleStreamResponseData(ctx, info, state, data))
				}
				require.Zero(t, state.Usage.PromptTokens)
				require.Equal(t, 64285, state.Usage.PromptTokensDetails.CachedCreationTokens)
				require.Equal(t, 64285, state.Usage.ClaudeCacheCreation1hTokens)
				if tc.name != "ordinary" {
					require.Zero(t, state.Usage.CompletionTokens)
				}
				sourceText := state.ResponseText.String()
				require.NotContains(t, sourceText, "[请求被拒绝]")
				HandleStreamFinalResponse(ctx, info, state)
				if tc.name == "reason" {
					expected := service.ResponseText2Usage(ctx, visibleRefusalReason, info.UpstreamModelName, info.GetEstimatePromptTokens())
					require.Equal(t, expected.CompletionTokens, state.Usage.CompletionTokens)
					require.Positive(t, state.Usage.CompletionTokens)
					require.True(t, state.Usage.BillingUsage.Estimated)
					require.True(t, common.GetContextKeyBool(ctx, constant.ContextKeyLocalCountTokens))
					require.Zero(t, state.Usage.PromptTokens)
				}
				if tc.name == "missing" {
					require.Zero(t, state.Usage.CompletionTokens)
				}
				var text strings.Builder
				var eventTypes []string
				foundFinish := false
				for _, data := range visibleSSEData(recorder.Body.String()) {
					if data == "[DONE]" {
						continue
					}
					if native {
						var event dto.ClaudeResponse
						require.NoError(t, json.Unmarshal([]byte(data), &event))
						eventTypes = append(eventTypes, event.Type)
						if event.Type == "content_block_start" && event.ContentBlock != nil && event.ContentBlock.Type == "text" {
							text.WriteString(event.ContentBlock.GetText())
						}
						if event.Type == "content_block_delta" && event.Delta != nil && event.Delta.Type == "text_delta" {
							text.WriteString(event.Delta.GetText())
							if tc.name == "thinking" {
								require.Equal(t, 3, event.GetIndex())
							}
						}
						if event.Type == "message_delta" {
							require.JSONEq(t, tc.delta, data) // includes original output=0, reason/category and cache-only input
							foundFinish = true
						}
					} else {
						var chunk dto.ChatCompletionsStreamResponse
						require.NoError(t, json.Unmarshal([]byte(data), &chunk))
						for _, choice := range chunk.Choices {
							text.WriteString(choice.Delta.GetContentString())
							if choice.FinishReason != nil {
								foundFinish = true
								expected := "content_filter"
								if tc.name == "ordinary" {
									expected = "stop"
								}
								require.Equal(t, expected, *choice.FinishReason)
								if tc.name == "reason" || tc.name == "existing" || tc.name == "thinking" {
									require.NotNil(t, choice.Delta.Refusal)
									require.Equal(t, visibleRefusalReason, *choice.Delta.Refusal)
								} else {
									require.Nil(t, choice.Delta.Refusal)
								}
							}
						}
						if chunk.Usage != nil {
							require.Equal(t, 64285, chunk.Usage.PromptTokens)
							require.Equal(t, 64285, chunk.Usage.PromptTokensDetails.CacheWriteTokens)
							require.Equal(t, state.Usage.CompletionTokens, chunk.Usage.CompletionTokens)
							if tc.name == "reason" {
								require.True(t, chunk.Usage.BillingUsage.Estimated)
							}
						}
					}
				}
				require.True(t, foundFinish)
				require.Equal(t, tc.want, text.String())
				if native && (tc.name == "reason" || tc.name == "missing") {
					require.Equal(t, []string{"message_start", "content_block_start", "content_block_delta", "content_block_stop", "message_delta", "message_stop"}, eventTypes)
				}
				if dir := os.Getenv("REFUSAL_REPLAY_DIR"); dir != "" {
					require.NoError(t, os.MkdirAll(dir, 0755))
					require.NoError(t, os.WriteFile(filepath.Join(dir, name+".sse"), recorder.Body.Bytes(), 0644))
					require.NoError(t, os.WriteFile(filepath.Join(dir, name+".upstream.sse"), []byte(upstream.String()), 0644))
				}
			})
		}
	}
}
