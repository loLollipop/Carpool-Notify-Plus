package service

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"carpool-notify/internal/model"
)

type radarTestClient func(*http.Request) (*http.Response, error)

func (client radarTestClient) Do(request *http.Request) (*http.Response, error) {
	return client(request)
}
func radarResponse(status int, value any) *http.Response {
	bytes, _ := json.Marshal(value)
	return &http.Response{StatusCode: status, Header: http.Header{"Etag": {`"radar-1"`}, "Last-Modified": {"Tue, 03 Nov 2026 03:50:00 GMT"}}, Body: io.NopCloser(strings.NewReader(string(bytes)))}
}
func radarFixture(now time.Time) (radarEnvelope, radarEnvelope) {
	stale := false
	pointer := radarEnvelope{SchemaVersion: "price-radar.v1", SnapshotID: "20261103035029-87ed3b124055", GeneratedAt: now.Add(-5 * time.Minute), PublishedAt: now.Add(-4 * time.Minute), Stale: &stale, RankingPolicy: "available-non-shared-first.v1"}
	pointer.SnapshotURL = "https://data.priceai.cc/v1/snapshots/" + pointer.SnapshotID + ".json"
	snapshot := pointer
	product := radarProduct{ID: "chatgpt-team-business", Slug: "chatgpt-team-business", Platform: "ChatGPT", GeneratedAt: now.Add(-6 * time.Minute)}
	for index := 0; index < 3; index++ {
		product.TopOffers = append(product.TopOffers, radarOffer{ID: fmt.Sprint("new-", index), SourceID: fmt.Sprint("shop-", index), Title: "ChatGPT Team 单席位月租新购", Price: 110 + float64(index*10), Currency: "CNY", Status: "in_stock", EffectiveStatus: "available", FreshnessStatus: "fresh", CapturedAt: now.Add(-10 * time.Minute), LastSeenAt: now.Add(-8 * time.Minute), VerifiedAt: now.Add(-8 * time.Minute), ExpiresAt: now.Add(24 * time.Hour)})
	}
	// Presets have their own generation time, as specified by the public schema.
	renewal := radarPreset{ID: "team_renewal", GeneratedAt: now.Add(-5 * time.Minute)}
	for index := 0; index < 3; index++ {
		offer := product.TopOffers[index]
		offer.ID = fmt.Sprint("renew-", index)
		offer.Title = "ChatGPT Business 单席位月租续费码"
		offer.Price += 10
		renewal.TopOffers = append(renewal.TopOffers, offer)
	}
	product.Presets = []radarPreset{renewal}
	snapshot.Products = []radarProduct{product}
	return pointer, snapshot
}
func radarService(t *testing.T) (*SubscriptionService, *time.Time) {
	service := openGoalTestService(t)
	service.MarketPriceURL = ""
	now := time.Date(2026, 11, 3, 4, 0, 0, 0, time.UTC)
	service.Clock = func() time.Time { return now }
	return service, &now
}
func TestRadarConditionalPollingAndImmutableSnapshot(t *testing.T) {
	service, now := radarService(t)
	pointer, snapshot := radarFixture(*now)
	pointerCalls, snapshotCalls := 0, 0
	service.MarketClient = radarTestClient(func(request *http.Request) (*http.Response, error) {
		if request.Header.Get("User-Agent") != "CarpoolNotify/1.0 market-monitor" {
			t.Fatal("monitor identity changed")
		}
		if _, ok := request.Context().Deadline(); !ok {
			t.Fatal("request has no timeout")
		}
		switch request.URL.String() {
		case defaultMarketPriceURL:
			pointerCalls++
			if pointerCalls > 1 && (request.Header.Get("If-None-Match") != `"radar-1"` || request.Header.Get("If-Modified-Since") == "") {
				t.Fatal("conditional cache validators missing")
			}
			if pointerCalls == 2 {
				return radarResponse(http.StatusNotModified, nil), nil
			}
			return radarResponse(http.StatusOK, pointer), nil
		case pointer.SnapshotURL:
			snapshotCalls++
			return radarResponse(http.StatusOK, snapshot), nil
		default:
			t.Fatalf("untrusted request %s", request.URL)
			return nil, nil
		}
	})
	view, err := service.RefreshMarketPrice()
	if err != nil || !view.AcquisitionUsable || !view.RenewalUsable || view.Stale || view.AcquisitionSnapshot.SampleCount != 3 || view.RenewalSnapshot.MedianPriceCents != 13000 {
		t.Fatalf("initial Radar view = %#v, %v", view, err)
	}
	collectedAt := view.Snapshot.CreatedAt
	for i := 0; i < 5; i++ {
		if _, err = service.RefreshMarketPrice(); err != nil {
			t.Fatal(err)
		}
	}
	if pointerCalls != 1 || snapshotCalls != 1 {
		t.Fatalf("one-minute budget exceeded: %d/%d", pointerCalls, snapshotCalls)
	}
	*now = now.Add(time.Minute)
	view, err = service.RefreshMarketPrice()
	if err != nil || !view.RenewalUsable || !view.Snapshot.CreatedAt.Equal(collectedAt) {
		t.Fatalf("304 view = %#v, %v", view, err)
	}
	*now = now.Add(time.Minute)
	if _, err = service.RefreshMarketPrice(); err != nil {
		t.Fatal(err)
	}
	if pointerCalls != 3 || snapshotCalls != 1 {
		t.Fatalf("same generation downloaded again: %d/%d", pointerCalls, snapshotCalls)
	}
	history, err := service.Store.ListMarketPriceSnapshots(marketRadarProvider, marketProduct, 10)
	if err != nil || len(history) != 1 {
		t.Fatalf("immutable history = %#v, %v", history, err)
	}
	*now = now.Add(3 * time.Hour)
	view, err = service.RefreshMarketPrice()
	if err != nil || view.RenewalUsable || !view.Stale || view.Warning == "" || !view.Snapshot.CreatedAt.Equal(collectedAt) {
		t.Fatalf("old source treated as fresh = %#v, %v", view, err)
	}
}
func TestRadarRejectsUntrustedOrInvalidSources(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*radarEnvelope, *radarEnvelope)
	}{
		{"foreign host", func(p, s *radarEnvelope) { p.SnapshotURL = "https://evil.invalid/a.json" }},
		{"credentials", func(p, s *radarEnvelope) {
			p.SnapshotURL = "https://user@data.priceai.cc/v1/snapshots/" + p.SnapshotID + ".json"
		}},
		{"unexpected query", func(p, s *radarEnvelope) { p.SnapshotURL += "?next=evil" }},
		{"path traversal id", func(p, s *radarEnvelope) { p.SnapshotID = "../../private" }},
		{"schema", func(p, s *radarEnvelope) { p.SchemaVersion = "other" }},
		{"stale flag", func(p, s *radarEnvelope) { yes := true; p.Stale = &yes }},
		{"missing stale", func(p, s *radarEnvelope) { p.Stale = nil }},
		{"old source freshly published", func(p, s *radarEnvelope) { p.GeneratedAt = p.GeneratedAt.Add(-24 * time.Hour) }},
		{"future source", func(p, s *radarEnvelope) { p.GeneratedAt = p.GeneratedAt.Add(time.Hour) }},
		{"snapshot mismatch", func(p, s *radarEnvelope) { s.SnapshotID = "20261103035029-badbad" }},
		{"wrong product", func(p, s *radarEnvelope) { s.Products[0].Slug = "claude" }},
		{"old product", func(p, s *radarEnvelope) { s.Products[0].GeneratedAt = s.GeneratedAt.Add(-24 * time.Hour) }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			service, now := radarService(t)
			pointer, snapshot := radarFixture(*now)
			test.mutate(&pointer, &snapshot)
			calls := 0
			service.MarketClient = radarTestClient(func(r *http.Request) (*http.Response, error) {
				calls++
				if r.URL.String() == defaultMarketPriceURL {
					return radarResponse(200, pointer), nil
				}
				if r.URL.Host != "data.priceai.cc" {
					t.Fatalf("untrusted host %s", r.URL)
				}
				return radarResponse(200, snapshot), nil
			})
			view, err := service.RefreshMarketPrice()
			if err != nil || view.AcquisitionUsable || view.RenewalUsable || view.Available || view.Warning == "" {
				t.Fatalf("invalid source accepted: %#v, %v", view, err)
			}
			if strings.Contains(test.name, "host") && calls != 1 {
				t.Fatalf("followed untrusted pointer: %d", calls)
			}
		})
	}
}
func TestRadarFailureBackoffAndGoalRenderingDoNotFetch(t *testing.T) {
	service, now := radarService(t)
	var calls atomic.Int32
	service.MarketClient = radarTestClient(func(r *http.Request) (*http.Response, error) {
		calls.Add(1)
		return radarResponse(http.StatusTooManyRequests, nil), nil
	})
	view, err := service.RefreshMarketPrice()
	if err != nil || view.WarningCode != "provider_unavailable" {
		t.Fatalf("failure state=%#v, %v", view, err)
	}
	for i := 0; i < 4; i++ {
		if _, err = service.GetGoalCenter(); err != nil {
			t.Fatal(err)
		}
		if _, err = service.RefreshMarketPrice(); err != nil {
			t.Fatal(err)
		}
	}
	*now = now.Add(time.Minute)
	if _, err = service.RefreshMarketPrice(); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 1 {
		t.Fatalf("failure retried before backoff: %d", calls.Load())
	}
	*now = now.Add(time.Minute)
	if _, err = service.RefreshMarketPrice(); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 2 {
		t.Fatalf("backoff did not retry: %d", calls.Load())
	}
	// A running refresh holds only the refresh lock; rendering still returns DB state.
	service.marketRefreshMu.Lock()
	done := make(chan error, 1)
	go func() { _, err := service.GetGoalCenter(); done <- err }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		service.marketRefreshMu.Unlock()
		t.Fatal("goal rendering blocked on network refresh")
	}
	service.marketRefreshMu.Unlock()
}
func TestRadarComparableMonthlyFiltersAndIndependentSegments(t *testing.T) {
	service, now := radarService(t)
	pointer, snapshot := radarFixture(*now)
	for index := range snapshot.Products[0].Presets[0].TopOffers {
		snapshot.Products[0].Presets[0].TopOffers[index].Title = "ChatGPT Team 席位周抛，非出租席位"
	}
	// Repeated listings across default rankings and presets do not inflate count.
	snapshot.Products[0].Presets = append(snapshot.Products[0].Presets, radarPreset{ID: "team_official", GeneratedAt: *now, TopOffers: snapshot.Products[0].TopOffers})
	service.MarketClient = radarTestClient(func(r *http.Request) (*http.Response, error) {
		if r.URL.String() == defaultMarketPriceURL {
			return radarResponse(200, pointer), nil
		}
		return radarResponse(200, snapshot), nil
	})
	view, err := service.RefreshMarketPrice()
	if err != nil || !view.AcquisitionUsable || view.RenewalUsable || view.AcquisitionSnapshot.SampleCount != 3 || view.RenewalSnapshot != nil || view.WarningCode != "insufficient_samples" {
		t.Fatalf("split result=%#v, %v", view, err)
	}
	good := snapshot.Products[0].TopOffers[0]
	for _, title := range []string{"ChatGPT Team 标准席位周抛，30天售后", "ChatGPT Team 非出租月租席位", "ChatGPT Team 月租成品号", "ChatGPT Team 2席月租", "ChatGPT Team 月租5x", "ChatGPT Team 年租月卡", "ChatGPT Team 月卡7天", "ChatGPT Team 席位", "Claude 月租席位"} {
		offer := good
		offer.Title = title
		legacy := priceAIOffer{SourceTitle: title, Price: offer.Price, Currency: offer.Currency, Status: offer.Status, EffectiveStatus: offer.EffectiveStatus}
		if radarMonthlyOffer(offer, *now) && isComparableMarketOffer(legacy) {
			t.Errorf("bad monthly rental accepted: %s", title)
		}
	}
	for _, mutate := range []func(*radarOffer){func(o *radarOffer) { o.ExpiresAt = *now }, func(o *radarOffer) { o.CapturedAt = now.Add(-3 * time.Hour) }, func(o *radarOffer) { o.FreshnessStatus = "stale" }, func(o *radarOffer) { o.VerifiedAt = time.Time{} }, func(o *radarOffer) { n := 2; o.MinimumOrder = &n }} {
		offer := good
		mutate(&offer)
		if radarMonthlyOffer(offer, *now) {
			t.Errorf("expired/stale/unverified/wholesale offer accepted: %#v", offer)
		}
	}
}
func TestRadarRealPublicTopFiveCannotStandInForMonthlyMarket(t *testing.T) {
	service, now := radarService(t)
	pointer, snapshot := radarFixture(*now)
	snapshot.Products[0].Presets = nil
	snapshot.Products[0].TopOffers = nil
	for index, title := range []string{"长效车 | 不踢不拉，直到空间死 | 1x team 不保证周限额【非出租席位】", "长效车 | 不踢不拉，直到空间死 | 1x team 不保证周限额【非出租席位】", "长效车 | 不踢不拉，直到空间死 | 1x team 不保证周限额【非出租席位】", "【Team 1x】标准席位周抛成品号，活5天左右（独享 Plus 同等额度）", "【Team 1x】标准席位周抛成品号，活5天左右（独享 Plus 同等额度）"} {
		snapshot.Products[0].TopOffers = append(snapshot.Products[0].TopOffers, radarOffer{ID: fmt.Sprint(index), Title: title, Price: 13.39 + float64(index), Currency: "CNY", Status: "in_stock", EffectiveStatus: "available", FreshnessStatus: "fresh", CapturedAt: *now, LastSeenAt: *now, VerifiedAt: *now, ExpiresAt: now.Add(24 * time.Hour)})
	}
	service.MarketClient = radarTestClient(func(r *http.Request) (*http.Response, error) {
		if r.URL.String() == defaultMarketPriceURL {
			return radarResponse(200, pointer), nil
		}
		return radarResponse(200, snapshot), nil
	})
	view, err := service.RefreshMarketPrice()
	if err != nil || view.Available || view.AcquisitionUsable || view.RenewalUsable || view.WarningCode != "insufficient_samples" || view.Snapshot != nil {
		t.Fatalf("unrepresentative top five became market: %#v, %v", view, err)
	}
	rows, _ := service.Store.ListMarketPriceSnapshots(marketRadarProvider, marketProduct, 5)
	if len(rows) != 0 {
		t.Fatalf("fabricated samples persisted: %#v", rows)
	}
}
func TestMarketOldSourceOrLegacyCacheCannotRecommendRaise(t *testing.T) {
	service, now := radarService(t)
	seedPricingSeats(t, service, 10, 9, 9000)
	for _, provider := range []string{marketProvider, marketRadarProvider} {
		snapshot := model.MarketPriceSnapshot{Provider: provider, Product: marketProduct, LowPriceCents: 13000, MedianPriceCents: 14000, HighPriceCents: 15000, SampleCount: 10, SourceUpdatedAt: now.Add(-48 * time.Hour), CreatedAt: *now}
		if _, err := service.Store.InsertMarketPriceSnapshot(snapshot); err != nil {
			t.Fatal(err)
		}
	}
	center, err := service.GetGoalCenter()
	if err != nil || center.Market.RenewalUsable || !center.Market.Stale || center.Pricing.Action == "raise" || center.Pricing.Basis != "customers_only" {
		t.Fatalf("stale source used=%#v, %v", center, err)
	}
	for _, candidate := range center.Candidates {
		if candidate.Recommended || candidate.MarketPosition != "unavailable" {
			t.Fatalf("stale automatic candidate=%#v", candidate)
		}
	}
	if _, err := service.ExemptBulkPricing(BulkPricingExemptionInput{SubscriptionIDs: []int64{1}, ReviewCycles: 1, ReasonCode: "manual"}); err == nil {
		t.Fatal("exemption used expired market")
	}
}
func TestRadarOfferExpirySurvivesServiceRestart(t *testing.T) {
	service, now := radarService(t)
	pointer, snapshot := radarFixture(*now)
	for index := range snapshot.Products[0].TopOffers {
		snapshot.Products[0].TopOffers[index].ExpiresAt = now.Add(20 * time.Minute)
	}
	service.MarketClient = radarTestClient(func(r *http.Request) (*http.Response, error) {
		if r.URL.String() == defaultMarketPriceURL {
			return radarResponse(200, pointer), nil
		}
		return radarResponse(200, snapshot), nil
	})
	view, err := service.RefreshMarketPrice()
	if err != nil || !view.AcquisitionUsable {
		t.Fatalf("initial expiring offer=%#v, %v", view, err)
	}
	*now = now.Add(21 * time.Minute)
	restarted := &SubscriptionService{Store: service.Store, Clock: service.Clock}
	view, err = restarted.cachedMarketPrice()
	if err != nil || view.AcquisitionUsable {
		t.Fatalf("expired offer survived restart=%#v, %v", view, err)
	}
}
func TestRadarResponseLimit(t *testing.T) {
	service, _ := radarService(t)
	service.MarketClient = radarTestClient(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(strings.Repeat(" ", maxMarketResponseBytes+1)))}, nil
	})
	view, err := service.RefreshMarketPrice()
	if err != nil || !strings.Contains(view.Warning, "size limit") {
		t.Fatalf("oversize response state=%#v, %v", view, err)
	}
}

