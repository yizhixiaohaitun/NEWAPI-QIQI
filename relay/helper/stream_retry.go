package helper

import (
	"bytes"
	"net/http"

	"github.com/gin-gonic/gin"
)

const bufferedStreamLimit = 4 << 20

// BufferedStreamWriter keeps one upstream attempt invisible until the handler
// has identified a safe client-visible output. This is intentionally per
// attempt: a retry can discard only bytes that have never reached the client.
type BufferedStreamWriter struct {
	gin.ResponseWriter
	header        http.Header
	buffer        bytes.Buffer
	pendingStatus int
	committed     bool
}

func NewBufferedStreamWriter(writer gin.ResponseWriter) *BufferedStreamWriter {
	return &BufferedStreamWriter{
		ResponseWriter: writer,
		header:         writer.Header().Clone(),
	}
}

func (w *BufferedStreamWriter) Header() http.Header {
	return w.header
}

func (w *BufferedStreamWriter) WriteHeader(code int) {
	if w.committed {
		w.ResponseWriter.WriteHeader(code)
		return
	}
	if w.pendingStatus == 0 {
		w.pendingStatus = code
	}
}

func (w *BufferedStreamWriter) WriteHeaderNow() {
	if w.committed {
		w.ResponseWriter.WriteHeaderNow()
		return
	}
	if w.pendingStatus == 0 {
		w.pendingStatus = http.StatusOK
	}
}

func (w *BufferedStreamWriter) Write(data []byte) (int, error) {
	if w.committed {
		return w.ResponseWriter.Write(data)
	}
	if w.buffer.Len()+len(data) > bufferedStreamLimit {
		if err := w.Commit(); err != nil {
			return 0, err
		}
		return w.ResponseWriter.Write(data)
	}
	return w.buffer.Write(data)
}

func (w *BufferedStreamWriter) WriteString(data string) (int, error) {
	if w.committed {
		return w.ResponseWriter.WriteString(data)
	}
	if w.buffer.Len()+len(data) > bufferedStreamLimit {
		if err := w.Commit(); err != nil {
			return 0, err
		}
		return w.ResponseWriter.WriteString(data)
	}
	return w.buffer.WriteString(data)
}

func (w *BufferedStreamWriter) Flush() {
	if w.committed {
		w.ResponseWriter.Flush()
	}
}

func (w *BufferedStreamWriter) Written() bool {
	return w.committed && w.ResponseWriter.Written()
}

func (w *BufferedStreamWriter) Status() int {
	if w.pendingStatus != 0 {
		return w.pendingStatus
	}
	return w.ResponseWriter.Status()
}

func (w *BufferedStreamWriter) Size() int {
	if w.committed {
		return w.ResponseWriter.Size()
	}
	return w.buffer.Len()
}

func (w *BufferedStreamWriter) Committed() bool {
	return w.committed
}

func (w *BufferedStreamWriter) Commit() error {
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

func (w *BufferedStreamWriter) Discard() {
	if w.committed {
		return
	}
	w.buffer.Reset()
}

func GetBufferedStreamWriter(c *gin.Context) *BufferedStreamWriter {
	if c == nil || c.Writer == nil {
		return nil
	}
	writer, _ := c.Writer.(*BufferedStreamWriter)
	return writer
}

func CommitBufferedStream(c *gin.Context) error {
	writer := GetBufferedStreamWriter(c)
	if writer == nil {
		return nil
	}
	return writer.Commit()
}

func DiscardBufferedStream(c *gin.Context) {
	if writer := GetBufferedStreamWriter(c); writer != nil {
		writer.Discard()
	}
}
