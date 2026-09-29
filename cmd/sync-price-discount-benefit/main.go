package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"carpool-notify/internal/cycle"
	"carpool-notify/internal/db"
)

func main() {
	flags := flag.NewFlagSet("sync-price-discount-benefit", flag.ExitOnError)
	databasePath := flags.String("database", "", "path to the production SQLite database")
	subscriptionID := flags.Int64("subscription-id", 0, "expected subscription ID")
	beforeCents := flags.Int64("before-cents", 0, "price before the scheduled discount, in cents")
	afterCents := flags.Int64("after-cents", 0, "price after the scheduled discount, in cents")
	effectiveDue := flags.String("effective-due", "", "effective billing date (YYYY-MM-DD)")
	apply := flags.Bool("apply", false, "perform the mutation after validating every expected value")
	_ = flags.Parse(os.Args[1:])

	if strings.TrimSpace(*databasePath) == "" || *subscriptionID <= 0 || *beforeCents <= *afterCents || *afterCents <= 0 {
		log.Fatal("database, positive subscription-id/prices, and before-cents > after-cents are required")
	}
	if _, err := time.ParseInLocation("2006-01-02", strings.TrimSpace(*effectiveDue), cycle.Location); err != nil {
		log.Fatalf("effective-due: %v", err)
	}
	if !*apply {
		log.Fatal("refusing to mutate without -apply")
	}
	databaseInfo, err := os.Stat(strings.TrimSpace(*databasePath))
	if err != nil {
		log.Fatalf("database path: %v", err)
	}
	if databaseInfo.IsDir() {
		log.Fatal("database path must be an existing file")
	}

	store, err := db.Open(strings.TrimSpace(*databasePath))
	if err != nil {
		log.Fatalf("open database: %v", err)
	}
	defer store.Close()

	created, err := store.SyncHistoricalPriceDiscountBenefit(
		*subscriptionID,
		*beforeCents,
		*afterCents,
		strings.TrimSpace(*effectiveDue),
	)
	if err != nil {
		log.Fatalf("sync historical price discount: %v", err)
	}
	status := "already_synced"
	if created {
		status = "synced"
	}
	fmt.Printf(
		"status=%s subscription_id=%d before_cents=%d after_cents=%d effective_due=%s\n",
		status,
		*subscriptionID,
		*beforeCents,
		*afterCents,
		strings.TrimSpace(*effectiveDue),
	)
}
