package handler

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"net/http"
	"time"

	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
)

const (
	csrfTokenHeader   = "X-CSRF-Token"
	csrfBindingKey    = "csrf_binding"
	csrfBindingLength = 32
	csrfTokenTTL      = 24 * time.Hour
	csrfClockSkew     = time.Minute
	csrfTimestampSize = 8
	csrfMACSize       = sha256.Size
)

type csrfManager struct {
	secret []byte
}

func newCSRFManager(secret string) *csrfManager {
	return &csrfManager{secret: []byte(secret)}
}

func (m *csrfManager) generateBinding() (string, error) {
	binding := make([]byte, csrfBindingLength)
	if _, err := rand.Read(binding); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(binding), nil
}

func (m *csrfManager) generateToken(binding string, issuedAt time.Time) string {
	payload := make([]byte, csrfTimestampSize, csrfTimestampSize+csrfMACSize)
	binary.BigEndian.PutUint64(payload, uint64(issuedAt.UTC().Unix()))
	payload = append(payload, m.sign(binding, payload)...)
	return base64.RawURLEncoding.EncodeToString(payload)
}

func (m *csrfManager) validateToken(binding, token string, now time.Time) bool {
	if m == nil || len(m.secret) == 0 || binding == "" || token == "" {
		return false
	}
	payload, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil || len(payload) != csrfTimestampSize+csrfMACSize {
		return false
	}
	issuedAt := time.Unix(int64(binary.BigEndian.Uint64(payload[:csrfTimestampSize])), 0).UTC()
	age := now.UTC().Sub(issuedAt)
	if age < -csrfClockSkew || age > csrfTokenTTL {
		return false
	}
	expectedMAC := m.sign(binding, payload[:csrfTimestampSize])
	return hmac.Equal(payload[csrfTimestampSize:], expectedMAC)
}

func (m *csrfManager) sign(binding string, timestamp []byte) []byte {
	mac := hmac.New(sha256.New, m.secret)
	_, _ = mac.Write(timestamp)
	_, _ = mac.Write([]byte{0})
	_, _ = mac.Write([]byte(binding))
	return mac.Sum(nil)
}

// CSRFProtection middleware validates CSRF tokens for state-changing operations
func (server *Server) CSRFProtection() gin.HandlerFunc {
	return func(context *gin.Context) {
		// Only protect state-changing methods
		if context.Request.Method == http.MethodGet ||
			context.Request.Method == http.MethodHead ||
			context.Request.Method == http.MethodOptions {
			context.Next()
			return
		}

		session := sessions.Default(context)
		// Get CSRF token from header
		token := context.GetHeader(csrfTokenHeader)
		if token == "" {
			respondError(context, http.StatusForbidden, "Missing CSRF token")
			context.Abort()
			return
		}

		binding, _ := session.Get(csrfBindingKey).(string)
		if server.csrfManager == nil || !server.csrfManager.validateToken(binding, token, time.Now().UTC()) {
			respondError(context, http.StatusForbidden, "Invalid CSRF token")
			context.Abort()
			return
		}

		context.Next()
	}
}

// GetCSRFToken generates and returns a CSRF token for the current session
func (server *Server) getCSRFToken(context *gin.Context) {
	session := sessions.Default(context)
	if server.csrfManager == nil {
		respondError(context, http.StatusInternalServerError, "CSRF protection unavailable")
		return
	}
	if !server.sessionAuthenticated(session) {
		respondError(context, http.StatusUnauthorized, "未登录")
		return
	}

	binding, _ := session.Get(csrfBindingKey).(string)
	token := server.csrfManager.generateToken(binding, time.Now().UTC())
	respondOK(context, gin.H{"csrf_token": token})
}
