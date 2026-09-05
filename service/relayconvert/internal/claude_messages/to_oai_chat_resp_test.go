package claudemessages

import (
	"bufio"
	"encoding/json"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
)

// The upstream sends no content blocks: the only explanation is in stop_details.
const refusalSSE = `event: message_start
data: {"type":"message_start","message":{"id":"msg_refusal","type":"message","role":"assistant","model":"claude-test","content":[],"usage":{"input_tokens":0,"output_tokens":0,"cache_creation_input_tokens":64285,"cache_creation":{"ephemeral_1h_input_tokens":64285}}}}

event: message_delta
data: {"type":"message_delta","delta":{"stop_reason":"refusal","stop_details":{"type":"refusal","category":"reasoning_extraction","explanation":"无法提供该请求的内容。"}},"usage":{"input_tokens":0,"output_tokens":0,"cache_creation_input_tokens":64285,"cache_creation":{"ephemeral_1h_input_tokens":64285}}}

event: message_stop
data: {"type":"message_stop"}

`

func wireChoice(t *testing.T, response *dto.ChatCompletionsStreamResponse) map[string]any {
	t.Helper()
	if response == nil {
		t.Fatal("expected converted chunk")
	}
	data, err := common.Marshal(response)
	if err != nil {
		t.Fatal(err)
	}
	var wire struct {
		Choices []map[string]any `json:"choices"`
	}
	if err := json.Unmarshal(data, &wire); err != nil {
		t.Fatal(err)
	}
	if len(wire.Choices) != 1 {
		t.Fatalf("choices = %s", data)
	}
	return wire.Choices[0]
}

func TestStreamResponseClaude2OpenAIRefusalSSE(t *testing.T) {
	info := &ClaudeResponseInfo{Created: 123}
	scanner := bufio.NewScanner(strings.NewReader(refusalSSE))
	events, chunks := 0, 0
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		var event dto.ClaudeResponse
		if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &event); err != nil {
			t.Fatal(err)
		}
		events++
		chunk := StreamResponseClaude2OpenAI(&event)
		FormatClaudeResponseInfo(&event, chunk, info)
		if event.Type == "message_stop" {
			if chunk != nil || !info.Done {
				t.Fatal("normal message_stop must not generate extra text or clear completion")
			}
			continue
		}
		chunks++
		choice := wireChoice(t, chunk)
		delta := choice["delta"].(map[string]any)
		if chunk.Id != "msg_refusal" || chunk.Model != "claude-test" {
			t.Fatalf("lost stream identity: %+v", chunk)
		}
		if event.Type == "message_start" {
			if delta["role"] != "assistant" || delta["content"] != "" || delta["refusal"] != nil {
				t.Fatalf("unexpected start: %#v", delta)
			}
		} else {
			if delta["refusal"] != "无法提供该请求的内容。" || choice["finish_reason"] != "content_filter" {
				t.Errorf("lost refusal explanation or finish reason: %#v", choice)
			}
			if delta["content"] != "[请求被拒绝]（网关提示）\n上游拒绝原因：无法提供该请求的内容。" {
				t.Errorf("missing labelled gateway notice: %#v", delta)
			}
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	if events != 3 || chunks != 2 {
		t.Fatalf("events=%d chunks=%d", events, chunks)
	}
	// Both events report the same cumulative cache usage, not two batches.
	usage := info.Usage
	if usage.PromptTokens != 0 || usage.CompletionTokens != 0 || usage.PromptTokensDetails.CachedCreationTokens != 64285 || usage.ClaudeCacheCreation1hTokens != 64285 {
		t.Fatalf("cache-only upstream usage changed or doubled: %+v", usage)
	}
	mapped := UsageFromClaudeUsage(usage)
	if mapped.PromptTokens != 64285 || mapped.TotalTokens != 64285 || mapped.PromptTokensDetails.CacheWriteTokens != 64285 {
		t.Fatalf("incorrect OpenAI usage mapping: %+v", mapped)
	}
}

