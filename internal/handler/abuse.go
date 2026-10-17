package handler

import (
	"log"
	"net/http"
	"net/netip"
	"strings"
	"sync"
	"time"

	"carpool-notify/internal/db"

	"github.com/gin-gonic/gin"
)

const (
	ipBlockDuration      = 24 * time.Hour
	requestBurst         = 120
	requestRatePerSecond = 6
	loginAttemptLimit    = 10
	loginAttemptWindow   = time.Minute
	loginFailureLimit    = 5
	loginFailureWindow   = 15 * time.Minute
	probeFailureLimit    = 20
	probeFailureWindow   = time.Minute
	rateViolationLimit   = 3
	rateViolationWindow  = time.Minute
	abuseStateTTL        = 30 * time.Minute
	maxAbuseStates       = 4096
	maxBlockedClients    = 8192
	maxConcurrentLogins  = 4
)

type abuseState struct {
	lastSeen       time.Time
	tokens         float64
	refilledAt     time.Time
	loginAttempts  fixedWindowState
	loginFailures  fixedWindowState
	probeFailures  fixedWindowState
	rateViolations fixedWindowState
}

// A single guard owns live and sandbox protection. Only new blocks write to
// SQLite; blocked traffic is rejected from memory before credentials or tokens
// are checked. Active bans are never evicted to make room for another client.
type requestGuard struct {
	mu          sync.Mutex
	store       *db.Store
	states      map[string]*abuseState
	blocks      map[string]time.Time
	nextCleanup time.Time
	loginSlots  chan struct{}
}

func newRequestGuard(store *db.Store, now time.Time) (*requestGuard, error) {
	guard := &requestGuard{
		store: store, states: make(map[string]*abuseState), blocks: make(map[string]time.Time),
		loginSlots: make(chan struct{}, maxConcurrentLogins),
	}
	if store != nil {
		blocks, err := store.ListActiveIPBlocks(now)
		if err != nil {
			return nil, err
		}
		for _, block := range blocks {
			guard.blocks[block.ClientKey] = block.BlockedUntil
		}
	}
	return guard, nil
}

func (server *Server) securityGuard() (*requestGuard, error) {
	server.securityMu.Lock()
	defer server.securityMu.Unlock()
	if server.requestGuard == nil {
		var store *db.Store
		if server.Service != nil {
			store = server.Service.Store
		}
		guard, err := newRequestGuard(store, time.Now().UTC())
		if err != nil {
			return nil, err
		}
		server.requestGuard = guard
	}
	return server.requestGuard, nil
}

// IPv4-mapped IPv6 is canonicalized; native IPv6 shares one /64 budget so an
// attacker cannot rotate interface addresses to reset counters or a ban.
func abuseClientKey(address string) string {
	ip, err := netip.ParseAddr(address)
	if err != nil {
		return "unknown-client"
	}
	ip = ip.Unmap().WithZone("")
	if ip.Is6() {
		return netip.PrefixFrom(ip, 64).Masked().String()
	}
	return ip.String()
}

func (guard *requestGuard) retryAfterLocked(key string, now time.Time) time.Duration {
	if until, exists := guard.blocks[key]; exists {
		if now.Before(until) {
			return until.Sub(now)
		}
		delete(guard.blocks, key)
	}
	return 0
}

func (guard *requestGuard) retryAfter(key string, now time.Time) time.Duration {
	guard.mu.Lock()
	defer guard.mu.Unlock()
	return guard.retryAfterLocked(key, now)
}

func (guard *requestGuard) stateLocked(key string, now time.Time) *abuseState {
	if !now.Before(guard.nextCleanup) {
		for client, state := range guard.states {
			if now.Sub(state.lastSeen) >= abuseStateTTL {
				delete(guard.states, client)
			}
		}
		for client, until := range guard.blocks {
			if !now.Before(until) {
				delete(guard.blocks, client)
			}
		}
		guard.nextCleanup = now.Add(time.Minute)
	}
	state := guard.states[key]
	if state == nil {
		// Reserve a persistent ban slot for every admitted client. Converting
		// its counter to a ban must always fit without evicting another ban.
		// At capacity, unknown clients are rejected before any endpoint runs.
		if len(guard.states) >= maxAbuseStates || len(guard.states)+len(guard.blocks) >= maxBlockedClients {
			return nil
		}
		state = &abuseState{tokens: requestBurst, refilledAt: now}
		guard.states[key] = state
	}
	state.lastSeen = now
	return state
}

