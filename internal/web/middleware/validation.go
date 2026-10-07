package middleware

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// MaxRequestBodySize is the maximum allowed request body size (10MB)
const MaxRequestBodySize = 10 << 20

// ValidateContentType ensures JSON requests have proper Content-Type header
func ValidateContentType(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Only validate POST, PUT, PATCH requests with body
		if r.Method == http.MethodPost || r.Method == http.MethodPut || r.Method == http.MethodPatch {
			if r.ContentLength > 0 {
				ct := r.Header.Get("Content-Type")
				if ct == "" || !strings.HasPrefix(ct, "application/json") {
					http.Error(w, `{"ok":false,"error":"Content-Type must be application/json"}`, http.StatusUnsupportedMediaType)
					return
				}
			}
		}
		next.ServeHTTP(w, r)
	})
}

// LimitRequestBody limits the size of request bodies
func LimitRequestBody(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, MaxRequestBodySize)
		next.ServeHTTP(w, r)
	})
}

// ValidateJSON validates that request body is valid JSON
func ValidateJSON(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Only validate requests with JSON body
		if r.Method == http.MethodPost || r.Method == http.MethodPut || r.Method == http.MethodPatch {
			ct := r.Header.Get("Content-Type")
			if strings.HasPrefix(ct, "application/json") && r.ContentLength > 0 {
				// Read body
				body, err := io.ReadAll(r.Body)
				if err != nil {
					http.Error(w, fmt.Sprintf(`{"ok":false,"error":"Failed to read request body: %v"}`, err), http.StatusBadRequest)
					return
				}
				r.Body.Close()

				// Validate JSON
				var js json.RawMessage
				if err := json.Unmarshal(body, &js); err != nil {
					http.Error(w, fmt.Sprintf(`{"ok":false,"error":"Invalid JSON: %v"}`, err), http.StatusBadRequest)
					return
				}

				// Restore body for next handler
				r.Body = io.NopCloser(strings.NewReader(string(body)))
			}
		}
		next.ServeHTTP(w, r)
	})
}
