package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestSandboxSettingsTestNotifyIsBlocked(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodPost, "/api/sandbox/settings/test-notify", nil)

	server := &Server{SandboxMode: true}
	server.postSettingsTestNotify(context)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusBadRequest)
	}
	if !strings.Contains(recorder.Body.String(), "演练模式不会发送真实通知") {
		t.Fatalf("body = %q", recorder.Body.String())
	}
}
