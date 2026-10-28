package notify

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

type iyuuTransportFunc func(*http.Request) (*http.Response, error)

func (transport iyuuTransportFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return transport(request)
}

func TestIYUUSenderRequiresExplicitSuccessfulResponse(t *testing.T) {
	for _, test := range []struct {
		name, body string
		ok         bool
	}{
		{"success", `{"errcode":0,"errmsg":"ok"}`, true},
		{"rejected", `{"errcode":400,"errmsg":"invalid token"}`, false},
		{"empty", "", false},
		{"html", "<html>proxy error</html>", false},
		{"missing status", `{"message":"ok"}`, false},
		{"null status", `{"errcode":null}`, false},
		{"oversized", `{"errcode":0,"errmsg":"` + strings.Repeat("x", 4096) + `"}`, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			sender := IYUUSender{Token: "test-token", Client: &http.Client{
				Transport: iyuuTransportFunc(func(*http.Request) (*http.Response, error) {
					return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(test.body))}, nil
				}),
			}}
			if err := sender.Send(context.Background(), "title", "body"); (err == nil) != test.ok {
				t.Fatalf("Send() = %v, want success %v", err, test.ok)
			}
		})
	}
}
