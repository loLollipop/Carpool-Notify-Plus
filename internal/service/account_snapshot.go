package service

import (
	"sort"
	"strings"
	"time"

	"carpool-notify/internal/cycle"
	"carpool-notify/internal/model"
)

// accountViewSnapshot contains every relation needed by the account list.
// Keeping it separate from single-account editing avoids an account/seat N+1
// query pattern on the dashboard and accounts page.
type accountViewSnapshot struct {
	now                        time.Time
	seatsByAccount             map[int64][]model.Seat
	activeBySeat               map[int64]model.Subscription
	frozenBySeat               map[int64]model.Subscription
	linkedCountBySeat          map[int64]int
	latestCostPeriodByAccount  map[int64]string
	pendingAfterSalesByAccount map[int64]int
}

func (service *SubscriptionService) loadAccountViewSnapshot() (accountViewSnapshot, error) {
	now := service.now()
	snapshot := accountViewSnapshot{
		now:                        now,
		seatsByAccount:             make(map[int64][]model.Seat),
		activeBySeat:               make(map[int64]model.Subscription),
		frozenBySeat:               make(map[int64]model.Subscription),
		linkedCountBySeat:          make(map[int64]int),
		latestCostPeriodByAccount:  make(map[int64]string),
		pendingAfterSalesByAccount: make(map[int64]int),
	}

	seats, err := service.Store.ListAllSeats()
	if err != nil {
		return accountViewSnapshot{}, err
	}
	for _, seat := range seats {
		snapshot.seatsByAccount[seat.AccountID] = append(snapshot.seatsByAccount[seat.AccountID], seat)
	}

	activeSubscriptions, err := service.Store.ListSubscriptions()
	if err != nil {
		return accountViewSnapshot{}, err
	}
	for _, subscription := range activeSubscriptions {
		if subscription.SeatID <= 0 {
			continue
		}
		snapshot.activeBySeat[subscription.SeatID] = subscription
		snapshot.linkedCountBySeat[subscription.SeatID]++
	}

	archivedSubscriptions, err := service.Store.ListArchivedSubscriptions()
	if err != nil {
		return accountViewSnapshot{}, err
	}
	for _, subscription := range archivedSubscriptions {
		if subscription.SeatID <= 0 {
			continue
		}
		snapshot.linkedCountBySeat[subscription.SeatID]++
		if subscription.SeatFrozenUntil == nil || !subscription.SeatFrozenUntil.After(now) {
			continue
		}
		current, exists := snapshot.frozenBySeat[subscription.SeatID]
		if !exists || current.SeatFrozenUntil == nil ||
			subscription.SeatFrozenUntil.After(*current.SeatFrozenUntil) ||
			(subscription.SeatFrozenUntil.Equal(*current.SeatFrozenUntil) && subscription.ID > current.ID) {
			snapshot.frozenBySeat[subscription.SeatID] = subscription
		}
	}

	costRecords, err := service.Store.ListAllAccountCostRecords()
	if err != nil {
		return accountViewSnapshot{}, err
	}
	for _, record := range costRecords {
		if !isAutomaticAccountCostSource(record.Source) {
			continue
		}
		if record.PeriodDate > snapshot.latestCostPeriodByAccount[record.AccountID] {
			snapshot.latestCostPeriodByAccount[record.AccountID] = record.PeriodDate
		}
	}

	afterSalesCases, err := service.Store.ListAfterSalesCases()
	if err != nil {
		return accountViewSnapshot{}, err
	}
	for _, caseItem := range afterSalesCases {
		if caseItem.AccountID > 0 && (caseItem.Status == model.AfterSalesStatusPending || caseItem.Status == model.AfterSalesStatusReview) {
			snapshot.pendingAfterSalesByAccount[caseItem.AccountID]++
		}
	}
	return snapshot, nil
}

