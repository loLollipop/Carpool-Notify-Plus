package service

import (
	"fmt"
	"sort"
	"time"

	"carpool-notify/internal/cycle"
	"carpool-notify/internal/model"
)

// CalendarActivityView is one operation in the existing subscription ledger.
// A creation includes its atomic initial payment; later bills remain separate.
type CalendarActivityView struct {
	ID              string                 `json:"id"`
	Kind            string                 `json:"kind"`
	OccurredAt      time.Time              `json:"occurred_at"`
	OccurredAtLabel string                 `json:"occurred_at_label"`
	AmountYuan      string                 `json:"amount_yuan"`
	Archived        bool                   `json:"archived"`
	Occurrence      CalendarOccurrenceView `json:"occurrence"`
}

func (service *SubscriptionService) buildCalendarActivity(
	monthStart, monthEnd time.Time,
	active, archived []SubscriptionView,
	bills []model.Bill,
	allocatedCosts map[int64]int64,
) ([]CalendarActivityView, error) {
	views := make([]SubscriptionView, 0, len(active)+len(archived))
	views = append(views, active...)
	views = append(views, archived...)
	byID := make(map[int64]SubscriptionView, len(views))
	for _, view := range views {
		byID[view.Subscription.ID] = view
	}
	initialBills := make(map[int64]model.Bill)
	for _, bill := range bills {
		view, exists := byID[bill.SubscriptionID]
		// The atomic create path writes both rows with the same timestamp.
		// Do not guess from due dates: legacy rows may receive their first bill later.
		if exists && bill.CreatedAt.Equal(view.Subscription.CreatedAt) && bill.PaidAt.Equal(view.Subscription.CreatedAt) {
			initialBills[bill.SubscriptionID] = bill
		}
	}

	items := make([]CalendarActivityView, 0)
	appendItem := func(id, kind string, at time.Time, view SubscriptionView, bill model.Bill) error {
		if at.Before(monthStart) || !at.Before(monthEnd) {
			return nil
		}
		dueAt := at.In(cycle.Location)
		amountYuan := ""
		if bill.ID > 0 {
			var err error
			dueAt, err = time.ParseInLocation("2006-01-02", bill.DueDate, cycle.Location)
			if err != nil {
				return fmt.Errorf("invalid activity bill %d due date %q: %w", bill.ID, bill.DueDate, err)
			}
			amountYuan = cycle.FormatCents(bill.AmountCents)
		}
		occurrence := service.buildOccurrenceView(
			view.Subscription, view.AccountSerial, dueAt, bill.ID > 0, bill,
			allocatedCosts[view.Subscription.ID], view.AccountDisplayEmail, view.AccountSpaceRole,
		)
		// Archived Team allocation and no-bill historical costs cannot be
		// reconstructed. Plus payments still retain their bill cost snapshot.
		if view.Subscription.ArchivedAt != nil && (!isPlusSubscription(view.Subscription) || bill.ID == 0) {
			occurrence.CostYuan = ""
			occurrence.ProfitYuan = ""
		}
		items = append(items, CalendarActivityView{
			ID:              id,
			Kind:            kind,
			OccurredAt:      at,
			OccurredAtLabel: at.In(cycle.Location).Format("2006-01-02 15:04"),
			AmountYuan:      amountYuan,
			Archived:        view.Subscription.ArchivedAt != nil,
			Occurrence:      occurrence,
		})
		return nil
	}
	for _, view := range views {
		subscription := view.Subscription
		if err := appendItem(fmt.Sprintf("created:%d", subscription.ID), "created", subscription.CreatedAt, view, initialBills[subscription.ID]); err != nil {
			return nil, err
		}
		if subscription.ArchivedAt != nil {
			if err := appendItem(fmt.Sprintf("archived:%d", subscription.ID), "archived", *subscription.ArchivedAt, view, model.Bill{}); err != nil {
				return nil, err
			}
		}
	}
	for _, bill := range bills {
		view, exists := byID[bill.SubscriptionID]
		if !exists || bill.ID == initialBills[bill.SubscriptionID].ID {
			continue
		}
		if err := appendItem(fmt.Sprintf("paid:%d", bill.ID), "paid", bill.PaidAt, view, bill); err != nil {
			return nil, err
		}
	}
	sort.Slice(items, func(left, right int) bool {
		if items[left].OccurredAt.Equal(items[right].OccurredAt) {
			return items[left].ID > items[right].ID
		}
		return items[left].OccurredAt.After(items[right].OccurredAt)
	})
	return items, nil
}
