package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type panicAuditBody struct{}

func (panicAuditBody) Read([]byte) (int, error) { panic("disabled audit path read body") }
func (panicAuditBody) Close() error             { return nil }

func TestContextRequestAuditDisabledDoesNotReadBodyOrWrapWriter(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setting := operation_setting.GetQiqiSetting()
	previous := setting.ContextRequestLoggingEnabled
	setting.ContextRequestLoggingEnabled = false
	t.Cleanup(func() { setting.ContextRequestLoggingEnabled = previous })
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	c.Request.Body = panicAuditBody{}
	original := c.Writer
	ContextRequestAudit()(c)
	require.Same(t, original, c.Writer)
}
