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
	mu            sync.Mutex
	timer         *time.Timer
	finished      bool
	cancel        context.CancelFunc
	timedOut      bool
	received      bool
	firstByteTime time.Time
}

func NewFirstByteWatch(cancel context.CancelFunc, timeout time.Duration) *FirstByteWatch {
	w := &FirstByteWatch{cancel: cancel}
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
	if n > 0 {
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

// TimedOut reports whether the deadline fired without any upstream body byte.
func (w *FirstByteWatch) TimedOut() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.timedOut && !w.received
}

// FirstByteTime is the first upstream HTTP body byte, not necessarily text.
func (w *FirstByteWatch) FirstByteTime() time.Time {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.firstByteTime
}

// Finish stops the timer and returns true only if it fired and no body byte
// was delivered, including a read that raced with cancellation.
func (w *FirstByteWatch) Finish() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.finished = true
	w.timer.Stop()
	return w.timedOut && !w.received
}
