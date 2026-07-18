package channel_purity

import (
	"bytes"
	"io"
	"net/http"
)

const DefaultObservationLimit = 1 << 20

type FeatureSink interface{ Observe(AnonymousFeatures) }
type FeatureSinkFunc func(AnonymousFeatures)

func (f FeatureSinkFunc) Observe(v AnonymousFeatures) { f(v) }

type RawSinkFunc func(status int, header http.Header, body []byte, truncated bool)

type observingBody struct {
	body      io.ReadCloser
	buf       bytes.Buffer
	limit     int
	truncated bool
	done      bool
	status    int
	header    http.Header
	sink      FeatureSink
	rawSink   RawSinkFunc
}

func ObserveResponse(resp *http.Response, sink FeatureSink, limit int) {
	observeResponse(resp, sink, nil, limit)
}

func ObserveRawResponse(resp *http.Response, sink RawSinkFunc, limit int) {
	observeResponse(resp, nil, sink, limit)
}

func observeResponse(resp *http.Response, sink FeatureSink, rawSink RawSinkFunc, limit int) {
	if resp == nil || resp.Body == nil || (sink == nil && rawSink == nil) {
		return
	}
	if limit <= 0 {
		limit = DefaultObservationLimit
	}
	resp.Body = &observingBody{body: resp.Body, limit: limit, status: resp.StatusCode, header: resp.Header.Clone(), sink: sink, rawSink: rawSink}
}

func (b *observingBody) Read(p []byte) (int, error) {
	n, err := b.body.Read(p)
	if n > 0 {
		remaining := b.limit - b.buf.Len()
		if remaining > 0 {
			keep := n
			if keep > remaining {
				keep = remaining
			}
			_, _ = b.buf.Write(p[:keep])
		}
		if n > remaining {
			b.truncated = true
		}
	}
	if err != nil {
		b.finish()
	}
	return n, err
}
func (b *observingBody) Close() error { b.finish(); return b.body.Close() }
func (b *observingBody) finish() {
	if b.done {
		return
	}
	b.done = true
	defer func() { _ = recover() }()
	if b.rawSink != nil {
		b.rawSink(b.status, b.header, b.buf.Bytes(), b.truncated)
	}
	if b.sink != nil {
		b.sink.Observe(ExtractAnonymousFeatures(b.status, b.header, b.buf.Bytes(), b.truncated))
	}
}
