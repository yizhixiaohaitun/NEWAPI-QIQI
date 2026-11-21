package openai

import (
	"encoding/json"
	"strings"

	"github.com/QuantumNous/new-api/dto"
)

// Unlike the legacy retry guard, the race must not select usage, role,
// finish markers, malformed JSON, or unknown extensions as first output.
func chatStreamRaceHasOutput(data string) bool {
	var frame struct {
		Choices []struct {
			Delta struct {
				Content          string                                   `json:"content"`
				Reasoning        string                                   `json:"reasoning"`
				ReasoningContent string                                   `json:"reasoning_content"`
				Refusal          string                                   `json:"refusal"`
				Audio            struct{ Data, Transcript string }        `json:"audio"`
				FunctionCall     streamHedgeFunction                      `json:"function_call"`
				ToolCalls        []struct{ Function streamHedgeFunction } `json:"tool_calls"`
			} `json:"delta"`
		} `json:"choices"`
	}
	if json.Unmarshal([]byte(data), &frame) != nil {
		return false
	}
	for _, choice := range frame.Choices {
		d := choice.Delta
		if d.Content != "" || d.Reasoning != "" || d.ReasoningContent != "" || d.Refusal != "" || d.Audio.Data != "" || d.Audio.Transcript != "" || d.FunctionCall.hasOutput() {
			return true
		}
		for _, call := range d.ToolCalls {
			if call.Function.hasOutput() {
				return true
			}
		}
	}
	return false
}

type streamHedgeFunction struct{ Name, Arguments string }

func (f streamHedgeFunction) hasOutput() bool { return f.Name != "" || f.Arguments != "" }

func responsesStreamRaceHasOutput(event *dto.ResponsesStreamResponse, data string) bool {
	var raw map[string]json.RawMessage
	if json.Unmarshal([]byte(data), &raw) != nil {
		return false
	}
	switch strings.ToLower(event.Type) {
	case "response.output_text.delta", "response.reasoning_text.delta", "response.reasoning_summary_text.delta", "response.refusal.delta", "response.function_call_arguments.delta", "response.custom_tool_call_input.delta", "response.output_audio.delta", "response.output_audio_transcript.delta":
		return hedgeNonemptyString(raw["delta"])
	case "response.output_text.done", "response.reasoning_text.done", "response.reasoning_summary_text.done":
		return hedgeNonemptyString(raw["text"])
	case "response.refusal.done":
		return hedgeNonemptyString(raw["refusal"])
	case "response.output_item.added", "response.output_item.done":
		return hedgeResponsesItem(raw["item"], event.Type == "response.output_item.done")
	case "response.content_part.added", "response.content_part.done", "response.reasoning_summary_part.added", "response.reasoning_summary_part.done":
		return hedgeResponsesPart(raw["part"])
	case "response.completed", "response.incomplete":
		var response struct{ Output []json.RawMessage }
		if json.Unmarshal(raw["response"], &response) != nil {
			return false
		}
		for _, item := range response.Output {
			if hedgeResponsesItem(item, true) {
				return true
			}
		}
	}
	return false
}

func hedgeNonemptyString(raw json.RawMessage) bool {
	var s string
	return json.Unmarshal(raw, &s) == nil && s != ""
}

func hedgeResponsesPart(raw json.RawMessage) bool {
	var p struct{ Text, Refusal string }
	return json.Unmarshal(raw, &p) == nil && (p.Text != "" || p.Refusal != "")
}

func hedgeResponsesItem(raw json.RawMessage, done bool) bool {
	var i struct {
		Type, Name, Arguments, Input string
		Content, Summary             []json.RawMessage
	}
	if json.Unmarshal(raw, &i) != nil {
		return false
	}
	switch i.Type {
	case "message", "reasoning":
		for _, parts := range [][]json.RawMessage{i.Content, i.Summary} {
			for _, p := range parts {
				if hedgeResponsesPart(p) {
					return true
				}
			}
		}
	case "function_call", "custom_tool_call":
		return i.Arguments != "" || i.Input != "" || (done && i.Name != "")
	}
	return false
}
