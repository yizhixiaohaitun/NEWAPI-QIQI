package openai

import (
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/service"
)

// Responses repeats the same content at several levels: delta, part.done,
// item.done and the terminal response. A snapshot replaces its part, not its
// siblings. Keep only billable strings, never wire JSON or binary payloads.
type responsesTextPartKey struct {
	id         string
	slot, part int
	kind       string
}

type responsesTextSpan struct {
	key  responsesTextPartKey
	full bool
	text strings.Builder
}

type responsesObservedText struct {
	ids   map[int]string
	spans []*responsesTextSpan
}

func (o *responsesObservedText) key(id string, index *int) responsesTextPartKey {
	key := responsesTextPartKey{id: id}
	if index != nil {
		key.slot = *index
		if id != "" {
			if o.ids == nil {
				o.ids = make(map[int]string)
			}
			o.ids[*index] = id
		}
	}
	if id != "" {
		key.slot = 0
	}
	return key
}

func (o *responsesObservedText) put(k responsesTextPartKey, kind string, part *int, text string, full bool) {
	if text == "" {
		return
	}
	k.kind = kind
	if part != nil {
		k.part = *part
	}
	// Normal token-by-token streaming extends one builder. Keep the order of
	// alternating spans so a late item ID can also resolve index-only frames.
	if n := len(o.spans); n > 0 && o.spans[n-1].key == k {
		s := o.spans[n-1]
		if full {
			s.full = true
			s.text.Reset()
		}
		s.text.WriteString(text)
		return
	}
	s := &responsesTextSpan{key: k, full: full}
	s.text.WriteString(text)
	o.spans = append(o.spans, s)
}

type responsesBillablePart struct{ Type, Text, Refusal string }
type responsesBillableItem struct {
	Type, ID, Role, Arguments, Input string
	Content, Summary                 []responsesBillablePart
}

func (o *responsesObservedText) part(k responsesTextPartKey, pos *int, p responsesBillablePart) {
	switch p.Type {
	case "output_text", "reasoning_text", "summary_text":
		o.put(k, p.Type, pos, p.Text, true)
	case "refusal":
		o.put(k, p.Type, pos, p.Refusal, true)
	}
}

func (o *responsesObservedText) item(index *int, item responsesBillableItem) {
	k := o.key(item.ID, index)
	switch item.Type {
	case "function_call":
		o.put(k, "arguments", nil, item.Arguments, true)
	case "custom_tool_call":
		o.put(k, "tool_input", nil, item.Input, true)
	case "message", "reasoning":
		if item.Type == "message" && item.Role != "" && item.Role != "assistant" {
			return
		}
		for i, p := range item.Content {
			o.part(k, &i, p)
		}
		for i, p := range item.Summary {
			o.part(k, &i, p)
		}
	}
}

// Called only after a successful writer operation. Buffered candidates are
// discarded by the handler before settlement unless commit also succeeds.
func (o *responsesObservedText) observe(e *dto.ResponsesStreamResponse, raw string) {
	k := o.key(e.ItemID, e.OutputIndex)
	switch e.Type {
	case "response.output_text.delta":
		o.put(k, "output_text", e.ContentIndex, e.Delta, false)
	case "response.reasoning_text.delta":
		o.put(k, "reasoning_text", e.ContentIndex, e.Delta, false)
	case "response.reasoning_summary_text.delta":
		o.put(k, "summary_text", e.SummaryIndex, e.Delta, false)
	case "response.refusal.delta":
		o.put(k, "refusal", e.ContentIndex, e.Delta, false)
	case "response.function_call_arguments.delta":
		o.put(k, "arguments", nil, e.Delta, false)
	case "response.custom_tool_call_input.delta":
		o.put(k, "tool_input", nil, e.Delta, false)
	case "response.output_text.done", "response.reasoning_text.done", "response.reasoning_summary_text.done", "response.refusal.done", "response.function_call_arguments.done", "response.custom_tool_call_input.done":
		var v struct{ Text, Refusal, Arguments, Input string }
		if common.UnmarshalJsonStr(raw, &v) != nil {
			return
		}
		switch e.Type {
		case "response.output_text.done":
			o.put(k, "output_text", e.ContentIndex, v.Text, true)
		case "response.reasoning_text.done":
			o.put(k, "reasoning_text", e.ContentIndex, v.Text, true)
		case "response.reasoning_summary_text.done":
			o.put(k, "summary_text", e.SummaryIndex, v.Text, true)
		case "response.refusal.done":
			o.put(k, "refusal", e.ContentIndex, v.Refusal, true)
		case "response.function_call_arguments.done":
			o.put(k, "arguments", nil, v.Arguments, true)
		case "response.custom_tool_call_input.done":
			o.put(k, "tool_input", nil, v.Input, true)
		}
	case "response.content_part.added", "response.content_part.done", "response.reasoning_summary_part.added", "response.reasoning_summary_part.done":
		var v struct{ Part responsesBillablePart }
		if common.UnmarshalJsonStr(raw, &v) != nil {
			return
		}
		pos := e.ContentIndex
		if v.Part.Type == "summary_text" {
			pos = e.SummaryIndex
		}
		o.part(k, pos, v.Part)
	case dto.ResponsesOutputTypeItemAdded, dto.ResponsesOutputTypeItemDone:
		var v struct{ Item responsesBillableItem }
		if common.UnmarshalJsonStr(raw, &v) == nil {
			o.item(e.OutputIndex, v.Item)
		}
	case "response.completed", "response.incomplete":
		var v struct {
			Response struct{ Output []responsesBillableItem }
		}
		if common.UnmarshalJsonStr(raw, &v) == nil {
			for i, item := range v.Response.Output {
				o.item(&i, item)
			}
		}
	}
}

