package dto

import (
	"bytes"
	"encoding/json"
	"strings"

	"github.com/QuantumNous/new-api/common"
)

// These are the fields understood by the native DTO, including SDK aliases.
// All other content/part fields are retained as raw JSON without numeric conversion.
var geminiPartFields = map[string]string{
	"text": "text", "thought": "thought", "inlineData": "inlineData", "inline_data": "inlineData",
	"functionCall": "functionCall", "function_call": "functionCall",
	"thoughtSignature": "thoughtSignature", "thought_signature": "thoughtSignature",
	"functionResponse": "functionResponse", "function_response": "functionResponse",
	"mediaResolution": "mediaResolution", "media_resolution": "mediaResolution",
	"videoMetadata": "videoMetadata", "video_metadata": "videoMetadata",
	"fileData": "fileData", "file_data": "fileData",
	"executableCode": "executableCode", "executable_code": "executableCode",
	"codeExecutionResult": "codeExecutionResult", "code_execution_result": "codeExecutionResult",
}

var geminiContentFields = map[string]string{"role": "role", "parts": "parts"}

// Keep unknown fields and recognized zero values omitted by the typed encoder.
// Current typed values always take precedence during marshal. The text marker
// also records an originally nonempty text so that deliberately clearing it
// emits an explicit empty string instead of resurrecting the old value.
func geminiNativeFields(data []byte, typed any, known map[string]string, preserveText bool) (map[string]json.RawMessage, error) {
	var original map[string]json.RawMessage
	if err := common.Unmarshal(data, &original); err != nil {
		return nil, err
	}
	encoded, err := common.Marshal(typed)
	if err != nil {
		return nil, err
	}
	var present map[string]json.RawMessage
	if err := common.Unmarshal(encoded, &present); err != nil {
		return nil, err
	}
	var retained map[string]json.RawMessage
	for name, raw := range original {
		canonical, ok := known[name]
		if !ok {
			// encoding/json also recognizes case-insensitive known field names.
			for alias, field := range known {
				if strings.EqualFold(name, alias) {
					canonical, ok = field, true
					break
				}
			}
		}
		if ok {
			if preserveText && canonical == "text" {
				if string(bytes.TrimSpace(raw)) != "null" {
					raw = json.RawMessage(`""`)
				}
			} else if _, exists := present[canonical]; exists {
				continue
			}
			name = canonical
		}
		if retained == nil {
			retained = make(map[string]json.RawMessage)
		}
		retained[name] = raw
	}
	return retained, nil
}

func marshalGeminiNative(typed any, retained map[string]json.RawMessage) ([]byte, error) {
	encoded, err := common.Marshal(typed)
	if err != nil || len(retained) == 0 {
		return encoded, err
	}
	var fields map[string]json.RawMessage
	if err = common.Unmarshal(encoded, &fields); err != nil {
		return nil, err
	}
	for name, raw := range retained {
		if _, exists := fields[name]; !exists {
			fields[name] = raw
		}
	}
	return common.Marshal(fields)
}

func (p GeminiPart) MarshalJSON() ([]byte, error) {
	type Alias GeminiPart
	return marshalGeminiNative(Alias(p), p.NativeFields)
}

func (c *GeminiChatContent) UnmarshalJSON(data []byte) error {
	type Alias GeminiChatContent
	var parsed Alias
	if err := common.Unmarshal(data, &parsed); err != nil {
		return err
	}
	*c = GeminiChatContent(parsed)
	var err error
	c.NativeFields, err = geminiNativeFields(data, parsed, geminiContentFields, false)
	return err
}

func (c GeminiChatContent) MarshalJSON() ([]byte, error) {
	type Alias GeminiChatContent
	return marshalGeminiNative(Alias(c), c.NativeFields)
}

// Decode tool-result numbers without converting them to float64. The ordinary
// map remains editable by existing callers.
func (r *GeminiFunctionResponse) UnmarshalJSON(data []byte) error {
	type Alias GeminiFunctionResponse
	var parsed struct {
		Alias
		Response json.RawMessage `json:"response"`
	}
	if err := common.Unmarshal(data, &parsed); err != nil {
		return err
	}
	*r = GeminiFunctionResponse(parsed.Alias)
	if len(parsed.Response) == 0 {
		return nil
	}
	return common.UnmarshalUseNumber(parsed.Response, &r.Response)
}
