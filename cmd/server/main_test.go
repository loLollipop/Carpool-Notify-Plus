package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestConfigureTrustedProxiesClientIP(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, test := range []struct {
		name      string
		proxies   []string
		remote    string
		forwarded string
		realIP    string
		want      string
	}{
		{name: "untrusted peer cannot spoof", proxies: []string{"127.0.0.1", "::1"}, remote: "192.0.2.10:1234", forwarded: "198.51.100.1", realIP: "198.51.100.2", want: "192.0.2.10"},
		{name: "loopback forwards client", proxies: []string{"127.0.0.1", "::1"}, remote: "127.0.0.1:1234", forwarded: "198.51.100.1", want: "198.51.100.1"},
		{name: "IPv6 loopback forwards client", proxies: []string{"127.0.0.1", "::1"}, remote: "[::1]:1234", forwarded: "2001:db8::1", want: "2001:db8::1"},
		{name: "explicit disable", proxies: []string{}, remote: "127.0.0.1:1234", forwarded: "198.51.100.1", realIP: "198.51.100.2", want: "127.0.0.1"},
		{name: "nil disable", remote: "127.0.0.1:1234", forwarded: "198.51.100.1", want: "127.0.0.1"},
		{name: "missing forwarded ignores real IP", proxies: []string{"127.0.0.1"}, remote: "127.0.0.1:1234", realIP: "198.51.100.1", want: "127.0.0.1"},
		{name: "invalid forwarded ignores real IP", proxies: []string{"127.0.0.1"}, remote: "127.0.0.1:1234", forwarded: "invalid", realIP: "198.51.100.1", want: "127.0.0.1"},
		{name: "trusted CIDR follows chain", proxies: []string{"10.0.0.0/8"}, remote: "10.0.0.1:1234", forwarded: "198.51.100.1, 10.0.0.2", want: "198.51.100.1"},
		{name: "chain stops at untrusted peer", proxies: []string{"10.0.0.0/8"}, remote: "10.0.0.1:1234", forwarded: "198.51.100.1, 192.0.2.1", want: "192.0.2.1"},
	} {
		t.Run(test.name, func(t *testing.T) {
			router := gin.New()
			if err := configureTrustedProxies(router, test.proxies); err != nil {
				t.Fatal(err)
			}
			router.GET("/", func(c *gin.Context) { c.String(http.StatusOK, c.ClientIP()) })
			request := httptest.NewRequest(http.MethodGet, "/", nil)
			request.RemoteAddr = test.remote
			request.Header.Set("X-Forwarded-For", test.forwarded)
			request.Header.Set("X-Real-IP", test.realIP)
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Body.String() != test.want {
				t.Fatalf("client IP = %q, want %q", response.Body.String(), test.want)
			}
		})
	}
}
