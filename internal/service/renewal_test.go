package service_test

import (
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"carpool-notify/internal/cycle"
	"carpool-notify/internal/db"
	"carpool-notify/internal/model"
	"carpool-notify/internal/service"
)

// Historical fixtures use an independent SQLite connection, never a runtime renewal writer.
func openHistoricalRenewalTestService(t *testing.T) (*service.SubscriptionService, *sql.DB) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "historical-renewal.db")
	store, err := db.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	history, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = history.Close() })
	return &service.SubscriptionService{Store: store}, history
}

func seedHistoricalRenewalApplication(t *testing.T, history *sql.DB, subscriptionID int64, status string, periods int, endDate string) {
	t.Helper()
	_, err := history.Exec(`INSERT INTO renewal_applications (
        tracking_token, subscription_id, customer_email, due_date, period_count, period_end_date,
        amount_cents, status, created_at, updated_at
    ) VALUES ('historical', ?, 'customer@example.com', '2026-08-31', ?, ?, ?, ?, '2026-08-20T00:00:00Z', '2026-08-20T00:00:00Z')`,
		subscriptionID, periods, endDate, int64(periods)*9000, status)
	if err != nil {
		t.Fatal(err)
	}
}

func TestHistoricalMultiPeriodRenewalProgressAndAccountingCorrection(t *testing.T) {
	subscriptionService, history := openHistoricalRenewalTestService(t)
	subscriptionService.Clock = func() time.Time {
		return time.Date(2026, time.August, 20, 10, 0, 0, 0, cycle.Location)
	}
	_, seatIDs := createTestAccountWithSeats(t, subscriptionService, "多周期续费母号", "车位1")
	subscriptionID, err := subscriptionService.CreateWithInitialBill(service.CreateInput{
		Name:             "多周期续费客户",
		PriceYuan:        "90",
		CronExpr:         "interval:30d",
		NotifyOffsetsRaw: "3",
		CustomerEmail:    "multi-period@example.com",
		SeatID:           seatIDs[0],
		BoardedAt:        "2026-08-01",
	})
	if err != nil {
		t.Fatal(err)
	}

	seedHistoricalRenewalApplication(t, history, subscriptionID, "approved", 2, "2026-10-30")
	for _, dueDate := range []string{"2026-08-31", "2026-09-30"} {
		if err := subscriptionService.Store.SetDuePaid(subscriptionID, dueDate, true, 9000); err != nil {
			t.Fatal(err)
		}
	}
	views, err := subscriptionService.ListView()
	if err != nil {
		t.Fatal(err)
	}
	if len(views) != 1 || views[0].NextDueDate != "2026-10-30" ||
		views[0].DaysRemaining != 71 || views[0].CycleDays != 90 {
		t.Fatalf("multi-period progress window = %#v, want 71/90 days", views)
	}
	subscriptionService.Clock = func() time.Time {
		return time.Date(2026, time.September, 15, 10, 0, 0, 0, cycle.Location)
	}
	views, err = subscriptionService.ListView()
	if err != nil {
		t.Fatal(err)
	}
	if len(views) != 1 || views[0].NextDueDate != "2026-10-30" ||
		views[0].DaysRemaining != 45 || views[0].CycleDays != 60 {
		t.Fatalf("multi-period progress after one cycle = %#v, want 45/60 days", views)
	}
	subscriptionService.Clock = func() time.Time {
		return time.Date(2026, time.October, 1, 10, 0, 0, 0, cycle.Location)
	}
	views, err = subscriptionService.ListView()
	if err != nil {
		t.Fatal(err)
	}
	if len(views) != 1 || views[0].NextDueDate != "2026-10-30" ||
		views[0].DaysRemaining != 29 || views[0].CycleDays != 30 {
		t.Fatalf("multi-period progress in final cycle = %#v, want 29/30 days", views)
	}
	for _, dueDate := range []string{"2026-08-31", "2026-09-30"} {
		bill, billErr := subscriptionService.Store.GetBillByOccurrence(subscriptionID, dueDate)
		if billErr != nil || bill.AmountCents != 9000 {
			t.Fatalf("bill %s = %#v, error = %v", dueDate, bill, billErr)
		}
	}
	// If accounting correction removes part of the approved package, the old
	// application must no longer widen progress for the now-shorter paid window.
	if err := subscriptionService.SetDuePaid(subscriptionID, "2026-09-30", false); err != nil {
		t.Fatal(err)
	}
	subscriptionService.Clock = func() time.Time {
		return time.Date(2026, time.August, 20, 10, 0, 0, 0, cycle.Location)
	}
	views, err = subscriptionService.ListView()
	if err != nil {
		t.Fatal(err)
	}
	if len(views) != 1 || views[0].NextDueDate != "2026-09-30" ||
		views[0].DaysRemaining != 41 || views[0].CycleDays != 30 {
		t.Fatalf("corrected multi-period progress = %#v, want 41 days capped by one 30-day cycle", views)
	}
}

