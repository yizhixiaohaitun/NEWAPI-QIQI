package openai

import (
	"context"
	"io"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// responsesDeliveryWriter observes the actual downstream writes, including
// automatic prelude commits and writes inside the best-effort SSE renderer.
// The scanner serializes writes (including pings) and joins them before the
// handler reads err for the last time.
type responsesDeliveryWriter struct {
	gin.ResponseWriter
	err            error
	cancelUpstream context.CancelFunc
}

func (w *responsesDeliveryWriter) recordWrite(n, want int, err error) (int, error) {
	if err == nil && n != want {
		err = io.ErrShortWrite
	}
	w.fail(err)
	return n, err
}

func (w *responsesDeliveryWriter) fail(err error) {
	if err == nil || w.err != nil {
		return
	}
	w.err = err
	if w.cancelUpstream != nil {
		w.cancelUpstream()
	}
}

func (w *responsesDeliveryWriter) Write(p []byte) (int, error) {
	if w.err != nil {
		return 0, w.err
	}
	n, err := w.ResponseWriter.Write(p)
	return w.recordWrite(n, len(p), err)
}

func (w *responsesDeliveryWriter) WriteString(s string) (int, error) {
	if w.err != nil {
		return 0, w.err
	}
	n, err := w.ResponseWriter.WriteString(s)
	return w.recordWrite(n, len(s), err)
}

func (w *responsesDeliveryWriter) Flush() {
	if w.err == nil {
		w.ResponseWriter.Flush()
	}
}

func (w *responsesDeliveryWriter) SetWriteDeadline(deadline time.Time) error {
	return http.NewResponseController(w.ResponseWriter).SetWriteDeadline(deadline)
}