func TestRadarCacheControlAndPresetSourceTime(t *testing.T) {
	service, now := radarService(t)
	pointer, snapshot := radarFixture(*now)
	snapshot.Products[0].Presets[0].GeneratedAt = now.Add(-119 * time.Minute)
	calls := 0
	service.MarketClient = radarTestClient(func(request *http.Request) (*http.Response, error) {
		calls++
		if request.URL.String() == defaultMarketPriceURL {
			response := radarResponse(200, pointer)
			response.Header.Set("Cache-Control", "public, max-age=300")
			response.Header.Set("Age", "30")
			return response, nil
		}
		return radarResponse(200, snapshot), nil
	})
	view, err := service.RefreshMarketPrice()
	if err != nil || !view.RenewalUsable {
		t.Fatalf("initial preset = %#v, %v", view, err)
	}
	if !view.RenewalSnapshot.SourceUpdatedAt.Equal(snapshot.Products[0].Presets[0].GeneratedAt) {
		t.Fatal("preset source time was replaced by newer parent time")
	}
	*now = now.Add(2 * time.Minute)
	view, err = service.RefreshMarketPrice()
	if err != nil || view.RenewalUsable || !view.AcquisitionUsable || calls != 2 {
		t.Fatalf("cache lifetime / independent preset expiry = %#v, calls=%d, err=%v", view, calls, err)
	}
}
