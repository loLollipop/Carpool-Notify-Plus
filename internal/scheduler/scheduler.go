package scheduler

import (
	"context"
	"log"
	"time"

	"carpool-notify/internal/service"
)

// Runner periodically processes due notifications.
type Runner struct {
	Service        *service.SubscriptionService
	Interval       time.Duration
	MarketInterval time.Duration
}

// Start runs the loop until the context is cancelled.
func (runner *Runner) Start(ctx context.Context) {
	if runner.Interval <= 0 {
		runner.Interval = time.Minute
	}
	if runner.MarketInterval <= 0 {
		runner.MarketInterval = service.MarketRefreshCheckInterval()
	}

	// Run once on startup for catch-up.
	runner.tick(ctx)
	runner.refreshMarket()

	ticker := time.NewTicker(runner.Interval)
	defer ticker.Stop()
	marketTicker := time.NewTicker(runner.MarketInterval)
	defer marketTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			runner.tick(ctx)
		case <-marketTicker.C:
			runner.refreshMarket()
		}
	}
}

func (runner *Runner) refreshMarket() {
	market, err := runner.Service.RefreshMarketPriceIfStale()
	if err != nil {
		log.Printf("scheduler market prices: %v", err)
		return
	}
	if market.Warning != "" {
		log.Printf("scheduler market prices: %s", market.Warning)
	}
}

func (runner *Runner) tick(ctx context.Context) {
	if err := runner.Service.ProcessAccountCostRenewals(); err != nil {
		log.Printf("scheduler account costs: %v", err)
	}
	if err := runner.Service.ProcessDueNotifications(ctx); err != nil {
		log.Printf("scheduler notifications: %v", err)
	}
}
