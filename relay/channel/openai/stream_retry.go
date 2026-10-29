package openai

import (
	"bytes"
	"encoding/json"
	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
	"net/http"
)

const upstreamStreamRetryBufferLimit = 4 << 20

type upstreamStreamRetryWriter struct {
	gin.ResponseWriter
	header        http.Header
	buffer        bytes.Buffer
	pendingStatus int
	committed     bool
}

func newUpstreamStreamRetryWriter(writer gin.ResponseWriter) *upstreamStreamRetryWriter {
	return &upstreamStreamRetryWriter{
		ResponseWriter: writer,
		header:         writer.Header().Clone(),
	}
}

func (w *upstreamStreamRetryWriter) Header() http.Header {
	return w.header
}

func (w *upstreamStreamRetryWriter) WriteHeader(code int) {
	if w.committed {
		w.ResponseWriter.WriteHeader(code)
		return
	}
	if w.pendingStatus == 0 {
		w.pendingStatus = code
	}
}

func (w *upstreamStreamRetryWriter) WriteHeaderNow() {
	if w.committed {
		w.ResponseWriter.WriteHeaderNow()
		return
	}
	if w.pendingStatus == 0 {
		w.pendingStatus = http.StatusOK
	}
}

func (w *upstreamStreamRetryWriter) Write(data []byte) (int, error) {
	if w.committed {
		return w.ResponseWriter.Write(data)
	}
	if w.buffer.Len()+len(data) > upstreamStreamRetryBufferLimit {
		if err := w.Commit(); err != nil {
			return 0, err
		}
		return w.ResponseWriter.Write(data)
	}
	return w.buffer.Write(data)
}

func (w *upstreamStreamRetryWriter) WriteString(data string) (int, error) {
	if w.committed {
		return w.ResponseWriter.WriteString(data)
	}
	if w.buffer.Len()+len(data) > upstreamStreamRetryBufferLimit {
		if err := w.Commit(); err != nil {
			return 0, err
		}
		return w.ResponseWriter.WriteString(data)
	}
	return w.buffer.WriteString(data)
}

func (w *upstreamStreamRetryWriter) Flush() {
	if w.committed {
		w.ResponseWriter.Flush()
	}
}

func (w *upstreamStreamRetryWriter) Written() bool {
	return w.committed && w.ResponseWriter.Written()
}

func (w *upstreamStreamRetryWriter) Status() int {
	if w.pendingStatus != 0 {
		return w.pendingStatus
	}
	return w.ResponseWriter.Status()
}

func (w *upstreamStreamRetryWriter) Size() int {
	if w.committed {
		return w.ResponseWriter.Size()
	}
	return w.buffer.Len()
}

func (w *upstreamStreamRetryWriter) Commit() error {
	if w.committed {
		return nil
	}
	for key, values := range w.header {
		w.ResponseWriter.Header()[key] = append([]string(nil), values...)
	}
	if w.pendingStatus != 0 {
		w.ResponseWriter.WriteHeader(w.pendingStatus)
	}
	w.committed = true
	if w.buffer.Len() == 0 {
		return nil
	}
	_, err := w.ResponseWriter.Write(w.buffer.Bytes())
	return err
}

// chatStreamDataCommitsOutput only buffers Chat frames without visible output:
// role-only/empty-content deltas and choice-less metadata. Reasoning, tool calls,
// audio, finish and usage frames commit so they are never replayed elsewhere.
func chatStreamDataCommitsOutput(data string) bool {
	var envelope struct {
		Choices []struct {
			Delta        map[string]json.RawMessage `json:"delta"`
			Text         string                     `json:"text"`
			FinishReason *string                    `json:"finish_reason"`
		} `json:"choices"`
		Usage json.RawMessage `json:"usage"`
	}
	if common.UnmarshalJsonStr(data, &envelope) != nil {
		return true
	}
	if len(envelope.Usage) > 0 && string(envelope.Usage) != "null" {
		return true
	}
	if len(envelope.Choices) == 0 {
		// Azure prompt_filter_results and similar metadata frames have no
		// visible output; keep them buffered until real output arrives.
		return false
	}
	for _, choice := range envelope.Choices {
		if choice.Text != "" || choice.FinishReason != nil {
			return true
		}
		for key, raw := range choice.Delta {
			if key == "role" {
				continue
			}
			if key != "content" {
				return true
			}
			var content *string
			if common.Unmarshal(raw, &content) != nil || (content != nil && *content != "") {
				return true
			}
		}
	}
	return false
}
