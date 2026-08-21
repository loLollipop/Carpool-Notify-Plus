package notify

import (
	"html"
	"strings"
)

type customerEmailSummaryItem struct {
	label string
	value string
}

var customerEmailSummaryLabels = map[string]struct{}{
	"客户邮箱":            {},
	"客户微信":            {},
	"原每期价格":           {},
	"调整后每期价格":         {},
	"本期应收":            {},
	"计费周期":            {},
	"到期日期":            {},
	"生效日期":            {},
	"备注":              {},
	"续费链接":            {},
	"链接":              {},
	"Customer email":  {},
	"Customer WeChat": {},
	"Previous price":  {},
	"New price":       {},
	"Amount due":      {},
	"Billing cycle":   {},
	"Due date":        {},
	"Effective date":  {},
	"Note":            {},
	"Renewal link":    {},
}

func parseCustomerEmailSummaryLine(line string) (customerEmailSummaryItem, bool) {
	separator := strings.Index(line, "：")
	separatorWidth := len("：")
	if separator < 0 {
		separator = strings.Index(line, ":")
		separatorWidth = 1
	}
	if separator <= 0 {
		return customerEmailSummaryItem{}, false
	}
	label := strings.TrimSpace(line[:separator])
	if _, exists := customerEmailSummaryLabels[label]; !exists {
		return customerEmailSummaryItem{}, false
	}
	value := strings.TrimSpace(line[separator+separatorWidth:])
	if value == "" {
		return customerEmailSummaryItem{}, false
	}
	return customerEmailSummaryItem{label: label, value: value}, true
}

// BuildCustomerEmailHTML turns the rendered plain-text customer template into
// a compact, email-client-safe HTML layout. The plain text remains the source
// of truth and is also sent as the multipart fallback.
func BuildCustomerEmailHTML(plainText string) string {
	normalized := strings.ReplaceAll(plainText, "\r\n", "\n")
	normalized = strings.ReplaceAll(normalized, "\r", "\n")
	summary := make([]customerEmailSummaryItem, 0, 8)
	content := make([]string, 0, 6)
	priceIncrease := false
	for _, rawLine := range strings.Split(normalized, "\n") {
		line := strings.TrimSpace(rawLine)
		if line == "" {
			continue
		}
		if item, ok := parseCustomerEmailSummaryLine(line); ok {
			summary = append(summary, item)
			if item.label == "原每期价格" || item.label == "调整后每期价格" ||
				item.label == "Previous price" || item.label == "New price" {
				priceIncrease = true
			}
			continue
		}
		content = append(content, line)
	}
	if len(content) == 0 {
		content = append(content, "请留意本次续费安排。")
	}

	cardTitle := "续费信息"
	accent := "#0f766e"
	accentSoft := "#edf8f5"
	accentBorder := "#cfe7e0"
	if priceIncrease {
		cardTitle = "调价摘要"
		accent = "#a16207"
		accentSoft = "#fff8e7"
		accentBorder = "#f0dfb5"
	}

	var builder strings.Builder
	builder.WriteString(`<!doctype html><html lang="zh-CN"><head><meta charset="UTF-8"><meta name="viewport" content="width=device-width,initial-scale=1"></head>`)
	builder.WriteString(`<body style="margin:0;padding:0;background:#f3f7f6;color:#172321;font-family:-apple-system,BlinkMacSystemFont,'Segoe UI','PingFang SC','Microsoft YaHei',sans-serif;">`)
	builder.WriteString(`<div style="display:none;max-height:0;overflow:hidden;opacity:0;">ChatGPT Team 续费提醒</div>`)
	builder.WriteString(`<table role="presentation" width="100%" cellspacing="0" cellpadding="0" border="0" style="background:#f3f7f6;"><tr><td align="center" style="padding:20px 12px;">`)
	builder.WriteString(`<table role="presentation" width="100%" cellspacing="0" cellpadding="0" border="0" style="max-width:580px;background:#ffffff;border:1px solid #dfe9e6;border-radius:18px;box-shadow:0 8px 28px rgba(24,63,55,.08);overflow:hidden;">`)
	builder.WriteString(`<tr><td style="padding:20px 22px 8px;"><div style="font-size:11px;font-weight:800;letter-spacing:1.4px;color:`)
	builder.WriteString(accent)
	builder.WriteString(`;">CARPOOL NOTIFY · CHATGPT TEAM</div></td></tr>`)

	if len(summary) > 0 {
		builder.WriteString(`<tr><td style="padding:8px 22px 0;"><table role="presentation" width="100%" cellspacing="0" cellpadding="0" border="0" style="background:`)
		builder.WriteString(accentSoft)
		builder.WriteString(`;border:1px solid `)
		builder.WriteString(accentBorder)
		builder.WriteString(`;border-radius:14px;"><tr><td style="padding:15px 17px 7px;font-size:12px;font-weight:800;color:`)
		builder.WriteString(accent)
		builder.WriteString(`;letter-spacing:.5px;">`)
		builder.WriteString(html.EscapeString(cardTitle))
		builder.WriteString(`</td></tr><tr><td style="padding:0 17px 12px;"><table role="presentation" width="100%" cellspacing="0" cellpadding="0" border="0">`)
		for index, item := range summary {
			border := ""
			if index > 0 {
				border = "border-top:1px solid rgba(29,78,69,.09);"
			}
			valueColor := "#172321"
			if strings.Contains(item.label, "应收") || strings.Contains(item.label, "价格") ||
				strings.Contains(strings.ToLower(item.label), "price") || strings.Contains(strings.ToLower(item.label), "amount") {
				valueColor = accent
			}
			builder.WriteString(`<tr><td style="` + border + `padding:7px 8px 7px 0;width:38%;font-size:12px;line-height:1.45;color:#6c7b77;vertical-align:top;">`)
			builder.WriteString(html.EscapeString(item.label))
			builder.WriteString(`</td><td style="` + border + `padding:7px 0;font-size:13px;line-height:1.45;font-weight:700;color:`)
			builder.WriteString(valueColor)
			builder.WriteString(`;vertical-align:top;word-break:break-word;">`)
			builder.WriteString(html.EscapeString(item.value))
			builder.WriteString(`</td></tr>`)
		}
		builder.WriteString(`</table></td></tr></table></td></tr>`)
	}

	builder.WriteString(`<tr><td style="padding:18px 22px 20px;">`)
	for index, line := range content {
		fontSize := "14px"
		fontWeight := "400"
		color := "#3e4c49"
		if index == 0 {
			fontSize = "15px"
			fontWeight = "600"
			color = "#172321"
		}
		margin := "0 0 10px"
		if index == len(content)-1 {
			margin = "0"
		}
		builder.WriteString(`<p style="margin:` + margin + `;font-size:` + fontSize + `;line-height:1.75;font-weight:` + fontWeight + `;color:` + color + `;">`)
		builder.WriteString(html.EscapeString(line))
		builder.WriteString(`</p>`)
	}
	builder.WriteString(`</td></tr><tr><td style="padding:12px 22px;background:#f8fbfa;border-top:1px solid #e7efed;font-size:11px;line-height:1.5;color:#83908d;">`)
	builder.WriteString(`本邮件由 Carpool Notify Plus 自动发送，如需协助请联系管理员。`)
	builder.WriteString(`</td></tr></table></td></tr></table></body></html>`)
	return builder.String()
}
