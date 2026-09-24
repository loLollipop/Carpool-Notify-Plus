package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"carpool-notify/internal/config"

	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
)

func TestAdminPasswordChangeInvalidatesExistingSession(t *testing.T) {
	gin.SetMode(gin.TestMode)
	configuration := config.Config{Password: "old-password", SessionSecret: "session-secret"}
	oldRouter := authTestRouter(t, configuration)
	oldCookie := authTestLogin(t, oldRouter, "old-password", http.StatusOK)
	authTestRequest(t, oldRouter, "/api/private", oldCookie, http.StatusNoContent)
	authTestSession(t, oldRouter, oldCookie, true)

	// A normal restart with unchanged credentials must keep a valid session.
	unchangedRouter := authTestRouter(t, configuration)
	authTestRequest(t, unchangedRouter, "/api/private", oldCookie, http.StatusNoContent)
	authTestSession(t, unchangedRouter, oldCookie, true)

	configuration.Password = "new-password"
	changedRouter := authTestRouter(t, configuration)
	authTestRequest(t, changedRouter, "/api/private", oldCookie, http.StatusUnauthorized)
	authTestSession(t, changedRouter, oldCookie, false)
	authTestLogin(t, changedRouter, "old-password", http.StatusUnauthorized)
	newCookie := authTestLogin(t, changedRouter, "new-password", http.StatusOK)
	authTestRequest(t, changedRouter, "/api/private", newCookie, http.StatusNoContent)
}

func authTestSession(t *testing.T, router http.Handler, sessionCookie *http.Cookie, wantAuthenticated bool) {
	t.Helper()
	recorder := authTestRequest(t, router, "/api/session", sessionCookie, http.StatusOK)
	var response struct {
		Authenticated bool `json:"authenticated"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode session response: %v: %s", err, recorder.Body.String())
	}
	if response.Authenticated != wantAuthenticated {
		t.Fatalf("authenticated = %t, want %t: %s", response.Authenticated, wantAuthenticated, recorder.Body.String())
	}
}

func authTestRouter(t *testing.T, configuration config.Config) *gin.Engine {
	t.Helper()
	server, err := NewServer(nil, configuration, "")
	if err != nil {
		t.Fatal(err)
	}
	router := gin.New()
	store := cookie.NewStore([]byte(configuration.SessionSecret))
	router.Use(sessions.Sessions("auth_test_session", store))
	router.POST("/api/login", server.postLogin)
	router.GET("/api/session", server.getSession)
	router.GET("/api/private", server.requireAPIAuth(), func(context *gin.Context) {
		context.Status(http.StatusNoContent)
	})
	return router
}

func authTestLogin(t *testing.T, router http.Handler, password string, wantStatus int) *http.Cookie {
	t.Helper()
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/login", strings.NewReader(`{"password":"`+password+`"}`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)
	if recorder.Code != wantStatus {
		t.Fatalf("login status = %d, want %d: %s", recorder.Code, wantStatus, recorder.Body.String())
	}
	if wantStatus != http.StatusOK {
		return nil
	}
	cookies := recorder.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatal("successful login did not set a session cookie")
	}
	return cookies[0]
}

func authTestRequest(t *testing.T, router http.Handler, path string, sessionCookie *http.Cookie, wantStatus int) *httptest.ResponseRecorder {
	t.Helper()
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, path, nil)
	if sessionCookie != nil {
		request.AddCookie(sessionCookie)
	}
	router.ServeHTTP(recorder, request)
	if recorder.Code != wantStatus {
		t.Fatalf("GET %s status = %d, want %d: %s", path, recorder.Code, wantStatus, recorder.Body.String())
	}
	return recorder
}
