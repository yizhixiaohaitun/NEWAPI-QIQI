package relayconvert

import (
	"encoding/json"
	"strings"

	"github.com/QuantumNous/new-api/common"
)

// CanConvertChatToResponses conservatively rejects fields the converter cannot
// preserve. Call with the effective JSON before decoding into a lossy DTO.
func CanConvertChatToResponses(raw []byte) bool {
	var req map[string]json.RawMessage
	if common.Unmarshal(raw, &req) != nil || req == nil {
		return false
	}
	allowed := map[string]bool{"model": true, "messages": true, "stream": true, "stream_options": true, "max_tokens": true, "max_completion_tokens": true, "reasoning_effort": true, "temperature": true, "top_p": true, "n": true, "response_format": true, "parallel_tool_calls": true, "tools": true, "tool_choice": true, "user": true, "store": true, "metadata": true}
	for k := range req {
		if !allowed[k] {
			return false
		}
	}
	if v, ok := req["n"]; ok {
		var n int
		if common.Unmarshal(v, &n) != nil || n != 1 {
			return false
		}
	}
	if _, a := req["max_tokens"]; a {
		if _, b := req["max_completion_tokens"]; b {
			return false
		}
	}
	if v, ok := req["stream_options"]; ok {
		var o map[string]json.RawMessage
		if common.Unmarshal(v, &o) != nil || o == nil {
			return false
		}
		for k := range o {
			if k != "include_usage" {
				return false
			}
		}
	}
	var msgs []map[string]json.RawMessage
	if common.Unmarshal(req["messages"], &msgs) != nil || len(msgs) == 0 {
		return false
	}
	seenConversation := false
	seenSystem := false
	for _, m := range msgs {
		for k := range m {
			if k != "role" && k != "content" && k != "tool_calls" && k != "tool_call_id" {
				return false
			}
		}
		var role string
		if common.Unmarshal(m["role"], &role) != nil {
			return false
		}
		switch role {
		case "developer":
			return false
		case "system":
			if seenConversation || seenSystem {
				return false
			}
			seenSystem = true
		case "user", "assistant", "tool":
			seenConversation = true
		default:
			return false
		}
		if role == "tool" {
			var id string
			if common.Unmarshal(m["tool_call_id"], &id) != nil || strings.TrimSpace(id) == "" {
				return false
			}
		} else if _, ok := m["tool_call_id"]; ok {
			return false
		}
		if rawCalls, ok := m["tool_calls"]; ok {
			if role != "assistant" {
				return false
			}
			var calls []map[string]json.RawMessage
			if common.Unmarshal(rawCalls, &calls) != nil {
				return false
			}
			for _, c := range calls {
				var id, typ string
				var fn map[string]json.RawMessage
				if common.Unmarshal(c["id"], &id) != nil || strings.TrimSpace(id) == "" || common.Unmarshal(c["type"], &typ) != nil || typ != "function" || common.Unmarshal(c["function"], &fn) != nil {
					return false
				}
				for k := range c {
					if k != "id" && k != "type" && k != "function" {
						return false
					}
				}
				for k := range fn {
					if k != "name" && k != "arguments" {
						return false
					}
				}
				var name, args string
				if common.Unmarshal(fn["name"], &name) != nil || strings.TrimSpace(name) == "" || common.Unmarshal(fn["arguments"], &args) != nil {
					return false
				}
			}
		}
		content, ok := m["content"]
		if !ok || string(content) == "null" {
			if role != "assistant" {
				return false
			}
			continue
		}
		var text string
		if common.Unmarshal(content, &text) == nil {
			if role == "system" && (strings.TrimSpace(text) != text || text == "") {
				return false
			}
			continue
		}
		if role == "tool" || role == "system" || role == "developer" {
			return false
		}
		var parts []map[string]json.RawMessage
		if common.Unmarshal(content, &parts) != nil || parts == nil {
			return false
		}
		for _, p := range parts {
			var typ string
			if common.Unmarshal(p["type"], &typ) != nil {
				return false
			}
			switch typ {
			case "text":
				for k := range p {
					if k != "type" && k != "text" {
						return false
					}
				}
				if common.Unmarshal(p["text"], &text) != nil {
					return false
				}
			case "image_url":
				if role != "user" {
					return false
				}
				for k := range p {
					if k != "type" && k != "image_url" {
						return false
					}
				}
				var img map[string]json.RawMessage
				if common.Unmarshal(p["image_url"], &img) != nil || len(img) != 1 || common.Unmarshal(img["url"], &text) != nil || text == "" {
					return false
				}
			default:
				return false
			}
		}
	}
	if v, ok := req["response_format"]; ok {
		var f map[string]json.RawMessage
		if common.Unmarshal(v, &f) != nil || f == nil {
			return false
		}
		for k := range f {
			if k != "type" && k != "json_schema" {
				return false
			}
		}
		var typ string
		if common.Unmarshal(f["type"], &typ) != nil || (typ != "text" && typ != "json_object" && typ != "json_schema") {
			return false
		}
		if typ != "json_schema" {
			if _, exists := f["json_schema"]; exists {
				return false
			}
		}
	}
	if v, ok := req["tool_choice"]; ok {
		var choice string
		if common.Unmarshal(v, &choice) == nil {
			if choice != "none" && choice != "auto" && choice != "required" {
				return false
			}
		} else {
			var obj map[string]json.RawMessage
			var typ string
			var fn map[string]json.RawMessage
			var name string
			if common.Unmarshal(v, &obj) != nil || len(obj) != 2 || common.Unmarshal(obj["type"], &typ) != nil || typ != "function" || common.Unmarshal(obj["function"], &fn) != nil || len(fn) != 1 || common.Unmarshal(fn["name"], &name) != nil || name == "" {
				return false
			}
		}
	}
	if rawTools, ok := req["tools"]; ok {
		var tools []map[string]json.RawMessage
		if common.Unmarshal(rawTools, &tools) != nil {
			return false
		}
		for _, tool := range tools {
			var typ string
			var fn map[string]json.RawMessage
			if common.Unmarshal(tool["type"], &typ) != nil || typ != "function" || common.Unmarshal(tool["function"], &fn) != nil {
				return false
			}
			for k := range tool {
				if k != "type" && k != "function" {
					return false
				}
			}
			for k := range fn {
				if k != "name" && k != "description" && k != "parameters" {
					return false
				}
			}
			var name string
			if common.Unmarshal(fn["name"], &name) != nil || name == "" {
				return false
			}
		}
	}
	return true
}