func TestStreamResponseClaude2OpenAINoInventedRefusal(t *testing.T) {
	for _, tc := range []struct {
		name, data, content, finish string
	}{
		{"missing details", `{"type":"message_delta","delta":{"stop_reason":"refusal"}}`, "", "content_filter"},
		{"empty explanation", `{"type":"message_delta","delta":{"stop_reason":"refusal","stop_details":{"type":"refusal","explanation":""}}}`, "", "content_filter"},
		{"null details", `{"type":"message_delta","delta":{"stop_reason":"refusal","stop_details":null}}`, "", "content_filter"},
		{"normal text", `{"type":"content_block_delta","delta":{"type":"text_delta","text":"正常文本"}}`, "正常文本", ""},
		{"text block start", `{"type":"content_block_start","content_block":{"type":"text","text":"Hello"}}`, "Hello", ""},
		{"normal end", `{"type":"message_delta","delta":{"stop_reason":"end_turn"}}`, "", "stop"},
		{"non refusal details", `{"type":"message_delta","delta":{"stop_reason":"end_turn","stop_details":{"explanation":"not a refusal"}}}`, "", "stop"},
		{"missing stop reason", `{"type":"message_delta","delta":{"stop_details":{"type":"refusal","explanation":"unconfirmed"}}}`, "", ""},
		{"nil delta", `{"type":"message_delta"}`, "", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var event dto.ClaudeResponse
			if err := json.Unmarshal([]byte(tc.data), &event); err != nil {
				t.Fatal(err)
			}
			choice := wireChoice(t, StreamResponseClaude2OpenAI(&event))
			delta := choice["delta"].(map[string]any)
			if _, exists := delta["refusal"]; exists {
				t.Fatalf("invented refusal: %#v", delta)
			}
			if tc.content != "" {
				if delta["content"] != tc.content {
					t.Fatalf("lost normal text: %#v", delta)
				}
			} else if _, exists := delta["content"]; exists {
				t.Fatalf("invented content: %#v", delta)
			}
			if tc.finish == "" {
				if choice["finish_reason"] != nil {
					t.Fatalf("unexpected finish: %#v", choice)
				}
			} else if choice["finish_reason"] != tc.finish {
				t.Fatalf("incorrect finish: %#v", choice)
			}
		})
	}
}

func TestRefusalNoticeState(t *testing.T) {
	for _, prior := range []string{
		`{"type":"content_block_delta","delta":{"type":"text_delta","text":"already visible"}}`,
		`{"type":"content_block_start","content_block":{"type":"text","text":"already visible"}}`,
		`{"type":"message_start","message":{"content":[{"type":"text","text":"already visible"}]}}`,
	} {
		info := &ClaudeResponseInfo{}
		var event dto.ClaudeResponse
		if err := json.Unmarshal([]byte(prior), &event); err != nil {
			t.Fatal(err)
		}
		FormatClaudeResponseInfo(&event, nil, info)
		var refusal dto.ClaudeResponse
		if err := json.Unmarshal([]byte(`{"type":"message_delta","delta":{"stop_reason":"refusal","stop_details":{"explanation":"reason"}}}`), &refusal); err != nil {
			t.Fatal(err)
		}
		chunk := StreamResponseClaude2OpenAI(&refusal)
		FormatClaudeResponseInfo(&refusal, chunk, info)
		if chunk.Choices[0].Delta.Content != nil {
			t.Fatal("duplicate notice after visible text")
		}
		if chunk.Choices[0].Delta.Refusal == nil {
			t.Fatal("lost semantic refusal")
		}
	}
	for _, data := range []string{
		`{"type":"message_delta","delta":{"stop_reason":"refusal"}}`,
		`{"type":"message_delta","delta":{"stop_reason":"refusal","stop_details":{"explanation":"   "}}}`,
	} {
		info := &ClaudeResponseInfo{}
		var event dto.ClaudeResponse
		if err := json.Unmarshal([]byte(data), &event); err != nil {
			t.Fatal(err)
		}
		chunk := StreamResponseClaude2OpenAI(&event)
		FormatClaudeResponseInfo(&event, chunk, info)
		if chunk.Choices[0].Delta.GetContentString() != "[请求被拒绝]（网关提示）\n上游未提供拒绝原因。" || chunk.Choices[0].Delta.Refusal != nil {
			t.Fatal("invented reason or missing status")
		}
		repeated := StreamResponseClaude2OpenAI(&event)
		FormatClaudeResponseInfo(&event, repeated, info)
		if repeated.Choices[0].Delta.Content != nil {
			t.Fatal("repeated compatibility notice")
		}
		if info.ResponseText.Len() != 0 {
			t.Fatal("notice leaked into token estimation")
		}
	}
}
