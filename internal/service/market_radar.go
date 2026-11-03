package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"carpool-notify/internal/model"
)

const marketRadarProvider = "priceai-radar"
const radarMaxAge = 2 * time.Hour
const marketMaxAge = 24 * time.Hour

// Only the immutable snapshot selected by the trusted public pointer is fetched.
var radarSnapshotID = regexp.MustCompile(`^[0-9]{14}-[a-zA-Z0-9]{6,64}$`)

type marketCacheState struct {
	hasResult        bool
	checkedAt        time.Time
	nextAttempt      time.Time
	failures         int
	warning          string
	warningCode      string
	etag             string
	lastModified     string
	snapshotID       string
	pointer          radarEnvelope
	acquisitionValid bool
	renewalValid     bool
	validUntil       map[string]time.Time
}

type radarEnvelope struct {
	SchemaVersion string         `json:"schema_version"`
	SnapshotID    string         `json:"snapshot_id"`
	GeneratedAt   time.Time      `json:"generated_at"`
	PublishedAt   time.Time      `json:"published_at"`
	Stale         *bool          `json:"stale"`
	RankingPolicy string         `json:"ranking_policy_version"`
	SnapshotURL   string         `json:"snapshot_url"`
	Products      []radarProduct `json:"products"`
}
type radarProduct struct {
	ID          string        `json:"id"`
	Slug        string        `json:"slug"`
	Platform    string        `json:"platform"`
	GeneratedAt time.Time     `json:"snapshot_generated_at"`
	TopOffers   []radarOffer  `json:"top_offers"`
	Presets     []radarPreset `json:"presets"`
}
type radarPreset struct {
	ID          string       `json:"id"`
	GeneratedAt time.Time    `json:"generated_at"`
	TopOffers   []radarOffer `json:"top_offers"`
}
type radarOffer struct {
	rankingGeneratedAt time.Time
	ID                 string    `json:"id"`
	SourceID           string    `json:"source_id"`
	Title              string    `json:"title"`
	Price              float64   `json:"price"`
	Currency           string    `json:"currency"`
	Status             string    `json:"status"`
	EffectiveStatus    string    `json:"effective_status"`
	FreshnessStatus    string    `json:"freshness_status"`
	StockCount         *int      `json:"stock_count"`
	MinimumOrder       *int      `json:"min_order_quantity"`
	CapturedAt         time.Time `json:"captured_at"`
	LastSeenAt         time.Time `json:"last_seen_at"`
	VerifiedAt         time.Time `json:"verified_at"`
	ExpiresAt          time.Time `json:"expires_at"`
}

func (service *SubscriptionService) internalMarketFixture() bool {
	// Explicit URL injection preserves the old internal-API fixture contract;
	// the production default always uses the public Radar, including its UA.
	return service.MarketClient != nil && strings.TrimSpace(service.MarketPriceURL) != "" && service.MarketPriceURL != defaultMarketPriceURL
}
func (service *SubscriptionService) currentMarketProvider() string {
	if service.internalMarketFixture() {
		return marketProvider
	}
	return marketRadarProvider
}

func timeFresh(at, now time.Time, age time.Duration) bool {
	return !at.IsZero() && !at.After(now.Add(5*time.Minute)) && now.Sub(at) <= age
}
func (service *SubscriptionService) marketSnapshotUsable(snapshot model.MarketPriceSnapshot) bool {
	age := marketMaxAge
	if snapshot.Provider == marketRadarProvider {
		age = radarMaxAge
	}
	return snapshot.SampleCount >= 3 && snapshot.LowPriceCents > 0 && snapshot.LowPriceCents <= snapshot.MedianPriceCents &&
		snapshot.MedianPriceCents <= snapshot.HighPriceCents && snapshot.HighPriceCents <= 30000 &&
		timeFresh(snapshot.SourceUpdatedAt, service.now(), age) && timeFresh(snapshot.CreatedAt, service.now(), age) &&
		(snapshot.Product == marketProduct || snapshot.Product == marketAcquisitionProduct)
}

