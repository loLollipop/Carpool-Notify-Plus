package handler

import (
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

const (
	loginFailureLimit   = 5
	loginBlockDuration  = 15 * time.Minute
	loginStateTTL       = time.Hour
	maxLoginStates      = 1024
	publicSubmitLimit   = 8
	publicSubmitWindow  = 10 * time.Minute
	publicStatusLimit   = 180
	publicStatusWindow  = time.Minute
	maxPublicRateStates = 4096
)

type loginFailureState struct {
	Failures     int
	LastFailure  time.Time
	BlockedUntil time.Time
}

type fixedWindowState struct {
	StartedAt time.Time
	Count     int
}

type fixedWindowLimiter struct {
	mu       sync.Mutex
	limit    int
	window   time.Duration
	maxKeys  int
	byClient map[string]fixedWindowState
}

func newFixedWindowLimiter(limit int, window time.Duration) *fixedWindowLimiter {
	return &fixedWindowLimiter{
		limit:    limit,
		window:   window,
		maxKeys:  maxPublicRateStates,
		byClient: make(map[string]fixedWindowState),
	}
}

func (limiter *fixedWindowLimiter) allow(clientKey string, now time.Time) (bool, time.Duration) {
	if limiter == nil || limiter.limit <= 0 || limiter.window <= 0 {
		return true, 0
	}
	limiter.mu.Lock()
	defer limiter.mu.Unlock()
	state, exists := limiter.byClient[clientKey]
	if !exists || now.Sub(state.StartedAt) >= limiter.window || now.Before(state.StartedAt) {
		if !exists && len(limiter.byClient) >= limiter.maxKeys {
			limiter.prune(now)
		}
		if !exists && len(limiter.byClient) >= limiter.maxKeys {
			limiter.removeOldest()
		}
		limiter.byClient[clientKey] = fixedWindowState{StartedAt: now, Count: 1}
		return true, 0
	}
	if state.Count >= limiter.limit {
		return false, state.StartedAt.Add(limiter.window).Sub(now)
	}
	state.Count++
	limiter.byClient[clientKey] = state
	return true, 0
}

func (limiter *fixedWindowLimiter) prune(now time.Time) {
	for clientKey, state := range limiter.byClient {
		if now.Sub(state.StartedAt) >= limiter.window || now.Before(state.StartedAt) {
			delete(limiter.byClient, clientKey)
		}
	}
}

func (limiter *fixedWindowLimiter) removeOldest() {
	oldestKey := ""
	var oldestStartedAt time.Time
	for clientKey, state := range limiter.byClient {
		if oldestKey == "" || state.StartedAt.Before(oldestStartedAt) {
			oldestKey = clientKey
			oldestStartedAt = state.StartedAt
		}
	}
	if oldestKey != "" {
		delete(limiter.byClient, oldestKey)
	}
}

func (server *Server) ensurePublicLimiters() {
	if server.publicSubmitLimiter == nil {
		server.publicSubmitLimiter = newFixedWindowLimiter(publicSubmitLimit, publicSubmitWindow)
	}
	if server.publicStatusLimiter == nil {
		server.publicStatusLimiter = newFixedWindowLimiter(publicStatusLimit, publicStatusWindow)
	}
}

func (server *Server) limitPublicRequests(limiter *fixedWindowLimiter) gin.HandlerFunc {
	return func(context *gin.Context) {
		allowed, retryAfter := limiter.allow(context.ClientIP(), time.Now().UTC())
		if allowed {
			context.Next()
			return
		}
		seconds := int64(retryAfter / time.Second)
		if retryAfter%time.Second != 0 {
			seconds++
		}
		if seconds < 1 {
			seconds = 1
		}
		context.Header("Retry-After", strconv.FormatInt(seconds, 10))
		context.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
			"ok":    false,
			"error": "请求过于频繁，请稍后再试",
		})
	}
}

// SecurityHeaders applies browser hardening globally and disables caching for
// authenticated/public API responses that may contain private business data.
func SecurityHeaders() gin.HandlerFunc {
	return func(context *gin.Context) {
		context.Header("X-Content-Type-Options", "nosniff")
		context.Header("X-Frame-Options", "DENY")
		context.Header("Referrer-Policy", "same-origin")
		context.Header("Content-Security-Policy", "frame-ancestors 'none'")
		context.Header("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		if strings.HasPrefix(context.Request.URL.Path, "/api/") || context.Request.URL.Path == "/export" {
			context.Header("Cache-Control", "no-store")
			context.Header("Pragma", "no-cache")
		}
		context.Next()
	}
}