func (guard *requestGuard) blockLocked(key, reason string, now time.Time) time.Duration {
	if retry := guard.retryAfterLocked(key, now); retry > 0 {
		return retry
	}
	until := now.Add(ipBlockDuration)
	guard.blocks[key] = until
	delete(guard.states, key)
	if guard.store != nil {
		if err := guard.store.PutIPBlock(key, until, reason, now); err != nil {
			// Keep the in-memory ban even when storage is unavailable.
			log.Printf("persist blocked client %s: %v", key, err)
		}
	}
	log.Printf("blocked client %s until %s: %s", key, until.Format(time.RFC3339), reason)
	return ipBlockDuration
}

func incrementWindow(state *fixedWindowState, window time.Duration, now time.Time) int {
	if state.StartedAt.IsZero() || now.Sub(state.StartedAt) >= window {
		*state = fixedWindowState{StartedAt: now}
	}
	state.Count++
	return state.Count
}

func (guard *requestGuard) allowRequest(key string, now time.Time) time.Duration {
	guard.mu.Lock()
	defer guard.mu.Unlock()
	if retry := guard.retryAfterLocked(key, now); retry > 0 {
		return retry
	}
	state := guard.stateLocked(key, now)
	if state == nil {
		return time.Minute
	}
	if elapsed := now.Sub(state.refilledAt).Seconds(); elapsed > 0 {
		state.tokens = min(requestBurst, state.tokens+elapsed*requestRatePerSecond)
		state.refilledAt = now
	}
	if state.tokens < 1 {
		return guard.blockLocked(key, "request flood", now)
	}
	state.tokens--
	return 0
}

func (guard *requestGuard) beginLogin(key string, now time.Time) time.Duration {
	guard.mu.Lock()
	defer guard.mu.Unlock()
	if retry := guard.retryAfterLocked(key, now); retry > 0 {
		return retry
	}
	state := guard.stateLocked(key, now)
	if state == nil {
		return time.Minute
	}
	if incrementWindow(&state.loginAttempts, loginAttemptWindow, now) > loginAttemptLimit {
		return guard.blockLocked(key, "login request flood", now)
	}
	return 0
}

func (guard *requestGuard) recordFailure(key, kind string, now time.Time) time.Duration {
	guard.mu.Lock()
	defer guard.mu.Unlock()
	if retry := guard.retryAfterLocked(key, now); retry > 0 {
		return retry
	}
	state := guard.stateLocked(key, now)
	if state == nil {
		return time.Minute
	}
	var count, limit int
	switch kind {
	case "login":
		count, limit = incrementWindow(&state.loginFailures, loginFailureWindow, now), loginFailureLimit
	case "rate":
		count, limit = incrementWindow(&state.rateViolations, rateViolationWindow, now), rateViolationLimit
	default:
		count, limit = incrementWindow(&state.probeFailures, probeFailureWindow, now), probeFailureLimit
	}
	if count >= limit {
		return guard.blockLocked(key, kind+" failures", now)
	}
	return 0
}

// Serialize the final ban check and session grant with concurrent failures.
// A successful login never clears a ban or the login request budget.
func (guard *requestGuard) completeLogin(key string, now time.Time, grant func() error) (time.Duration, error) {
	guard.mu.Lock()
	defer guard.mu.Unlock()
	if retry := guard.retryAfterLocked(key, now); retry > 0 {
		return retry, nil
	}
	if err := grant(); err != nil {
		return 0, err
	}
	if state := guard.states[key]; state != nil {
		state.loginFailures = fixedWindowState{}
	}
	return 0, nil
}

func protectedRequestPath(path string) bool {
	return path == "/api" || strings.HasPrefix(path, "/api/") ||
		path == "/export" || path == "/login" || path == "/redeem"
}

func isProbeFailure(path string, status int) bool {
	if path == "/api/login" {
		return false // Counted by the login handler, including malformed JSON.
	}
	if status == http.StatusUnauthorized || status == http.StatusForbidden || status == http.StatusNotFound {
		return true
	}
	return status == http.StatusBadRequest && (path == "/api/redeem" || path == "/api/sandbox/redeem")
}

func (server *Server) protectRequests() gin.HandlerFunc {
	return func(context *gin.Context) {
		if !protectedRequestPath(context.Request.URL.Path) {
			context.Next()
			return
		}
		guard, err := server.securityGuard()
		if err != nil {
			context.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{"ok": false, "error": "安全防护暂不可用，请稍后再试"})
			return
		}
		key := abuseClientKey(context.ClientIP())
		if retry := guard.allowRequest(key, time.Now().UTC()); retry > 0 {
			respondIPBlocked(context, retry)
			return
		}
		context.Next()
		if isProbeFailure(context.Request.URL.Path, context.Writer.Status()) {
			guard.recordFailure(key, "probe", time.Now().UTC())
		}
	}
}

func respondIPBlocked(context *gin.Context, retry time.Duration) {
	setRetryAfter(context, retry)
	context.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
		"ok": false, "error": "访问过于频繁或失败次数过多，当前 IP 已被临时限制，请稍后再试",
	})
}