func TestMultiPeriodRenewalProgressKeepsFullWindowAfterExtension(t *testing.T) {
	subscriptionService, history := openHistoricalRenewalTestService(t)
	subscriptionService.Clock = func() time.Time {
		return time.Date(2026, time.August, 20, 10, 0, 0, 0, cycle.Location)
	}
	_, seatIDs := createTestAccountWithSeats(t, subscriptionService, "延期多周期母号", "车位1")
	subscriptionID, err := subscriptionService.CreateWithInitialBill(service.CreateInput{
		Name: "延期多周期客户", PriceYuan: "90", CronExpr: "interval:30d",
		NotifyOffsetsRaw: "3", CustomerEmail: "extended-multi-period@example.com",
		SeatID: seatIDs[0], BoardedAt: "2026-08-01",
	})
	if err != nil {
		t.Fatal(err)
	}
	seedHistoricalRenewalApplication(t, history, subscriptionID, "approved", 2, "2026-10-30")
	for _, dueDate := range []string{"2026-08-31", "2026-09-30"} {
		if err := subscriptionService.Store.SetDuePaid(subscriptionID, dueDate, true, 9000); err != nil {
			t.Fatal(err)
		}
	}
	reviewedViews, err := subscriptionService.ListView()
	if err != nil || len(reviewedViews) != 1 {
		t.Fatalf("extension review view = %#v, %v", reviewedViews, err)
	}
	if _, err := subscriptionService.RecordCustomerBenefits(service.RecordCustomerBenefitsInput{
		SubscriptionIDs: []int64{subscriptionID}, BenefitType: model.CustomerBenefitTypeExtension,
		ExtensionDays: 9, OperationKey: "test-multi-period-extension", BenefitDate: "2026-08-20",
		ExtensionReviewSnapshots: []service.ExtensionReviewSnapshot{{
			SubscriptionID: subscriptionID, ExpectedUpdatedAt: reviewedViews[0].Subscription.UpdatedAt.Format(time.RFC3339Nano),
			ExpectedDueDate: reviewedViews[0].NextDueDate,
		}},
	}); err != nil {
		t.Fatal(err)
	}
	views, err := subscriptionService.ListView()
	if err != nil || len(views) != 1 || views[0].NextDueDate != "2026-11-08" ||
		views[0].DaysRemaining != 80 || views[0].CycleDays != 99 {
		t.Fatalf("extended multi-period progress = %#v, %v; want 80/99 days", views, err)
	}
	var endDate string
	if err := history.QueryRow(`SELECT period_end_date FROM renewal_applications WHERE subscription_id = ?`, subscriptionID).Scan(&endDate); err != nil || endDate != "2026-10-30" {
		t.Fatalf("historical renewal snapshot changed: %q, %v", endDate, err)
	}
	for _, dueDate := range []string{"2026-08-31", "2026-09-30"} {
		if _, err := subscriptionService.Store.GetBillByOccurrence(subscriptionID, dueDate); err != nil {
			t.Fatalf("historical bill %s changed: %v", dueDate, err)
		}
	}
}

func TestHistoricalPendingRenewalDoesNotAppearInOperationsOverview(t *testing.T) {
	subscriptionService, history := openHistoricalRenewalTestService(t)
	subscriptionService.Clock = func() time.Time {
		return time.Date(2026, time.August, 20, 10, 0, 0, 0, cycle.Location)
	}
	_, seatIDs := createTestAccountWithSeats(t, subscriptionService, "待办续费母号", "车位1")
	subscriptionID, err := subscriptionService.CreateWithInitialBill(service.CreateInput{
		Name:             "待办续费客户",
		PriceYuan:        "90",
		CronExpr:         "interval:30d",
		NotifyOffsetsRaw: "3",
		CustomerEmail:    "operations-renewal@example.com",
		CustomerWechat:   "wx-operations-renewal",
		SeatID:           seatIDs[0],
		BoardedAt:        "2026-08-01",
	})
	if err != nil {
		t.Fatal(err)
	}
	seedHistoricalRenewalApplication(t, history, subscriptionID, "pending", 1, "2026-09-30")

	overview, err := subscriptionService.GetOperationsOverview()
	if err != nil {
		t.Fatal(err)
	}
	for _, tasks := range [][]service.OperationTask{overview.Tasks, overview.Notifications} {
		for _, task := range tasks {
			if task.Kind == "renewal_review" || strings.Contains(task.Route, "section=renewals") {
				t.Fatalf("historical renewal still produces work: %#v", task)
			}
		}
	}
	if overview.Work.UrgentCount != 0 || overview.Unread.RedemptionCount != 0 {
		t.Fatalf("historical renewal still affects counts: %#v %#v", overview.Work, overview.Unread)
	}
}