func (service *SubscriptionService) readMarketSnapshots() (marketPriceSnapshots, *model.MarketPriceSnapshot, error) {
	var result marketPriceSnapshots
	provider := service.currentMarketProvider()
	for _, target := range []*model.MarketPriceSnapshot{&result.Acquisition, &result.Renewal} {
		product := marketAcquisitionProduct
		if target == &result.Renewal {
			product = marketProduct
		}
		snapshot, err := service.Store.LatestMarketPriceSnapshot(provider, product)
		if err != nil && err != sql.ErrNoRows {
			return result, nil, err
		}
		if err == nil {
			*target = snapshot
		}
	}
	// Old caches remain visible, explicitly unusable, without fabricating split data.
	var legacy *model.MarketPriceSnapshot
	if result.Acquisition.SampleCount == 0 && result.Renewal.SampleCount == 0 {
		for _, product := range []string{marketProduct, legacyMarketProduct} {
			snapshot, err := service.Store.LatestMarketPriceSnapshot(marketProvider, product)
			if err != nil && err != sql.ErrNoRows {
				return result, nil, err
			}
			if err == nil {
				legacy = &snapshot
				break
			}
		}
	}
	return result, legacy, nil
}

// Goal rendering never waits for network I/O. The scheduler and explicit refresh
// update this cache; if a refresh holds the lock, only immutable DB rows are read.
func (service *SubscriptionService) cachedMarketPrice() (MarketPriceView, error) {
	if service.marketRefreshMu.TryLock() {
		defer service.marketRefreshMu.Unlock()
		snapshots, legacy, err := service.readMarketSnapshots()
		return service.marketView(snapshots, legacy, true), err
	}
	snapshots, legacy, err := service.readMarketSnapshots()
	view := service.marketView(snapshots, legacy, false)
	view.AcquisitionUsable = false
	view.RenewalUsable = false
	view.Stale = true
	view.Warning = "行情检查进行中，当前缓存仅供参考"
	view.WarningCode = "refreshing"
	return view, err
}

