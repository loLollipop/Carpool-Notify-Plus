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
	flags := flag.NewFlagSet("apply-benefit-extension", flag.ExitOnError)
	databasePath := flags.String("database", "", "path to the production SQLite database")
	benefitID := flags.Int64("benefit-id", 0, "existing customer benefit ID")
	subscriptionID := flags.Int64("subscription-id", 0, "expected subscription ID")
	days := flags.Int("days", 0, "extension days to apply")
	expectedDue := flags.String("expected-due", "", "expected current effective due date (YYYY-MM-DD)")
	apply := flags.Bool("apply", false, "perform the mutation after validating every expected value")
	_ = flags.Parse(os.Args[1:])

	if strings.TrimSpace(*databasePath) == "" || *benefitID <= 0 || *subscriptionID <= 0 || *days < 1 || *days > 365 {
		log.Fatal("database, positive benefit-id/subscription-id, and days 1-365 are required")
	}
	if _, err := time.ParseInLocation("2006-01-02", strings.TrimSpace(*expectedDue), cycle.Location); err != nil {
		log.Fatalf("expected-due: %v", err)
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

	event, applied, err := store.ApplyCustomerBenefitExtension(
		*benefitID,
		*subscriptionID,
		*days,
		strings.TrimSpace(*expectedDue),
		time.Now(),
	)
	if err != nil {
		log.Fatalf("apply extension: %v", err)
	}
	status := "already_applied"
	if applied {
		status = "applied"
	}
	fmt.Printf(
		"status=%s benefit_id=%d subscription_id=%d base_due=%s previous_due=%s effective_due=%s days=%d\n",
		status,
		event.CustomerBenefitID,
		event.SubscriptionID,
		event.BaseDueDate,
		event.PreviousEffectiveDueDate,
		event.EffectiveDueDate,
		event.ExtensionDays,
	)
}
