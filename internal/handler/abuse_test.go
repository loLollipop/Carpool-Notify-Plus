package handler

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"carpool-notify/internal/config"
	"carpool-notify/internal/db"
	"carpool-notify/internal/service"

	"github.com/gin-gonic/gin"
)

func testRequestGuard(t *testing.T) *requestGuard {
	t.Helper()
	guard, err := newRequestGuard(nil, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	return guard
}

func TestAbuseClientKeyCanonicalizesAddresses(t *testing.T) {
	for input, want := range map[string]string{
		"192.0.2.1": "192.0.2.1", "::ffff:192.0.2.1": "192.0.2.1",
		"2001:db8:abcd:1::1":      "2001:db8:abcd:1::/64",
		"2001:db8:abcd:1:ffff::2": "2001:db8:abcd:1::/64",
		"fe80::1%eth0":            "fe80::/64", "invalid": "unknown-client",
	} {
		if got := abuseClientKey(input); got != want {
			t.Errorf("key(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestMalformedLoginBansAllAdminAndRedemptionEntrypoints(t *testing.T) {
	_, router := renewalRoutesTestServer(t)
	client := csrfTestClient{router: router, remoteAddr: "198.51.100.1:1234"}
	for i := 1; i <= loginFailureLimit; i++ {
		response := client.do(t, http.MethodPost, "/api/login", "{", "")
		want := http.StatusBadRequest
		if i == loginFailureLimit {
			want = http.StatusTooManyRequests
		}
		if response.Code != want {
			t.Fatalf("attempt %d status=%d, want %d", i, response.Code, want)
		}
	}
	for _, path := range []string{
		"/api/login", "/api/session", "/api/accounts", "/api/redeem-settings",
		"/api/redeem/token", "/api/sandbox/redeem-settings?sandbox_token=wrong", "/export",
	} {
		method, body := http.MethodGet, ""
		if path == "/api/login" {
			method, body = http.MethodPost, `{"password":"test-password"}`
		}
		response := client.do(t, method, path, body, "")
		if response.Code != http.StatusTooManyRequests || response.Header().Get("Retry-After") == "" {
			t.Fatalf("%s bypassed ban: %d %s", path, response.Code, response.Body.String())
		}
	}
	client.remoteAddr = "198.51.100.2:1234"
	if response := client.do(t, http.MethodPost, "/api/login", `{"password":"test-password"}`, ""); response.Code != http.StatusOK {
		t.Fatalf("another IP was blocked: %d %s", response.Code, response.Body.String())
	}
}

func TestTokenAndSandboxProbesTriggerSharedBan(t *testing.T) {
	for _, path := range []string{"/api/redeem/nonexistent", "/api/sandbox/redeem-settings?sandbox_token=wrong"} {
		t.Run(path, func(t *testing.T) {
			_, router := renewalRoutesTestServer(t)
			client := csrfTestClient{router: router}
			for i := 0; i < probeFailureLimit; i++ {
				response := client.do(t, http.MethodGet, path, "", "")
				if response.Code != http.StatusNotFound && response.Code != http.StatusForbidden {
					t.Fatalf("probe %d: %d %s", i, response.Code, response.Body.String())
				}
			}
			if response := client.do(t, http.MethodGet, "/api/session", "", ""); response.Code != http.StatusTooManyRequests {
				t.Fatalf("probe ban not shared with admin: %d", response.Code)
			}
		})
	}
}

func TestRepeatedRedemptionRateViolationsBanClient(t *testing.T) {
	_, router := renewalRoutesTestServer(t)
	client := csrfTestClient{router: router}
	for i := 0; i < publicSubmitLimit+rateViolationLimit; i++ {
		response := client.do(t, http.MethodPost, "/api/redeem", `{"redeem_code":"wrong"}`, "")
		want := http.StatusBadRequest
		if i >= publicSubmitLimit {
			want = http.StatusTooManyRequests
		}
		if response.Code != want {
			t.Fatalf("submission %d status=%d, want %d: %s", i+1, response.Code, want, response.Body.String())
		}
	}
	if response := client.do(t, http.MethodGet, "/api/redeem-settings", "", ""); response.Code != http.StatusTooManyRequests {
		t.Fatalf("repeated limit violations did not ban client: %d", response.Code)
	}
}

func TestRequestFloodStopsBeforeHandlerAndPreservesOtherClients(t *testing.T) {
	guard := testRequestGuard(t)
	server := &Server{requestGuard: guard}
	router := gin.New()
	_ = router.SetTrustedProxies(nil)
	router.Use(server.protectRequests())
	var handled int
	router.GET("/api/redeem-settings", func(c *gin.Context) { handled++; c.Status(http.StatusOK) })
	for i := 0; i <= requestBurst; i++ {
		response := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodGet, "/api/redeem-settings", nil)
		request.RemoteAddr = "192.0.2.1:1234"
		request.Header.Set("X-Forwarded-For", fmt.Sprintf("198.51.100.%d", i))
		router.ServeHTTP(response, request)
		if i == requestBurst && response.Code != http.StatusTooManyRequests {
			t.Fatalf("flood not blocked: %d", response.Code)
		}
	}
	if handled != requestBurst {
		t.Fatalf("handled=%d, want %d", handled, requestBurst)
	}
	if retry := guard.retryAfter("192.0.2.1", time.Now().UTC()); retry < 23*time.Hour {
		t.Fatalf("flood ban duration=%s", retry)
	}
	if retry := guard.allowRequest("192.0.2.2", time.Now().UTC()); retry != 0 {
		t.Fatalf("another IP was blocked: %s", retry)
	}
}

func TestLoginFloodCannotBypassBudgetConcurrently(t *testing.T) {
	guard := testRequestGuard(t)
	now := time.Now().UTC()
	var admitted atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 40; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if guard.beginLogin("192.0.2.1", now) == 0 {
				admitted.Add(1)
			}
		}()
	}
	wg.Wait()
	if got := admitted.Load(); got != loginAttemptLimit {
		t.Fatalf("concurrent admitted=%d, want %d", got, loginAttemptLimit)
	}
	granted := false
	retry, err := guard.completeLogin("192.0.2.1", now, func() error { granted = true; return nil })
	if err != nil || retry != ipBlockDuration || granted {
		t.Fatalf("in-flight login bypassed ban: retry=%s err=%v granted=%t", retry, err, granted)
	}
}

func TestLoginPasswordChecksHaveGlobalConcurrencyLimit(t *testing.T) {
	guard := testRequestGuard(t)
	for i := 0; i < maxConcurrentLogins; i++ {
		guard.loginSlots <- struct{}{}
	}
	server := &Server{requestGuard: guard}
	response := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(response)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/login", strings.NewReader(`{"password":"test-password"}`))
	c.Request.RemoteAddr = "192.0.2.1:1234"
	server.postLogin(c)
	if response.Code != http.StatusServiceUnavailable || response.Header().Get("Retry-After") == "" {
		t.Fatalf("full login slots status=%d retry=%q", response.Code, response.Header().Get("Retry-After"))
	}
	if guard.states["192.0.2.1"].loginFailures.Count != 0 {
		t.Fatal("server overload was counted as a password failure")
	}
}

func TestSuccessfulLoginsKeepAttemptBudget(t *testing.T) {
	guard := testRequestGuard(t)
	now := time.Now().UTC()
	for i := 0; i < loginAttemptLimit; i++ {
		if retry := guard.beginLogin("client", now); retry != 0 {
			t.Fatalf("attempt %d blocked early", i)
		}
		guard.recordFailure("client", "login", now)
		if retry, err := guard.completeLogin("client", now, func() error { return nil }); retry != 0 || err != nil {
			t.Fatalf("successful login failed: %s %v", retry, err)
		}
	}
	if retry := guard.beginLogin("client", now); retry != ipBlockDuration {
		t.Fatalf("success cleared attempt budget: %s", retry)
	}
}

func TestNormalPollingAndBanExpiry(t *testing.T) {
	guard := testRequestGuard(t)
	limiter := newFixedWindowLimiter(publicStatusLimit, publicStatusWindow)
	t.Cleanup(limiter.Stop)
	now := time.Now().UTC()
	for i := 0; i < 24*60*12; i++ {
		at := now.Add(time.Duration(i) * 5 * time.Second)
		if retry := guard.allowRequest("normal", at); retry != 0 {
			t.Fatalf("normal polling banned at %d: %s", i, retry)
		}
		if allowed, _ := limiter.allow("normal", at); !allowed {
			t.Fatalf("normal polling limited at %d", i)
		}
	}
	for i := 0; i < loginFailureLimit; i++ {
		guard.recordFailure("attacker", "login", now)
	}
	if retry := guard.allowRequest("attacker", now.Add(ipBlockDuration-time.Nanosecond)); retry <= 0 {
		t.Fatal("ban expired early")
	}
	if retry := guard.allowRequest("attacker", now.Add(ipBlockDuration)); retry != 0 {
		t.Fatalf("ban did not expire: %s", retry)
	}
}

func TestProtectionStateCapacityCannotEvictActiveBudgetOrBan(t *testing.T) {
	guard := testRequestGuard(t)
	now := time.Now().UTC()
	for i := 0; i < loginFailureLimit; i++ {
		guard.recordFailure("banned", "login", now)
	}
	for i := 0; i < maxAbuseStates; i++ {
		if retry := guard.allowRequest(fmt.Sprint(i), now); retry != 0 {
			t.Fatalf("unexpected capacity at %d", i)
		}
	}
	if retry := guard.allowRequest("overflow", now); retry <= 0 {
		t.Fatal("new client admitted at capacity")
	}
	if len(guard.states) != maxAbuseStates || guard.states["0"].tokens != requestBurst-1 {
		t.Fatal("capacity pressure evicted/reset an existing budget")
	}
	if retry := guard.retryAfter("banned", now); retry != ipBlockDuration {
		t.Fatalf("capacity pressure evicted ban: %s", retry)
	}
	limiter := newFixedWindowLimiter(1, time.Minute)
	t.Cleanup(limiter.Stop)
	limiter.maxKeys = 1
	limiter.allow("first", now)
	if allowed, _ := limiter.allow("overflow", now); allowed {
		t.Fatal("public limiter admitted a new client at capacity")
	}
	if allowed, _ := limiter.allow("first", now); allowed {
		t.Fatal("capacity pressure reset public rate budget")
	}
}

func TestIPBanRestoredByNewServer(t *testing.T) {
	path := filepath.Join(t.TempDir(), "security.db")
	store, err := db.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	guard, err := newRequestGuard(store, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < loginFailureLimit; i++ {
		guard.recordFailure("192.0.2.1", "login", time.Now().UTC())
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = db.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	server, err := NewServer(&service.SubscriptionService{Store: store}, config.Config{Password: "test-password"}, "")
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	if retry := server.requestGuard.retryAfter("192.0.2.1", time.Now().UTC()); retry < 23*time.Hour {
		t.Fatalf("ban lost after restart: %s", retry)
	}
}

func TestBanCapacityReservesSlotAndRejectsUnknownClientsAcrossRoutes(t *testing.T) {
	guard := testRequestGuard(t)
	now := time.Now().UTC()
	for i := 0; i < maxBlockedClients-1; i++ {
		guard.blocks[fmt.Sprintf("blocked-%d", i)] = now.Add(ipBlockDuration)
	}
	for i := 0; i < loginFailureLimit; i++ {
		guard.recordFailure("192.0.2.1", "login", now)
	}
	if len(guard.blocks) != maxBlockedClients || guard.retryAfter("192.0.2.1", now) != ipBlockDuration {
		t.Fatal("last admitted client did not receive its reserved ban slot")
	}
	if retry := guard.beginLogin("192.0.2.2", now); retry <= 0 {
		t.Fatal("new login admitted with no capacity to enforce a ban")
	}
	server := &Server{requestGuard: guard}
	router := gin.New()
	router.Use(server.protectRequests())
	for _, path := range []string{"/api/session", "/api/redeem-settings", "/api/sandbox/redeem-settings"} {
		router.GET(path, func(c *gin.Context) { t.Error("capacity bypass reached handler"); c.Status(http.StatusOK) })
	}
	for _, ip := range []string{"192.0.2.1", "192.0.2.2"} {
		for _, path := range []string{"/api/session", "/api/redeem-settings", "/api/sandbox/redeem-settings"} {
			request := httptest.NewRequest(http.MethodGet, path, nil)
			request.RemoteAddr = ip + ":1234"
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Code != http.StatusTooManyRequests {
				t.Fatalf("capacity bypass %s %s: %d", ip, path, response.Code)
			}
		}
	}
	if len(guard.states) != 0 || len(guard.blocks) != maxBlockedClients {
		t.Fatal("capacity denial grew state or evicted an existing ban")
	}
}