func (service *SubscriptionService) loadMarketPrice(force bool) (MarketPriceView, error) {
	service.marketRefreshMu.Lock()
	defer service.marketRefreshMu.Unlock()
	latest, legacy, err := service.readMarketSnapshots()
	if err != nil {
		return MarketPriceView{SourceURL: marketSourcePageURL}, err
	}
	now := service.now()
	if now.Before(service.marketCache.nextAttempt) {
		return service.marketView(latest, legacy, true), nil
	}
	if service.internalMarketFixture() && !force && service.marketSnapshotUsable(latest.Acquisition) && service.marketSnapshotUsable(latest.Renewal) &&
		now.Sub(latest.Acquisition.CreatedAt) < marketCacheTTL && now.Sub(latest.Renewal.CreatedAt) < marketCacheTTL {
		return service.marketView(latest, legacy, true), nil
	}
	service.marketCache.checkedAt = now.UTC()
	service.marketCache.nextAttempt = now.Add(time.Minute)
	var refreshed marketPriceSnapshots
	var unchanged bool
	if service.internalMarketFixture() {
		refreshed, err = service.fetchInternalMarketPriceSnapshots()
	} else {
		refreshed, unchanged, err = service.fetchRadarMarketSnapshots()
	}
	if err != nil {
		service.marketCache.failures++
		delay := time.Minute * time.Duration(1<<minInt(service.marketCache.failures, 6))
		service.marketCache.nextAttempt = now.Add(delay)
		service.marketCache.warning = "行情提供方暂时不可用，历史缓存仅供参考：" + err.Error()
		service.marketCache.warningCode = "provider_unavailable"
	} else if !unchanged {
		service.marketCache.hasResult = true
		rows := make([]model.MarketPriceSnapshot, 0, 2)
		for _, snapshot := range []model.MarketPriceSnapshot{refreshed.Acquisition, refreshed.Renewal} {
			if snapshot.SampleCount >= 3 {
				rows = append(rows, snapshot)
			}
		}
		if err = service.Store.InsertMarketPriceSnapshots(rows); err != nil {
			service.marketCache.snapshotID = ""
			return MarketPriceView{}, err
		}
		if refreshed.Acquisition.SampleCount >= 3 {
			latest.Acquisition = refreshed.Acquisition
		}
		if refreshed.Renewal.SampleCount >= 3 {
			latest.Renewal = refreshed.Renewal
		}
		if len(rows) > 0 {
			legacy = nil
		}
		service.marketCache.acquisitionValid = refreshed.Acquisition.SampleCount >= 3
		service.marketCache.renewalValid = refreshed.Renewal.SampleCount >= 3
		service.marketCache.failures = 0
		service.marketCache.warning = ""
		service.marketCache.warningCode = ""
		if len(rows) < 2 {
			service.marketCache.warning = "公开行情中可比的月租新购或续费报价不足 3 条，相关分段不参与自动调价"
			service.marketCache.warningCode = "insufficient_samples"
		}
	} else {
		service.marketCache.failures = 0
		if service.marketCache.warningCode == "provider_unavailable" {
			service.marketCache.warning = ""
			service.marketCache.warningCode = ""
		}
		if !service.marketCache.acquisitionValid || !service.marketCache.renewalValid {
			service.marketCache.warning = "公开行情中可比的月租新购或续费报价不足 3 条，相关分段不参与自动调价"
			service.marketCache.warningCode = "insufficient_samples"
		}
	}
	view := service.marketView(latest, legacy, true)
	// An unsuccessful provider check remains a successful, explicitly warned cache view.
	return view, nil
}

func (service *SubscriptionService) marketView(snapshots marketPriceSnapshots, legacy *model.MarketPriceSnapshot, withState bool) MarketPriceView {
	view := MarketPriceView{SourceURL: marketSourcePageURL, AcquisitionHistory: []model.MarketPriceSnapshot{}, RenewalHistory: []model.MarketPriceSnapshot{}, History: []model.MarketPriceSnapshot{}}
	if snapshots.Acquisition.SampleCount > 0 {
		acquisition := snapshots.Acquisition
		view.AcquisitionSnapshot = &acquisition
		view.AcquisitionHistory = service.marketPriceHistory(acquisition.Product, acquisition)
		view.AcquisitionUsable = service.marketSnapshotUsable(acquisition) && (acquisition.Provider != marketRadarProvider || (withState && service.marketCache.snapshotID != ""))
	}
	if snapshots.Renewal.SampleCount > 0 {
		renewal := snapshots.Renewal
		view.RenewalSnapshot = &renewal
		view.Snapshot = &renewal
		view.RenewalHistory = service.marketPriceHistory(renewal.Product, renewal)
		view.History = view.RenewalHistory
		view.RenewalUsable = service.marketSnapshotUsable(renewal) && (renewal.Provider != marketRadarProvider || (withState && service.marketCache.snapshotID != ""))
	}
	if legacy != nil {
		view.Snapshot = legacy
		view.History = []model.MarketPriceSnapshot{*legacy}
	}
	if withState {
		state := service.marketCache
		view.CheckedAt = state.checkedAt
		view.Warning = state.warning
		view.WarningCode = state.warningCode
		if state.hasResult {
			view.AcquisitionUsable = view.AcquisitionUsable && state.acquisitionValid
			view.RenewalUsable = view.RenewalUsable && state.renewalValid
		}
		for product, usableUntil := range state.validUntil {
			if !service.now().Before(usableUntil) {
				if product == marketAcquisitionProduct {
					view.AcquisitionUsable = false
				} else {
					view.RenewalUsable = false
				}
			}
		}
	}
	if withState && service.marketCache.warningCode == "provider_unavailable" {
		view.AcquisitionUsable = false
		view.RenewalUsable = false
	}
	view.Available = view.AcquisitionSnapshot != nil || view.RenewalSnapshot != nil || view.Snapshot != nil
	view.Stale = !view.AcquisitionUsable || !view.RenewalUsable
	if view.Warning == "" && view.Stale {
		view.WarningCode = "stale"
		view.Warning = "行情缺失或源时间已过期，历史缓存仅供参考，相关分段不参与自动调价"
	}
	return view
}

