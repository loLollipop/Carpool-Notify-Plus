package handler

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func TestLoginFailureStateHasHardCapacity(t *testing.T) {
	server := &Server{}
	now := time.Now().UTC()
	for index := 0; index < maxLoginStates+100; index++ {
		server.recordLoginFailure(fmt.Sprintf("203.0.113.%d", index), now.Add(time.Duration(index)*time.Second))
	}
	if got := len(server.loginFailures); got != maxLoginStates {
		t.Fatalf("login failure states = %d, want hard cap %d", got, maxLoginStates)
	}
}

func TestSecurityHeadersProtectAPIResponses(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(SecurityHeaders())
	router.GET("/api/private", func(context *gin.Context) {
		context.JSON(http.StatusOK, gin.H{"ok": true})
	})

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/private", nil))

	for key, want := range map[string]string{
		"Cache-Control":           "no-store",
		"Pragma":                  "no-cache",
		"X-Content-Type-Options":  "nosniff",
		"X-Frame-Options":         "DENY",
		"Referrer-Policy":         "same-origin",
		"Content-Security-Policy": "frame-ancestors 'none'",
	} {
		if got := recorder.Header().Get(key); got != want {
			t.Fatalf("%s = %q, want %q", key, got, want)
		}
	}
}

func TestLimitRequestBodyRejectsKnownOversizedPayload(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(LimitRequestBody(8))
	router.POST("/api/test", func(context *gin.Context) {
		context.Status(http.StatusNoContent)
	})

	recorder := httptest.NewRecorder()
	router.ServeHTTP(
		recorder,
		httptest.NewRequest(http.MethodPost, "/api/test", strings.NewReader("123456789")),
	)
	if recorder.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusRequestEntityTooLarge)
	}
}

func TestRequestLoggerRedactsBearerTokens(t *testing.T) {
	request := httptest.NewRequest(
		http.MethodGet,
		"/api/sandbox/redeem/tracking-secret?sandbox_token=access-secret&q=visible",
		nil,
	)
	target := redactedRequestTarget(request)
	if strings.Contains(target, "tracking-secret") || strings.Contains(target, "access-secret") {
		t.Fatalf("request target leaked a token: %q", target)
	}
	if !strings.Contains(target, "/api/sandbox/redeem/:token") || !strings.Contains(target, "q=visible") {
		t.Fatalf("request target lost useful routing details: %q", target)
	}
}

func TestLoginRateLimitBlocksRepeatedFailures(t *testing.T) {
	gin.SetMode(gin.TestMode)
	server := &Server{}
	for attempt := 1; attempt <= loginFailureLimit+1; attempt++ {
		recorder := httptest.NewRecorder()
		context, _ := gin.CreateTestContext(recorder)
		context.Request = httptest.NewRequest(
			http.MethodPost,
			"/api/login",
			strings.NewReader(`{"password":"wrong"}`),
		)
		context.Request.Header.Set("Content-Type", "application/json")
		context.Request.RemoteAddr = "203.0.113.10:4321"
		server.postLogin(context)

		if attempt <= loginFailureLimit && recorder.Code != http.StatusUnauthorized {
			t.Fatalf("attempt %d status = %d, want 401", attempt, recorder.Code)
		}
		if attempt == loginFailureLimit+1 {
			if recorder.Code != http.StatusTooManyRequests {
				t.Fatalf("blocked attempt status = %d, want 429", recorder.Code)
			}
			if recorder.Header().Get("Retry-After") == "" {
				t.Fatal("blocked response is missing Retry-After")
			}
		}
	}
}