// LimitRequestBody prevents oversized public or authenticated request bodies
// from consuming the small server's memory. Individual handlers may impose a
// smaller limit for their own payloads.
func LimitRequestBody(maxBytes int64) gin.HandlerFunc {
	return func(context *gin.Context) {
		if context.Request.Body == nil {
			context.Next()
			return
		}
		if context.Request.ContentLength > maxBytes {
			context.AbortWithStatusJSON(http.StatusRequestEntityTooLarge, gin.H{
				"ok":    false,
				"error": "请求内容过大",
			})
			return
		}
		context.Request.Body = http.MaxBytesReader(
			context.Writer,
			context.Request.Body,
			maxBytes,
		)
		context.Next()
	}
}

// RequestLogger retains useful access logs without writing redemption bearer
// tokens or sandbox access tokens to journald.
func RequestLogger() gin.HandlerFunc {
	return gin.LoggerWithFormatter(func(parameters gin.LogFormatterParams) string {
		return fmt.Sprintf(
			"[GIN] %s | %3d | %13v | %15s | %-7s %q%s\n",
			parameters.TimeStamp.Format("2006/01/02 - 15:04:05"),
			parameters.StatusCode,
			parameters.Latency,
			parameters.ClientIP,
			parameters.Method,
			redactedRequestTarget(parameters.Request),
			parameters.ErrorMessage,
		)
	})
}

func redactedRequestTarget(request *http.Request) string {
	if request == nil || request.URL == nil {
		return ""
	}
	path := request.URL.Path
	for _, prefix := range []string{"/api/redeem/", "/api/sandbox/redeem/"} {
		if strings.HasPrefix(path, prefix) && strings.TrimPrefix(path, prefix) != "" {
			path = prefix + ":token"
			break
		}
	}
	query, err := url.ParseQuery(request.URL.RawQuery)
	if err != nil {
		return path
	}
	for _, key := range []string{"sandbox_token", "token", "password", "secret"} {
		if query.Has(key) {
			query.Set(key, "REDACTED")
		}
	}
	if encoded := query.Encode(); encoded != "" {
		return path + "?" + encoded
	}
	return path
}

func (server *Server) loginRetryAfter(clientIP string, now time.Time) time.Duration {
	server.loginMu.Lock()
	defer server.loginMu.Unlock()
	state, exists := server.loginFailures[clientIP]
	if !exists || state.BlockedUntil.IsZero() || !now.Before(state.BlockedUntil) {
		if exists && (!state.BlockedUntil.IsZero() || now.Sub(state.LastFailure) >= loginStateTTL) {
			delete(server.loginFailures, clientIP)
		}
		return 0
	}
	return state.BlockedUntil.Sub(now)
}

func (server *Server) recordLoginFailure(clientIP string, now time.Time) {
	server.loginMu.Lock()
	defer server.loginMu.Unlock()
	if server.loginFailures == nil {
		server.loginFailures = make(map[string]loginFailureState)
	}
	if _, exists := server.loginFailures[clientIP]; !exists && len(server.loginFailures) >= maxLoginStates {
		for address, state := range server.loginFailures {
			if now.Sub(state.LastFailure) >= loginStateTTL && !now.Before(state.BlockedUntil) {
				delete(server.loginFailures, address)
			}
		}
		if len(server.loginFailures) >= maxLoginStates {
			oldestAddress := ""
			var oldestFailure time.Time
			for address, state := range server.loginFailures {
				if oldestAddress == "" || state.LastFailure.Before(oldestFailure) {
					oldestAddress = address
					oldestFailure = state.LastFailure
				}
			}
			if oldestAddress != "" {
				delete(server.loginFailures, oldestAddress)
			}
		}
	}
	state := server.loginFailures[clientIP]
	if now.Sub(state.LastFailure) >= loginStateTTL {
		state = loginFailureState{}
	}
	state.Failures++
	state.LastFailure = now
	if state.Failures >= loginFailureLimit {
		state.BlockedUntil = now.Add(loginBlockDuration)
	}
	server.loginFailures[clientIP] = state
}

func (server *Server) clearLoginFailures(clientIP string) {
	server.loginMu.Lock()
	delete(server.loginFailures, clientIP)
	server.loginMu.Unlock()
}

func respondLoginRateLimited(context *gin.Context, retryAfter time.Duration) {
	seconds := int64(retryAfter / time.Second)
	if retryAfter%time.Second != 0 {
		seconds++
	}
	if seconds < 1 {
		seconds = 1
	}
	context.Header("Retry-After", strconv.FormatInt(seconds, 10))
	respondError(context, http.StatusTooManyRequests, "登录失败次数过多，请稍后再试")
}