func (service *SubscriptionService) marketPriceHistory(product string, fallback model.MarketPriceSnapshot) []model.MarketPriceSnapshot {
	history, err := service.Store.ListMarketPriceSnapshots(fallback.Provider, product, marketHistoryLimit)
	if err != nil || len(history) == 0 {
		history = []model.MarketPriceSnapshot{fallback}
	}
	for left, right := 0, len(history)-1; left < right; left, right = left+1, right-1 {
		history[left], history[right] = history[right], history[left]
	}
	return history
}

func normalizedMarketSnapshot(provider, product string, prices []int64, source, collected time.Time) model.MarketPriceSnapshot {
	return model.MarketPriceSnapshot{Provider: provider, Product: product, SampleCount: len(prices), LowPriceCents: percentileCents(prices, .25), MedianPriceCents: percentileCents(prices, .5), HighPriceCents: percentileCents(prices, .75), SourceUpdatedAt: source.UTC(), CreatedAt: collected.UTC()}
}

func (service *SubscriptionService) radarGET(rawURL string, conditional bool, target any) (bool, http.Header, error) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultMarketTimeout)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return false, nil, err
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("User-Agent", "CarpoolNotify/1.0 market-monitor")
	if conditional {
		request.Header.Set("If-None-Match", service.marketCache.etag)
		request.Header.Set("If-Modified-Since", service.marketCache.lastModified)
	}
	client := service.MarketClient
	if client == nil {
		client = &http.Client{Timeout: defaultMarketTimeout, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	}
	response, err := client.Do(request)
	if err != nil {
		return false, nil, err
	}
	defer response.Body.Close()
	if response.Request != nil && response.Request.URL.String() != rawURL {
		return false, nil, fmt.Errorf("unexpected Price Radar redirect")
	}
	if response.StatusCode == http.StatusNotModified && conditional {
		return true, response.Header, nil
	}
	if response.StatusCode != http.StatusOK {
		return false, nil, fmt.Errorf("Price Radar returned HTTP %d", response.StatusCode)
	}
	bytes, err := io.ReadAll(io.LimitReader(response.Body, maxMarketResponseBytes+1))
	if err != nil {
		return false, nil, err
	}
	if len(bytes) > maxMarketResponseBytes {
		return false, nil, fmt.Errorf("Price Radar response exceeds size limit")
	}
	if err := json.Unmarshal(bytes, target); err != nil {
		return false, nil, fmt.Errorf("decode Price Radar: %w", err)
	}
	return false, response.Header, nil
}
func validateRadarEnvelope(payload radarEnvelope, now time.Time) error {
	if payload.SchemaVersion != "price-radar.v1" || payload.RankingPolicy != "available-non-shared-first.v1" || !radarSnapshotID.MatchString(payload.SnapshotID) || payload.Stale == nil {
		return fmt.Errorf("invalid Price Radar schema or snapshot identity")
	}
	if *payload.Stale || !timeFresh(payload.GeneratedAt, now, radarMaxAge) || !timeFresh(payload.PublishedAt, now, radarMaxAge) || payload.PublishedAt.Before(payload.GeneratedAt) {
		return fmt.Errorf("stale Price Radar source")
	}
	return nil
}
func (service *SubscriptionService) fetchRadarMarketSnapshots() (marketPriceSnapshots, bool, error) {
	var pointer radarEnvelope
	notModified, headers, err := service.radarGET(defaultMarketPriceURL, true, &pointer)
	if err != nil {
		return marketPriceSnapshots{}, false, err
	}
	// Cache-Control may ask us to poll less often than the one-minute budget.
	if delay := radarCacheDelay(headers); delay > time.Minute {
		service.marketCache.nextAttempt = service.now().Add(delay)
	}
	if notModified {
		pointer = service.marketCache.pointer
		if pointer.SnapshotID == "" {
			return marketPriceSnapshots{}, false, fmt.Errorf("unexpected uncached 304")
		}
	}
	if err := validateRadarEnvelope(pointer, service.now()); err != nil {
		return marketPriceSnapshots{}, false, err
	}
	expectedURL := "https://data.priceai.cc/v1/snapshots/" + pointer.SnapshotID + ".json"
	if pointer.SnapshotURL != expectedURL {
		return marketPriceSnapshots{}, false, fmt.Errorf("untrusted Price Radar snapshot URL")
	}
	if !notModified {
		service.marketCache.pointer = pointer
		service.marketCache.etag = headers.Get("ETag")
		service.marketCache.lastModified = headers.Get("Last-Modified")
	}
	if pointer.SnapshotID == service.marketCache.snapshotID {
		return marketPriceSnapshots{}, true, nil
	}
	var snapshot radarEnvelope
	_, _, err = service.radarGET(pointer.SnapshotURL, false, &snapshot)
	if err != nil {
		return marketPriceSnapshots{}, false, err
	}
	if err := validateRadarEnvelope(snapshot, service.now()); err != nil {
		return marketPriceSnapshots{}, false, err
	}
	if snapshot.SnapshotID != pointer.SnapshotID || !snapshot.GeneratedAt.Equal(pointer.GeneratedAt) || !snapshot.PublishedAt.Equal(pointer.PublishedAt) {
		return marketPriceSnapshots{}, false, fmt.Errorf("mismatched Price Radar snapshot")
	}
	var product *radarProduct
	for index := range snapshot.Products {
		candidate := &snapshot.Products[index]
		if candidate.ID == "chatgpt-team-business" {
			if product != nil || candidate.Slug != candidate.ID || candidate.Platform != "ChatGPT" {
				return marketPriceSnapshots{}, false, fmt.Errorf("invalid Team product identity")
			}
			product = candidate
		}
	}
	if product == nil || !timeFresh(product.GeneratedAt, service.now(), radarMaxAge) {
		return marketPriceSnapshots{}, false, fmt.Errorf("missing or stale Team product")
	}
	if len(product.TopOffers) > 5 || len(product.Presets) > 5 {
		return marketPriceSnapshots{}, false, fmt.Errorf("invalid Radar ranking size")
	}
	offers := append([]radarOffer(nil), product.TopOffers...)
	for index := range offers {
		offers[index].rankingGeneratedAt = product.GeneratedAt
	}
	for _, preset := range product.Presets {
		if len(preset.TopOffers) > 5 {
			return marketPriceSnapshots{}, false, fmt.Errorf("invalid Radar preset size")
		}
		if timeFresh(preset.GeneratedAt, service.now(), radarMaxAge) {
			for _, offer := range preset.TopOffers {
				offer.rankingGeneratedAt = preset.GeneratedAt
				offers = append(offers, offer)
			}
		}
	}
	result := marketPriceSnapshots{}
	service.marketCache.validUntil = make(map[string]time.Time)
	for _, segment := range []marketOfferSegment{marketOfferAcquisition, marketOfferRenewal} {
		comparable := []priceAIOffer{}
		oldest := minTime(snapshot.GeneratedAt, product.GeneratedAt)
		until := oldest.Add(radarMaxAge)
		seen := map[string]bool{}
		for _, offer := range offers {
			if offer.ID == "" || seen[offer.ID] || !radarMonthlyOffer(offer, service.now()) {
				continue
			}
			legacy := priceAIOffer{SourceID: offer.SourceID, SourceTitle: offer.Title, Price: offer.Price, Currency: offer.Currency, Status: offer.Status, EffectiveStatus: offer.EffectiveStatus, StockCount: offer.StockCount}
			if !isComparableMarketOffer(legacy) || marketSegmentForOffer(legacy) != segment {
				continue
			}
			seen[offer.ID] = true
			comparable = append(comparable, legacy)
			// The oldest observed sample bounds the entire segment, never the newest.
			oldest = minTime(oldest, offer.rankingGeneratedAt)
			oldest = minTime(oldest, offer.CapturedAt)
			oldest = minTime(oldest, offer.LastSeenAt)
			oldest = minTime(oldest, offer.VerifiedAt)
			until = minTime(until, offer.ExpiresAt)
		}
		prices, _ := comparableMarketPrices(comparable, segment)
		prices, _ = excludeDetachedLowPriceCluster(prices)
		if len(prices) < 3 {
			continue
		}
		productID := marketAcquisitionProduct
		if segment == marketOfferRenewal {
			productID = marketProduct
		}
		normalized := normalizedMarketSnapshot(marketRadarProvider, productID, prices, oldest, service.now())
		if segment == marketOfferAcquisition {
			result.Acquisition = normalized
		} else {
			result.Renewal = normalized
		}
		service.marketCache.validUntil[productID] = minTime(until, oldest.Add(radarMaxAge))
	}
	service.marketCache.snapshotID = snapshot.SnapshotID
	return result, false, nil
}
func minTime(left, right time.Time) time.Time {
	if right.Before(left) {
		return right
	}
	return left
}
func radarMonthlyOffer(offer radarOffer, now time.Time) bool {
	if offer.EffectiveStatus != "available" || offer.FreshnessStatus != "fresh" || (offer.MinimumOrder != nil && *offer.MinimumOrder > 1) ||
		!timeFresh(offer.CapturedAt, now, radarMaxAge) || !timeFresh(offer.LastSeenAt, now, radarMaxAge) || !timeFresh(offer.VerifiedAt, now, radarMaxAge) || !offer.ExpiresAt.After(now) {
		return false
	}
	title := strings.ToLower(offer.Title)
	if !strings.Contains(title, "team") && !strings.Contains(title, "business") {
		return false
	}
	for _, excluded := range []string{"非出租", "非月租", "周租", "周卡", "周抛", "日租", "日抛", "半月", "15天", "7 天", "5 天", "成品号", "终身", "直到空间死", "不踢不拉", "5x", "k12", "年付", "年租", "季付", "季租", "三个月", "3个月", "90天", "180天", "7天", "5天", "24小时", "weekly", "daily", "annual", "yearly", "5 seat", "5席", "五席"} {
		if strings.Contains(title, excluded) {
			return false
		}
	}
	for _, month := range []string{"月租", "月付", "月卡", "一个月", "1个月", "30天", "30 天", "30 days", "30 day", "monthly", "1 month"} {
		if strings.Contains(title, month) {
			return true
		}
	}
	return false
}

func radarCacheDelay(headers http.Header) time.Duration {
	age, _ := strconv.ParseInt(headers.Get("Age"), 10, 64)
	for _, directive := range strings.Split(headers.Get("Cache-Control"), ",") {
		pair := strings.SplitN(strings.TrimSpace(directive), "=", 2)
		if len(pair) != 2 || !strings.EqualFold(pair[0], "max-age") {
			continue
		}
		seconds, err := strconv.ParseInt(strings.Trim(pair[1], "\""), 10, 64)
		if err == nil && seconds > maxInt64(age, 0) {
			// Restrict arithmetic to one year, beyond any useful feed freshness.
			return time.Duration(minInt64(seconds-maxInt64(age, 0), 365*24*60*60)) * time.Second
		}
	}
	return time.Minute
}
