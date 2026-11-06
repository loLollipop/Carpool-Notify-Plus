package model

import (
	"net/mail"
	"regexp"
	"strconv"
	"strings"
)

var accountRemarkSerialPattern = regexp.MustCompile(`(?:\(([0-9]+)\)|（([0-9]+)）)\s*$`)

// AccountGroupingEmail identifies the owner shared by Team spaces. Login email
// takes precedence; old imports may instead store an email in name or remark.
func AccountGroupingEmail(account Account) string {
	if email := parseStandaloneEmail(account.Email); email != "" {
		return email
	}
	if email := parseStandaloneEmail(account.Name); email != "" {
		return email
	}
	remark := strings.TrimSpace(account.Remark)
	indices := accountRemarkSerialPattern.FindStringSubmatchIndex(remark)
	if len(indices) == 0 {
		return ""
	}
	raw := ""
	if indices[2] >= 0 {
		raw = remark[indices[2]:indices[3]]
	} else {
		raw = remark[indices[4]:indices[5]]
	}
	serial, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || serial <= 0 || serial > 9007199254740991 {
		return ""
	}
	return parseStandaloneEmail(strings.TrimSpace(remark[:indices[0]]))
}

func parseStandaloneEmail(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" || !strings.Contains(raw, "@") {
		return ""
	}
	address, err := mail.ParseAddress(raw)
	if err != nil || address.Name != "" || strings.TrimSpace(address.Address) != raw {
		return ""
	}
	return raw
}
