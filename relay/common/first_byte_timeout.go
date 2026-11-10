package common

import (
	"context"
	"io"
	"sync"
	"time"
)

// FirstByteWatch bounds an HTTP attempt only until any upstream body byte is
// received. Even metadata or moderation bytes stop it: replaying after those
// bytes could silently discard safety information.
type FirstByteWatch struct {
	mu             sync.Mutex
	timer          *time.Timer
	finished       bool
	cancel         context.CancelFunc
	timedOut       bool
	received       bool
	firstByteTime  time.Time
	stopOnBodyByte bool
}

func NewFirstByteWatch(cancel context.CancelFunc, timeout time.Duration) *FirstByteWatch {
	return newFirstByteWatch(cancel, timeout, true)
}

// NewFirstOutputWatch bounds native message streams until their first meaningful
// output. Unlike the legacy HTTP-byte watch, protocol metadata and heartbeats do
// not stop the deadline; protocol handlers call MarkOutput once visible output
// is identified.
func NewFirstOutputWatch(cancel context.CancelFunc, timeout time.Duration) *FirstByteWatch {
	return newFirstByteWatch(cancel, timeout, false)
}

func newFirstByteWatch(cancel context.CancelFunc, timeout time.Duration, stopOnBodyByte bool) *FirstByteWatch {
	w := &FirstByteWatch{cancel: cancel, stopOnBodyByte: stopOnBodyByte}
	w.timer = time.AfterFunc(timeout, func() {
		w.mu.Lock()
		defer w.mu.Unlock()
		if !w.received && !w.finished {
			w.timedOut = true
			w.cancel()
		}
	})
	return w
}

func (w *FirstByteWatch) Wrap(body io.ReadCloser) io.ReadCloser {
	return &firstByteReader{ReadCloser: body, watch: w}
}

type firstByteReader struct {
	io.ReadCloser
	watch *FirstByteWatch
}

func (r *firstByteReader) Read(p []byte) (int, error) {
	n, err := r.ReadCloser.Read(p)
	if n > 0 && r.watch.stopOnBodyByte {
		r.watch.mu.Lock()
		if !r.watch.received {
			r.watch.firstByteTime = time.Now()
		}
		r.watch.received = true
		r.watch.timer.Stop()
		r.watch.mu.Unlock()
	}
	return n, err
}

// MarkOutput stops an output-based watch after the protocol handler has
// identified real client-visible content (text, thought, or tool output).
func (w *FirstByteWatch) MarkOutput() {
	if w == nil {
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.received {
		return
	}
	w.firstByteTime = time.Now()
	w.received = true
	w.timer.Stop()
}

func (w *FirstByteWatch) Stop() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if !w.timedOut {
		w.received = true
	}
	w.timer.Stop()
}

func (w *FirstByteWatch) Pending() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return !w.received
}

// TimedOut reports whether the active deadline expired. The legacy watch
// tolerates a body-read race; output-based watches remain timed out once their
// deadline fired, even if buffered output is parsed afterward.
func (w *FirstByteWatch) TimedOut() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.timedOut && (!w.received || !w.stopOnBodyByte)
}

// FirstByteTime is the first upstream HTTP body byte, not necessarily text.
func (w *FirstByteWatch) FirstByteTime() time.Time {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.firstByteTime
}

// Finish stops the timer and reports whether the first-byte/output deadline
// expired. Legacy HTTP-byte watches keep their original read-race behavior.
func (w *FirstByteWatch) Finish() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.finished = true
	w.timer.Stop()
	return w.timedOut && (!w.received || !w.stopOnBodyByte)
}
