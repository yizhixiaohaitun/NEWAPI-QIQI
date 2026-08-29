package controller

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCopyVideoRequestHeadersPreservesRangeNegotiation(t *testing.T) {
	source := http.Header{
		"Range":              []string{"bytes=1024-2047"},
		"If-Range":           []string{`"video-etag"`},
		"Authorization":      []string{"Bearer client-secret"},
		"X-Unrelated-Header": []string{"do-not-forward"},
	}
	destination := make(http.Header)

	copyVideoRequestHeaders(destination, source)

	assert.Equal(t, "bytes=1024-2047", destination.Get("Range"))
	assert.Equal(t, `"video-etag"`, destination.Get("If-Range"))
	assert.Empty(t, destination.Get("Authorization"))
	assert.Empty(t, destination.Get("X-Unrelated-Header"))
}

func TestWriteProxiedVideoResponseSupportsPartialContent(t *testing.T) {
	upstream := &http.Response{
		StatusCode: http.StatusPartialContent,
		Header: http.Header{
			"Accept-Ranges":       []string{"bytes"},
			"Content-Disposition": []string{`attachment; filename="result.mp4"`},
			"Content-Length":      []string{"4"},
			"Content-Range":       []string{"bytes 4-7/12"},
			"Content-Type":        []string{"application/octet-stream"},
			"Set-Cookie":          []string{"provider-secret=1"},
		},
		Body: io.NopCloser(strings.NewReader("data")),
	}
	recorder := httptest.NewRecorder()

	require.NoError(t, writeProxiedVideoResponse(recorder, upstream, http.MethodGet))

	assert.Equal(t, http.StatusPartialContent, recorder.Code)
	assert.Equal(t, "bytes", recorder.Header().Get("Accept-Ranges"))
	assert.Equal(t, "bytes 4-7/12", recorder.Header().Get("Content-Range"))
	assert.Equal(t, "4", recorder.Header().Get("Content-Length"))
	assert.Equal(t, "video/mp4", recorder.Header().Get("Content-Type"))
	assert.Equal(t, "inline", recorder.Header().Get("Content-Disposition"))
	assert.Equal(t, "private, max-age=86400", recorder.Header().Get("Cache-Control"))
	assert.Empty(t, recorder.Header().Get("Set-Cookie"))
	assert.Equal(t, "data", recorder.Body.String())
}

func TestWriteProxiedVideoResponseHeadHasNoBody(t *testing.T) {
	upstream := &http.Response{
		StatusCode: http.StatusOK,
		Header: http.Header{
			"Content-Length": []string{"4"},
			"Content-Type":   []string{"video/mp4"},
		},
		Body: io.NopCloser(strings.NewReader("data")),
	}
	recorder := httptest.NewRecorder()

	require.NoError(t, writeProxiedVideoResponse(recorder, upstream, http.MethodHead))

	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.Equal(t, "4", recorder.Header().Get("Content-Length"))
	assert.Empty(t, recorder.Body.String())
}
