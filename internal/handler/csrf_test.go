package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"carpool-notify/internal/config"

	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
)

func TestCSRFProtectionUsesCookieSessionBinding(t *testing.T) {
	gin.SetMode(gin.TestMode)
	server, err := NewServer(nil, config.Config{
		Password:      "test-password",
		SessionSecret: "test-session-secret",
	}, "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(server.Close)

	router := gin.New()
	store := cookie.NewStore([]byte("test-session-secret"))
	router.Use(sessions.Sessions("csrf_test_session", store))
	router.GET("/csrf", server.getCSRFToken)
	router.POST("/login", server.postLogin)
	router.POST("/logout", server.requireAPIAuth(), server.CSRFProtection(), server.postLogout)
	router.POST("/private", server.requireAPIAuth(), server.CSRFProtection(), func(context *gin.Context) {
		context.Status(http.StatusNoContent)
	})

	client := csrfTestClient{router: router}
	login := client.do(t, http.MethodPost, "/login", `{"password":"test-password"}`, "")
	if login.Code != http.StatusOK {
		t.Fatalf("login status = %d, want 200: %s", login.Code, login.Body.String())
	}

	first := client.do(t, http.MethodGet, "/csrf", "", "")
	firstToken := decodeCSRFToken(t, first)
	if first.Code != http.StatusOK {
		t.Fatalf("first token status = %d, want 200: %s", first.Code, first.Body.String())
	}

	second := client.do(t, http.MethodGet, "/csrf", "", "")
	secondToken := decodeCSRFToken(t, second)
	if second.Code != http.StatusOK {
		t.Fatalf("second token status = %d, want 200: %s", second.Code, second.Body.String())
	}

	missing := client.do(t, http.MethodPost, "/private", "", "")
	if missing.Code != http.StatusForbidden {
		t.Fatalf("missing token status = %d, want 403", missing.Code)
	}
	wrong := client.do(t, http.MethodPost, "/private", "", "wrong-token")
	if wrong.Code != http.StatusForbidden {
		t.Fatalf("wrong token status = %d, want 403", wrong.Code)
	}
	valid := client.do(t, http.MethodPost, "/private", "", firstToken)
	if valid.Code != http.StatusNoContent {
		t.Fatalf("valid token status = %d, want 204: %s", valid.Code, valid.Body.String())
	}
	validSecond := client.do(t, http.MethodPost, "/private", "", secondToken)
	if validSecond.Code != http.StatusNoContent {
		t.Fatalf("second token status = %d, want 204: %s", validSecond.Code, validSecond.Body.String())
	}

	logout := client.do(t, http.MethodPost, "/logout", "", firstToken)
	if logout.Code != http.StatusOK {
		t.Fatalf("logout status = %d, want 200: %s", logout.Code, logout.Body.String())
	}
	lateToken := client.doWithCookie(t, http.MethodGet, "/csrf", "", "", login.Result().Cookies()[0])
	if lateToken.Code != http.StatusOK || len(lateToken.Result().Cookies()) != 0 {
		t.Fatalf("token endpoint unexpectedly rewrote session: status=%d cookies=%d", lateToken.Code, len(lateToken.Result().Cookies()))
	}
	afterLogout := client.do(t, http.MethodPost, "/private", "", decodeCSRFToken(t, lateToken))
	if afterLogout.Code != http.StatusUnauthorized {
		t.Fatalf("late token restored logged-out session: status=%d", afterLogout.Code)
	}
}

func TestCSRFTokenExpiresAndIsBoundToSession(t *testing.T) {
	manager := newCSRFManager("test-secret")
	expired := manager.generateToken("binding-a", time.Now().UTC().Add(-csrfTokenTTL-time.Minute))
	if manager.validateToken("binding-a", expired, time.Now().UTC()) {
		t.Fatal("expired CSRF token was accepted")
	}
	valid := manager.generateToken("binding-a", time.Now().UTC())
	if manager.validateToken("binding-b", valid, time.Now().UTC()) {
		t.Fatal("CSRF token was accepted for another session binding")
	}
}

type csrfTestClient struct {
	router http.Handler
	cookie *http.Cookie
}

func (client *csrfTestClient) do(t *testing.T, method, path, body, token string) *httptest.ResponseRecorder {
	return client.doWithCookie(t, method, path, body, token, client.cookie)
}

func (client *csrfTestClient) doWithCookie(t *testing.T, method, path, body, token string, sessionCookie *http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	if sessionCookie != nil {
		request.AddCookie(sessionCookie)
	}
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		request.Header.Set(csrfTokenHeader, token)
	}
	recorder := httptest.NewRecorder()
	client.router.ServeHTTP(recorder, request)
	if sessionCookie == client.cookie {
		for _, cookie := range recorder.Result().Cookies() {
			if cookie.Name == "csrf_test_session" {
				client.cookie = cookie
			}
		}
	}
	return recorder
}

func decodeCSRFToken(t *testing.T, recorder *httptest.ResponseRecorder) string {
	t.Helper()
	var response struct {
		CSRFToken string `json:"csrf_token"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode CSRF response: %v: %s", err, recorder.Body.String())
	}
	return response.CSRFToken
}
