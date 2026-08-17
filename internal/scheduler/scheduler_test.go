package scheduler

import (
	"context"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"carpool-notify/internal/cycle"
	"carpool-notify/internal/db"
	"carpool-notify/internal/service"
)

type schedulerMarketClient struct {
	calls atomic.Int32
}

func (client *schedulerMarketClient) Do(*http.Request) (*http.Response, error) {
	client.calls.Add(1)
	return &http.Response{
		StatusCode: http.StatusOK,
		Body: io.NopCloser(strings.NewReader(`{
			"offers": [
				{"sourceId":"a","sourceTitle":"ChatGPT Business 席位","price":110,"currency":"CNY","status":"in_stock","effectiveStatus":"available"},
				{"sourceId":"b","sourceTitle":"ChatGPT Team 激活码","price":130,"currency":"CNY","status":"in_stock","effectiveStatus":"available"},
				{"sourceId":"c","sourceTitle":"ChatGPT Business Slot","price":150,"currency":"CNY","status":"in_stock","effectiveStatus":"available"}
			]
		}`)),
	}, nil
}

func TestRunnerRefreshesMarketOnStartupAndAfterInterval(t *testing.T) {
	store, err := db.Open(filepath.Join(t.TempDir(), "scheduler-market.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })

	var now atomic.Value
	now.Store(time.Date(2026, time.August, 15, 12, 0, 0, 0, cycle.Location))
	client := &schedulerMarketClient{}
	subscriptionService := &service.SubscriptionService{
		Store:        store,
		Clock:        func() time.Time { return now.Load().(time.Time) },
		MarketClient: client,
	}
	runner := &Runner{
		Service:        subscriptionService,
		Interval:       time.Hour,
		MarketInterval: 10 * time.Millisecond,
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		runner.Start(ctx)
		close(done)
	}()

	waitForMarketCalls(t, client, 1)
	waitForMarketSnapshots(t, store, 1)
	now.Store(now.Load().(time.Time).Add(service.MarketRefreshInterval() + time.Minute))
	waitForMarketCalls(t, client, 2)
	waitForMarketSnapshots(t, store, 2)
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("scheduler did not stop after context cancellation")
	}
}

func waitForMarketSnapshots(t *testing.T, store *db.Store, want int) {
	t.Helper()
	deadline := time.After(time.Second)
	ticker := time.NewTicker(5 * time.Millisecond)
	defer ticker.Stop()
	for {
		snapshots, err := store.ListMarketPriceSnapshots("priceai", "chatgpt-team-business", 10)
		if err != nil {
			t.Fatal(err)
		}
		if len(snapshots) >= want {
			return
		}
		select {
		case <-deadline:
			t.Fatalf("market snapshots = %d, want at least %d", len(snapshots), want)
		case <-ticker.C:
		}
	}
}

func waitForMarketCalls(t *testing.T, client *schedulerMarketClient, want int32) {
	t.Helper()
	deadline := time.After(time.Second)
	ticker := time.NewTicker(5 * time.Millisecond)
	defer ticker.Stop()
	for {
		if client.calls.Load() >= want {
			return
		}
		select {
		case <-deadline:
			t.Fatalf("market calls = %d, want at least %d", client.calls.Load(), want)
		case <-ticker.C:
		}
	}
}
