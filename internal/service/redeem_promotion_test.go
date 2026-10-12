package service

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"carpool-notify/internal/db"
	"carpool-notify/internal/model"
)

func newRedeemPromotionTestService(t *testing.T) *SubscriptionService {
	t.Helper()
	fixtureDB, err := db.Open(filepath.Join(t.TempDir(), "promotion.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = fixtureDB.Close() })
	return &SubscriptionService{Store: fixtureDB}
}

func TestRedeemPromotionDefaultsForNewAndLegacySettings(t *testing.T) {
	for _, test := range []struct {
		name string
		raw  string
	}{
		{name: "new installation"},
		{name: "empty persisted settings", raw: " "},
		{name: "legacy settings", raw: `{"announcement_title":"保留原公告","announcement_items":["保留自定义条目"],"support_wechat_id":"original-wechat","payment_qr_data_url":"data:image/png;base64,old"}`},
	} {
		t.Run(test.name, func(t *testing.T) {
			service := newRedeemPromotionTestService(t)
			if test.raw != "" {
				if err := service.Store.SetSetting(model.SettingRedeemPageSettings, test.raw); err != nil {
					t.Fatal(err)
				}
			}
			settings, err := service.GetRedeemPageSettings()
			if err != nil {
				t.Fatal(err)
			}
			if !settings.PromotionEnabled || settings.PromotionBadge != "推广" ||
				settings.PromotionText != "个人自营纯血中转 · 欢迎对接～" ||
				settings.PromotionURL != "https://ikun.fuckoai.bond/" || settings.PromotionCTALabel != "前往体验" {
				t.Fatalf("promotion defaults = %#v", settings)
			}
			if test.name == "legacy settings" && (settings.AnnouncementTitle != "保留原公告" ||
				len(settings.AnnouncementItems) != 1 || settings.AnnouncementItems[0] != "保留自定义条目" ||
				settings.SupportWechatID != "original-wechat" || settings.PaymentQRCodeDataURL != "data:image/png;base64,old") {
				t.Fatalf("legacy fields changed: %#v", settings)
			}
		})
	}
}

func TestRedeemPromotionDisabledAndCustomValuesPersist(t *testing.T) {
	service := newRedeemPromotionTestService(t)
	settings := model.DefaultRedeemPageSettings
	settings.PromotionEnabled = false
	settings.PromotionBadge = "  合作 🤝  "
	settings.PromotionText = "  欢迎咨询 & 对接，保留纯文本～  "
	settings.PromotionURL = "  https://example.com/体验?from=redeem&label=a%20b#合作  "
	settings.PromotionCTALabel = "  了解详情 🚀  "
	settings.PaymentQRCodeDataURL = "data:image/png;base64,iVBORw0KGgo="
	if err := service.SaveRedeemPageSettings(settings); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		stored, err := service.GetRedeemPageSettings()
		if err != nil {
			t.Fatal(err)
		}
		if stored.PromotionEnabled || stored.PromotionBadge != strings.TrimSpace(settings.PromotionBadge) ||
			stored.PromotionText != strings.TrimSpace(settings.PromotionText) ||
			stored.PromotionURL != strings.TrimSpace(settings.PromotionURL) ||
			stored.PromotionCTALabel != strings.TrimSpace(settings.PromotionCTALabel) ||
			stored.PaymentQRCodeDataURL != settings.PaymentQRCodeDataURL {
			t.Fatalf("custom settings not preserved: %#v", stored)
		}
		if err := service.SaveRedeemPageSettings(stored); err != nil {
			t.Fatal(err)
		}
	}
	// A zero-value input, as decoded from older API clients, must not enable ads.
	if err := service.SaveRedeemPageSettings(model.RedeemPageSettings{}); err != nil {
		t.Fatal(err)
	}
	stored, err := service.GetRedeemPageSettings()
	if err != nil {
		t.Fatal(err)
	}
	if stored.PromotionEnabled || stored.PromotionText != "" || stored.PromotionURL != "" ||
		stored.PromotionBadge != "推广" || stored.PromotionCTALabel != "前往体验" {
		t.Fatalf("zero-value promotion normalization = %#v", stored)
	}
	if err := service.Store.SetSetting(model.SettingRedeemPageSettings, `{"promotion_enabled":false}`); err != nil {
		t.Fatal(err)
	}
	stored, err = service.GetRedeemPageSettings()
	if err != nil || stored.PromotionEnabled {
		t.Fatalf("explicit false was lost: enabled=%v err=%v", stored.PromotionEnabled, err)
	}
}

func TestRedeemPromotionValidation(t *testing.T) {
	for _, test := range []struct {
		name   string
		change func(*model.RedeemPageSettings)
		want   string
	}{
		{"missing text", func(s *model.RedeemPageSettings) { s.PromotionText = " \n " }, "启用推广时请填写推广文案"},
		{"missing URL", func(s *model.RedeemPageSettings) { s.PromotionURL = " " }, "启用推广时请填写推广链接"},
		{"badge length", func(s *model.RedeemPageSettings) { s.PromotionBadge = strings.Repeat("推", 9) }, "推广标记最多 8 个字"},
		{"text length", func(s *model.RedeemPageSettings) { s.PromotionText = strings.Repeat("🚀", 121) }, "推广文案最多 120 个字"},
		{"CTA length", func(s *model.RedeemPageSettings) { s.PromotionCTALabel = strings.Repeat("往", 13) }, "推广按钮文字最多 12 个字"},
		{"URL byte length", func(s *model.RedeemPageSettings) {
			s.PromotionURL = "https://example.com/" + strings.Repeat("文", 677)
		}, "推广链接最多 2048 字节"},
		{"badge HTML", func(s *model.RedeemPageSettings) { s.PromotionBadge = "<b>推广" }, "推广标记仅支持纯文本"},
		{"text HTML", func(s *model.RedeemPageSettings) { s.PromotionText = "<script>alert(1)</script>" }, "推广文案仅支持纯文本"},
		{"CTA angle brackets", func(s *model.RedeemPageSettings) { s.PromotionCTALabel = "继续 >" }, "推广按钮文字仅支持纯文本"},
	} {
		t.Run(test.name, func(t *testing.T) {
			settings := model.DefaultRedeemPageSettings
			test.change(&settings)
			_, err := normalizeRedeemPageSettings(settings)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want %q", err, test.want)
			}
		})
	}
	settings := model.DefaultRedeemPageSettings
	settings.PromotionBadge = strings.Repeat("推", 8)
	settings.PromotionText = strings.Repeat("🚀", 120)
	settings.PromotionCTALabel = strings.Repeat("往", 12)
	settings.PromotionURL = "https://example.com/" + strings.Repeat("a", 2048-len("https://example.com/"))
	if _, err := normalizeRedeemPageSettings(settings); err != nil {
		t.Fatalf("exact field limits rejected: %v", err)
	}
	settings.PromotionEnabled = false
	settings.PromotionText = ""
	settings.PromotionURL = ""
	settings.PromotionBadge = " "
	settings.PromotionCTALabel = " "
	normalized, err := normalizeRedeemPageSettings(settings)
	if err != nil || normalized.PromotionBadge != "推广" || normalized.PromotionCTALabel != "前往体验" {
		t.Fatalf("disabled empty fields = %#v, err = %v", normalized, err)
	}
}

