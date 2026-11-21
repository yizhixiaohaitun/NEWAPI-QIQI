package controller

import (
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/types"
)

type hedgeHTTPProtocol struct {
	path, payload string
	channelType   int
	format        types.RelayFormat
	frames        func(label string, preamble bool) []string
}

func hedgeHTTPProtocolFor(modelName, name string) hedgeHTTPProtocol {
	switch name {
	case "json_parallel_all_chat_alias":
		return hedgeHTTPProtocol{"/v1/chat/completions", `{"model":"` + modelName + `","stream":true,"messages":[{"role":"user","content":"replayed body"}]}`, constant.ChannelTypeOpenAI, types.RelayFormatOpenAI, func(label string, preamble bool) []string {
			base := `{"id":"` + label + `-preamble","object":"chat.completion.chunk","model":"` + modelName + `",`
			if preamble {
				return []string{base + `"choices":[{"index":0,"delta":{"role":"assistant","content":""}}]}`, base + `"choices":[],"usage":{"prompt_tokens":5,"completion_tokens":0,"total_tokens":5}}`}
			}
			return []string{base + `"choices":[{"index":0,"delta":{"content":"` + label + `-visible"}}]}`, base + `"choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":5,"completion_tokens":1,"total_tokens":6}}`, "[DONE]"}
		}}
	case "json_parallel_all_responses_alias":
		return hedgeHTTPProtocol{"/v1/responses", `{"model":"` + modelName + `","stream":true,"store":false,"input":"replayed body"}`, constant.ChannelTypeOpenAI, types.RelayFormatOpenAIResponses, func(label string, preamble bool) []string {
			if preamble {
				return []string{`{"type":"response.created","response":{"id":"` + label + `-preamble","model":"` + modelName + `","status":"in_progress","output":[]}}`, `{"type":"response.output_text.delta","delta":""}`}
			}
			return []string{`{"type":"response.output_text.delta","delta":"` + label + `-visible"}`, `{"type":"response.completed","response":{"id":"` + label + `-preamble","model":"` + modelName + `","status":"completed","output":[{"type":"message","content":[{"type":"output_text","text":"` + label + `-visible"}]}],"usage":{"input_tokens":5,"output_tokens":1,"total_tokens":6}}}`}
		}}
	case "json_parallel_all_gemini_alias":
		return hedgeHTTPProtocol{"/v1beta/models/" + modelName + ":streamGenerateContent", `{"contents":[{"role":"user","parts":[{"text":"replayed body"}]}]}`, constant.ChannelTypeGemini, types.RelayFormatGemini, func(label string, preamble bool) []string {
			if preamble {
				return []string{`{"responseId":"` + label + `-preamble","candidates":[{"index":0,"content":{"role":"model","parts":[{"text":""}]}}],"usageMetadata":{"promptTokenCount":5}}`}
			}
			return []string{`{"responseId":"` + label + `-preamble","candidates":[{"index":0,"content":{"role":"model","parts":[{"text":"` + label + `-visible"}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":5,"candidatesTokenCount":1,"totalTokenCount":6}}`}
		}}
	default:
		return hedgeHTTPProtocol{"/v1/messages", `{"model":"` + modelName + `","stream":true,"max_tokens":64,"messages":[{"role":"user","content":"replayed body"}]}`, constant.ChannelTypeAnthropic, types.RelayFormatClaude, nil}
	}
}
