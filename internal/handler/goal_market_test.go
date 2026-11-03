package handler

import (
	"carpool-notify/internal/model"
	"carpool-notify/internal/service"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"
)

type unavailableGoalMarketClient struct{}

func (unavailableGoalMarketClient) Do(*http.Request) (*http.Response, error) {
	return nil, errors.New("provider offline")
}
func TestPostRefreshGoalMarketReturnsWarningWithoutFalseUpdate(t *testing.T) {
	server, router := subscriptionEditTestServer(t)
	router.POST("/goals/market/refresh", server.postRefreshGoalMarket)
	server.Service.MarketClient = unavailableGoalMarketClient{}
	now := server.Service.Clock()
	if _, err := server.Service.Store.InsertMarketPriceSnapshot(model.MarketPriceSnapshot{Provider: "priceai", Product: "chatgpt-team-business-renewal", LowPriceCents: 10000, MedianPriceCents: 12000, HighPriceCents: 14000, SampleCount: 8, SourceUpdatedAt: now.Add(-48 * time.Hour), CreatedAt: now.Add(-48 * time.Hour)}); err != nil {
		t.Fatal(err)
	}
	response := subscriptionEditRequest(t, router, http.MethodPost, "/goals/market/refresh", map[string]any{})
	if response.Code != http.StatusOK {
		t.Fatalf("cache warning response=%d %s", response.Code, response.Body.String())
	}
	var body struct {
		Message string                  `json:"message"`
		Market  service.MarketPriceView `json:"market"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Message == "" || strings.Contains(body.Message, "已更新") || !body.Market.Stale || body.Market.WarningCode != "provider_unavailable" || body.Market.AcquisitionUsable || body.Market.RenewalUsable || body.Market.CheckedAt.IsZero() {
		t.Fatalf("misleading refresh=%s", response.Body.String())
	}
}
