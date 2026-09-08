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

func TestRequestLoggerRedactsRenewalTrackingTokens(t *testing.T) {
	request := httptest.NewRequest(
		http.MethodGet,
		"/api/renewal/renewal-secret",
		nil,
	)
	target := redactedRequestTarget(request)
	if strings.Contains(target, "renewal-secret") {
		t.Fatalf("request target leaked a renewal token: %q", target)
	}
	if target != "/api/renewal/:token" {
		t.Fatalf("request target = %q, want redacted renewal route", target)
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

func TestPublicRateLimiterSeparatesClientsAndResets(t *testing.T) {
	limiter := newFixedWindowLimiter(2, time.Minute)
	now := time.Date(2026, time.September, 6, 12, 0, 0, 0, time.UTC)
	if allowed, _ := limiter.allow("198.51.100.1", now); !allowed {
		t.Fatal("first request was blocked")
	}
	if allowed, _ := limiter.allow("198.51.100.1", now.Add(time.Second)); !allowed {
		t.Fatal("second request was blocked")
	}
	if allowed, retryAfter := limiter.allow("198.51.100.1", now.Add(2*time.Second)); allowed || retryAfter <= 0 {
		t.Fatalf("third request allowed=%t retry=%s, want blocked", allowed, retryAfter)
	}
	if allowed, _ := limiter.allow("198.51.100.2", now.Add(2*time.Second)); !allowed {
		t.Fatal("one client exhausted another client's allowance")
	}
	if allowed, _ := limiter.allow("198.51.100.1", now.Add(time.Minute)); !allowed {
		t.Fatal("request was not allowed after the window reset")
	}
}

func TestPublicRateLimitMiddlewareReturnsRetryAfter(t *testing.T) {
	gin.SetMode(gin.TestMode)
	server := &Server{}
	limiter := newFixedWindowLimiter(1, time.Minute)
	router := gin.New()
	router.GET("/api/redeem/:token", server.limitPublicRequests(limiter), func(context *gin.Context) {
		context.Status(http.StatusNoContent)
	})

	first := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/redeem/token", nil)
	request.RemoteAddr = "203.0.113.10:4321"
	router.ServeHTTP(first, request)
	if first.Code != http.StatusNoContent {
		t.Fatalf("first status = %d, want 204", first.Code)
	}

	second := httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/api/redeem/token", nil)
	request.RemoteAddr = "203.0.113.10:4321"
	router.ServeHTTP(second, request)
	if second.Code != http.StatusTooManyRequests || second.Header().Get("Retry-After") == "" {
		t.Fatalf("limited response status=%d retry-after=%q", second.Code, second.Header().Get("Retry-After"))
	}
}
