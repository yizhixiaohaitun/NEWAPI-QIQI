package helper

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newBufferedWriterTestContext() (*gin.Context, *httptest.ResponseRecorder) {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/stream", nil)
	return c, recorder
}

func TestBufferedStreamWriterHoldsBytesUntilCommit(t *testing.T) {
	c, recorder := newBufferedWriterTestContext()
	writer := NewBufferedStreamWriter(c.Writer)
	c.Writer = writer

	_, err := c.Writer.Write([]byte("metadata"))
	require.NoError(t, err)
	c.Writer.Flush()
	assert.False(t, recorder.Flushed)
	assert.Empty(t, recorder.Body.String())

	require.NoError(t, writer.Commit())
	assert.Equal(t, "metadata", recorder.Body.String())
	assert.True(t, writer.Committed())
}

func TestBufferedStreamWriterDiscardDoesNotLeakBytes(t *testing.T) {
	c, recorder := newBufferedWriterTestContext()
	writer := NewBufferedStreamWriter(c.Writer)
	c.Writer = writer

	_, err := c.Writer.Write([]byte("message_start"))
	require.NoError(t, err)
	writer.Discard()

	assert.Empty(t, recorder.Body.String())
	assert.False(t, writer.Committed())
}

func TestBufferedStreamWriterCopiesHeadersOnCommit(t *testing.T) {
	c, recorder := newBufferedWriterTestContext()
	writer := NewBufferedStreamWriter(c.Writer)
	c.Writer = writer
	writer.Header().Set("Content-Type", "text/event-stream")
	writer.WriteHeader(http.StatusOK)

	require.NoError(t, writer.Commit())
	assert.Equal(t, "text/event-stream", recorder.Header().Get("Content-Type"))
	assert.Equal(t, http.StatusOK, recorder.Code)
}