func buildAccountViewFromSnapshot(account model.Account, snapshot accountViewSnapshot) (AccountView, error) {
	seats := snapshot.seatsByAccount[account.ID]
	sort.SliceStable(seats, func(left int, right int) bool { return seats[left].ID < seats[right].ID })
	seatViews := make([]SeatView, 0, len(seats))
	usedCount := 0
	for _, seat := range seats {
		seatView := buildSeatViewFromSnapshot(seat, snapshot)
		if seatView.Occupied || seatView.Frozen {
			usedCount++
		}
		seatViews = append(seatViews, seatView)
	}
	view := AccountView{
		Account:       account,
		DisplaySerial: accountDisplaySerial(account),
		Seats:         seatViews,
		SeatTotal:     len(seatViews),
		SeatUsed:      usedCount,
		IsFull:        len(seatViews) > 0 && usedCount >= len(seatViews),
		CanDelete:     usedCount == 0,
	}
	if strings.TrimSpace(account.BannedAt) == "" {
		renewalAt, err := nextAccountCostRenewalFromLatestPeriod(
			account,
			snapshot.latestCostPeriodByAccount[account.ID],
		)
		if err != nil {
			return AccountView{}, err
		}
		applyAccountRenewalFields(&view, account, renewalAt, snapshot.now)
	}
	return view, nil
}

func buildSeatViewFromSnapshot(seat model.Seat, snapshot accountViewSnapshot) SeatView {
	view := SeatView{
		Seat:                    seat,
		LinkedSubscriptionCount: snapshot.linkedCountBySeat[seat.ID],
	}
	if subscription, exists := snapshot.activeBySeat[seat.ID]; exists {
		view.Occupied = true
		view.ActiveSubscriptionID = subscription.ID
		view.ActiveSubscriptionName = subscription.Name
		view.ActiveBusinessType = subscription.BusinessType
		view.ActivePriceYuan = cycle.FormatCents(subscription.PricePerPersonCents)
		if subscription.NextPriceCents != nil {
			view.ActiveNextPriceYuan = cycle.FormatCents(*subscription.NextPriceCents)
		}
		view.ActiveNextPriceEffectiveDueDate = subscription.NextPriceEffectiveDueDate
		view.ActiveCostYuan = cycle.FormatCents(subscription.CostCents)
		view.ActiveAgencyFeeYuan = cycle.FormatCents(subscription.AgencyFeeCents)
		view.ActiveIsResale = subscription.IsResale
		view.ActiveCronExpr = subscription.CronExpr
		view.ActiveOffsetsText = cycle.FormatOffsets(subscription.NotifyOffsets)
		view.ActiveRemark = subscription.Remark
		view.ActiveTradeURL = subscription.TradeURL
		view.ActiveCustomerEmail = subscription.CustomerEmail
		view.ActiveCustomerWechat = subscription.CustomerWechat
		view.ActiveAccountID = subscription.AccountID
		if view.ActiveAccountID == 0 {
			view.ActiveAccountID = seat.AccountID
		}
		view.ActiveBoardedAt = subscription.BoardedAt
	} else if subscription, exists := snapshot.frozenBySeat[seat.ID]; exists {
		view.Frozen = true
		view.FrozenSubscriptionName = subscription.Name
		view.FrozenCustomerEmail = subscription.CustomerEmail
		if subscription.SeatFrozenUntil != nil {
			view.FrozenUntil = subscription.SeatFrozenUntil.UTC().Format(time.RFC3339)
			view.FrozenUntilLabel = subscription.SeatFrozenUntil.In(cycle.Location).Format("2006-01-02 15:04")
		}
	}
	view.CanDelete = !view.Occupied && !view.Frozen
	return view
}

func isAutomaticAccountCostSource(source string) bool {
	switch strings.TrimSpace(source) {
	case model.AccountCostSourceInitial, model.AccountCostSourceRenewal, model.AccountCostSourceZeroRenewal:
		return true
	default:
		return false
	}
}
