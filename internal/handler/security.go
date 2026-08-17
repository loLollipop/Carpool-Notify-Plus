package handler

import (
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

const (
	loginFailureLimit  = 5
	loginBlockDuration = 15 * time.Minute
	loginStateTTL      = time.Hour
	maxLoginStates     = 1024
)

type loginFailureState struct {
	Failures     int
	LastFailure  time.Time
	BlockedUntil time.Time
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