func (o *responsesObservedText) tokens(model string) int {
	if len(o.spans) == 1 {
		return service.CountTextToken(o.spans[0].text.String(), model)
	}
	parts := make(map[responsesTextPartKey]*strings.Builder)
	for _, s := range o.spans {
		k := s.key
		if id := o.ids[k.slot]; k.id == "" && id != "" {
			k.id, k.slot = id, 0
		}
		b := parts[k]
		if b == nil {
			b = new(strings.Builder)
			parts[k] = b
		}
		if s.full {
			b.Reset()
		}
		b.WriteString(s.text.String())
	}
	var n int
	for _, b := range parts {
		n += service.CountTextToken(b.String(), model)
	}
	return n
}

// Zero is a valid metered value. Distinguish it from a missing field, and
// preserve all cache and other provider details when only one side is missing.
type responsesReportedTokens struct {
	Total      *int `json:"total_tokens"`
	Input      *int `json:"input_tokens"`
	Prompt     *int `json:"prompt_tokens"`
	Output     *int `json:"output_tokens"`
	Completion *int `json:"completion_tokens"`
}

func (r responsesReportedTokens) hasOutput() bool { return r.Output != nil || r.Completion != nil }

func (o *responsesObservedText) settleUsage(u *dto.Usage, r responsesReportedTokens, model string, prompt int) {
	knownInput, knownOutput := r.Input != nil || r.Prompt != nil, r.hasOutput()
	if r.Input != nil {
		u.PromptTokens = max(0, *r.Input)
	} else if r.Prompt != nil {
		u.PromptTokens = max(0, *r.Prompt)
	}
	if r.Output != nil {
		u.CompletionTokens = max(0, *r.Output)
	} else if r.Completion != nil {
		u.CompletionTokens = max(0, *r.Completion)
	}
	// A reported total plus one component is also a metered breakdown.
	if r.Total != nil {
		if knownInput && !knownOutput && *r.Total >= u.PromptTokens {
			u.CompletionTokens = *r.Total - u.PromptTokens
			knownOutput = true
		}
		if knownOutput && !knownInput && *r.Total >= u.CompletionTokens {
			u.PromptTokens = *r.Total - u.CompletionTokens
			knownInput = true
		}
	}
	estimated := false
	if !knownOutput && u.CompletionTokens == 0 {
		if n := o.tokens(model); n > 0 {
			u.CompletionTokens, estimated = n, true
		}
	}
	if !knownInput && u.PromptTokens == 0 && u.CompletionTokens > 0 && prompt > 0 {
		u.PromptTokens, estimated = prompt, true
	}
	u.InputTokens, u.OutputTokens = u.PromptTokens, u.CompletionTokens
	if r.Total == nil {
		u.TotalTokens = u.PromptTokens + u.CompletionTokens
	}
	if estimated {
		u.BillingUsage = dto.NewOpenAIResponsesBillingUsage(u)
		u.BillingUsage.Estimated = true
	}
}