func TestRedeemPromotionURLValidation(t *testing.T) {
	for _, value := range []string{
		"javascript:alert(1)", "data:text/html,<script>", "ftp://example.com/", "//example.com/", "/local",
		"https:example.com", "https:///path", "https://", "https://?query=1", "https://#fragment",
		"https://user:password@example.com/", "https://user@example.com/", "https://example.com@evil.example/",
		"https://example.com\\@evil.example/", "https://example.com/path\\test", "https://example.com/path with space",
		"https://example.com/path\nnext", "https://example.com/path\tnext", "https://example.com/path\x00",
		"https://example.com/path\u00a0next", "https://example.com/<tag>", "https://example.com:bad/",
		"https://example.com:/", "https://example.com:0/", "https://example.com:65536/", "https://example.com:-1/",
		"https://./", "https://..example.com/", "https://example..com/", "https://-example.com/",
		"https://example-.com/", "https://example_com/", "https://[example.com]/", "https://[::1", "https://::1/",
		"https://example.com/%zz", "https://%65xample.com/", "https://[not-an-ip]/",
	} {
		t.Run(value, func(t *testing.T) {
			for _, enabled := range []bool{true, false} {
				settings := model.DefaultRedeemPageSettings
				settings.PromotionEnabled = enabled
				settings.PromotionURL = value
				_, err := normalizeRedeemPageSettings(settings)
				if err == nil || !strings.Contains(err.Error(), "推广链接") {
					t.Fatalf("URL %q enabled=%v accepted, err=%v", value, enabled, err)
				}
			}
		})
	}
	for _, value := range []string{
		"https://ikun.fuckoai.bond/", "http://example.com/", "https://example.com:65535/path",
		"https://example.com/体验?from=redeem&name=a%20b#详情", "https://[2001:db8::1]:443/path?q=1#top",
		"https://127.0.0.1/", "https://例子.中国/体验", "https://localhost/path",
	} {
		t.Run(value, func(t *testing.T) {
			settings := model.DefaultRedeemPageSettings
			settings.PromotionURL = "  " + value + "  "
			normalized, err := normalizeRedeemPageSettings(settings)
			if err != nil || normalized.PromotionURL != value {
				t.Fatalf("valid URL %q = %q, err=%v", value, normalized.PromotionURL, err)
			}
		})
	}
}

func TestRedeemPromotionRemovedTabGuidanceUpgrade(t *testing.T) {
	if strings.Contains(model.DefaultRedeemRenewalGuidance, "自助续费") ||
		!strings.Contains(model.DefaultRedeemRenewalGuidance, "联系管理员") {
		t.Fatalf("default guidance = %q", model.DefaultRedeemRenewalGuidance)
	}
	for _, legacy := range []string{
		"到期后若未及时续费，账号可能会被移出 Team；未备份的工作空间内容可能无法找回。",
		"到期后如果没有及时续费，席位可能会被移出空间；移出前未备份的工作空间内容可能无法找回。",
		"兑换成功后，可在本页切换到“自助续费”，也可联系客服协助续费；到期仍未续费的席位将自动移出空间。",
	} {
		service := newRedeemPromotionTestService(t)
		custom := "自定义续费说明：请在到期前联系管理员。"
		encoded, err := json.Marshal(model.RedeemPageSettings{AnnouncementItems: []string{" " + legacy + " ", custom}})
		if err != nil {
			t.Fatal(err)
		}
		if err := service.Store.SetSetting(model.SettingRedeemPageSettings, string(encoded)); err != nil {
			t.Fatal(err)
		}
		settings, err := service.GetRedeemPageSettings()
		if err != nil {
			t.Fatal(err)
		}
		if len(settings.AnnouncementItems) != 2 || settings.AnnouncementItems[0] != model.DefaultRedeemRenewalGuidance ||
			settings.AnnouncementItems[1] != custom {
			t.Fatalf("guidance upgrade = %#v", settings.AnnouncementItems)
		}
	}
}
