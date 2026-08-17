package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestAPIResponsesDisableCaching(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name       string
		respond    func(*gin.Context)
		wantStatus int
		wantBody   string
	}{
		{
			name: "success",
			respond: func(context *gin.Context) {
				respondOK(context, gin.H{"message": "saved"})
			},
			wantStatus: http.StatusOK,
			wantBody:   `"ok":true`,
		},
		{
			name: "error",
			respond: func(context *gin.Context) {
				respondError(context, http.StatusBadRequest, "invalid")
			},
			wantStatus: http.StatusBadRequest,
			wantBody:   `"ok":false`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			context, _ := gin.CreateTestContext(recorder)

			test.respond(context)

			if recorder.Code != test.wantStatus {
				t.Fatalf("status = %d, want %d", recorder.Code, test.wantStatus)
			}
			if got := recorder.Header().Get("Cache-Control"); got != "no-store" {
				t.Fatalf("Cache-Control = %q, want no-store", got)
			}
			if !strings.Contains(recorder.Body.String(), test.wantBody) {
				t.Fatalf("body = %q, want to contain %q", recorder.Body.String(), test.wantBody)
			}
		})
	}
}
