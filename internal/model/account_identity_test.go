package model

import "testing"

func TestAccountGroupingEmail(t *testing.T) {
	for _, test := range []struct {
		name    string
		account Account
		want    string
	}{
		{"login wins", Account{Email: " Owner@Example.com ", Name: "name@example.com", Remark: "source@example.com(13)"}, "Owner@Example.com"},
		{"name fallback", Account{Name: "name@example.com", Remark: "source@example.com(13)"}, "name@example.com"},
		{"legacy remark", Account{Name: "old import", Remark: " Source@Example.com（47） "}, "Source@Example.com"},
		{"ascii remark", Account{Remark: "source@example.com(13)"}, "source@example.com"},
		{"no email", Account{Name: "space", Remark: "notes(47)"}, ""},
		{"no serial", Account{Remark: "source@example.com"}, ""},
		{"zero serial", Account{Remark: "source@example.com(0)"}, ""},
		{"large serial", Account{Remark: "source@example.com(9007199254740992)"}, ""},
		{"display name not owner", Account{Email: "Owner <owner@example.com>"}, ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := AccountGroupingEmail(test.account); got != test.want {
				t.Fatalf("grouping email = %q, want %q", got, test.want)
			}
		})
	}
}
