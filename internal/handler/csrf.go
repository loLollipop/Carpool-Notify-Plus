package handler

import (
	"crypto/rand"
	"encoding/base64"
	"net/http"
	"sync"
	"time"

	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
)

const (
	csrfTokenHeader = "X-CSRF-Token"
	csrfTokenKey    = "csrf_token"
	csrfTokenLength = 32
	csrfTokenTTL    = 24 * time.Hour
)

type csrfToken struct {
	Token     string
	CreatedAt time.Time
}

type csrfManager struct {
	mu     sync.RWMutex
	tokens map[string]csrfToken
}

func newCSRFManager() *csrfManager {
	manager := &csrfManager{
		tokens: make(map[string]csrfToken),
	}
	// Start cleanup goroutine
	go manager.autoCleanup()
	return manager
}

func (m *csrfManager) generateToken(sessionID string) (string, error) {
	bytes := make([]byte, csrfTokenLength)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	token := base64.URLEncoding.EncodeToString(bytes)

	m.mu.Lock()
	m.tokens[sessionID] = csrfToken{
		Token:     token,
		CreatedAt: time.Now().UTC(),
	}
	m.mu.Unlock()

	return token, nil
}

func (m *csrfManager) validateToken(sessionID, token string) bool {
	if sessionID == "" || token == "" {
		return false
	}

	m.mu.RLock()
	stored, exists := m.tokens[sessionID]
	m.mu.RUnlock()

	if !exists {
		return false
	}

	// Check if token has expired
	if time.Since(stored.CreatedAt) > csrfTokenTTL {
		m.deleteToken(sessionID)
		return false
	}

	return stored.Token == token
}

func (m *csrfManager) deleteToken(sessionID string) {
	m.mu.Lock()
	delete(m.tokens, sessionID)
	m.mu.Unlock()
}

func (m *csrfManager) autoCleanup() {
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	for range ticker.C {
		m.mu.Lock()
		now := time.Now().UTC()
		for sessionID, token := range m.tokens {
			if now.Sub(token.CreatedAt) > csrfTokenTTL {
				delete(m.tokens, sessionID)
			}
		}
		m.mu.Unlock()
	}
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

		// Get session ID
		session := sessions.Default(context)
		sessionID := session.ID()
		if sessionID == "" {
			respondError(context, http.StatusForbidden, "Invalid session")
			context.Abort()
			return
		}

		// Get CSRF token from header
		token := context.GetHeader(csrfTokenHeader)
		if token == "" {
			respondError(context, http.StatusForbidden, "Missing CSRF token")
			context.Abort()
			return
		}

		// Validate token
		if !server.csrfManager.validateToken(sessionID, token) {
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
	sessionID := session.ID()

	token, err := server.csrfManager.generateToken(sessionID)
	if err != nil {
		respondError(context, http.StatusInternalServerError, "Failed to generate CSRF token")
		return
	}

	respondOK(context, gin.H{"csrf_token": token})
}
