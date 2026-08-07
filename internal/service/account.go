package service

import (
	"database/sql"
	"fmt"
	"net/mail"
	"strings"
	"time"

	"carpool-notify/internal/cycle"
	"carpool-notify/internal/model"
)

// AccountView is one account with seat occupancy for the accounts page and forms.
type AccountView struct {
	Account   model.Account `json:"account"`
	Seats     []SeatView    `json:"seats"`
	SeatTotal int           `json:"seat_total"`
	SeatUsed  int           `json:"seat_used"`
	IsFull    bool          `json:"is_full"`
	CanDelete bool          `json:"can_delete"`
}

// SeatView is one seat with optional active subscription occupancy.
type SeatView struct {
	Seat                   model.Seat `json:"seat"`
	Occupied               bool       `json:"occupied"`
	ActiveSubscriptionID   int64      `json:"active_subscription_id"`
	ActiveSubscriptionName string     `json:"active_subscription_name"`
	// Edit form fields for the occupying subscription (empty when free).
	ActivePriceYuan         string `json:"active_price_yuan"`
	ActiveCostYuan          string `json:"active_cost_yuan"`
	ActiveAgencyFeeYuan     string `json:"active_agency_fee_yuan"`
	ActiveIsResale          bool   `json:"active_is_resale"`
	ActiveCronExpr          string `json:"active_cron_expr"`
	ActiveOffsetsText       string `json:"active_offsets_text"`
	ActiveRemark            string `json:"active_remark"`
	ActiveTradeURL          string `json:"active_trade_url"`
	ActiveCustomerEmail     string `json:"active_customer_email"`
	ActiveCustomerWechat    string `json:"active_customer_wechat"`
	ActiveAccountID         int64  `json:"active_account_id"`
	ActiveBoardedAt         string `json:"active_boarded_at"`
	LinkedSubscriptionCount int    `json:"linked_subscription_count"`
	CanDelete               bool   `json:"can_delete"`
}

// CreateAccountInput creates an account with at least one named seat.
// Prefer SeatCount (1–1000); seats are auto-named 车位1…车位N.
// SeatNames is an optional override used by tests and internal helpers.
type CreateAccountInput struct {
	Name                 string
	Remark               string
	PaymentMethod        string
	Email                string
	SpaceName            string
	OpenedAt             string
	CostYuan             string
	TotalCostYuan        *string
	ZeroRenewalNextMonth bool
	SeatCount            int
	SeatNames            []string
}

// UpdateAccountInput updates account metadata and optional seat capacity.
// SeatCount of 0 means "do not change seat count".
type UpdateAccountInput struct {
	Name                 string
	Remark               string
	PaymentMethod        string
	Email                string
	SpaceName            string
	OpenedAt             string
	CostYuan             string
	TotalCostYuan        *string
	ZeroRenewalNextMonth bool
	SeatCount            int
}

// CreateSeatInput adds one seat under an account.
type CreateSeatInput struct {
	Name string
}

// UpdateSeatInput renames a seat.
type UpdateSeatInput struct {
	Name string
}

// ListAccountsView returns all accounts with seat occupancy details.
func (service *SubscriptionService) ListAccountsView() ([]AccountView, error) {
	accounts, err := service.Store.ListAccounts()
	if err != nil {
		return nil, err
	}
	views := make([]AccountView, 0, len(accounts))
	for _, account := range accounts {
		view, err := service.buildAccountView(account)
		if err != nil {
			return nil, err
		}
		views = append(views, view)
	}
	return views, nil
}

// GetAccountView returns one account with seats.
func (service *SubscriptionService) GetAccountView(accountID int64) (AccountView, error) {
	account, err := service.Store.GetAccount(accountID)
	if err != nil {
		if err == sql.ErrNoRows {
			return AccountView{}, fmt.Errorf("账号不存在")
		}
		return AccountView{}, err
	}
	return service.buildAccountView(account)
}

func (service *SubscriptionService) buildAccountView(account model.Account) (AccountView, error) {
	seats, err := service.Store.ListSeatsByAccount(account.ID)
	if err != nil {
		return AccountView{}, err
	}
	seatViews := make([]SeatView, 0, len(seats))
	usedCount := 0
	for _, seat := range seats {
		seatView, err := service.buildSeatView(seat)
		if err != nil {
			return AccountView{}, err
		}
		if seatView.Occupied {
			usedCount++
		}
		seatViews = append(seatViews, seatView)
	}
	return AccountView{
		Account:   account,
		Seats:     seatViews,
		SeatTotal: len(seatViews),
		SeatUsed:  usedCount,
		IsFull:    len(seatViews) > 0 && usedCount >= len(seatViews),
		// No active occupancy: delete cascades free seats (history seat links are cleared).
		CanDelete: usedCount == 0,
	}, nil
}

func (service *SubscriptionService) buildSeatView(seat model.Seat) (SeatView, error) {
	activeSubscription, err := service.Store.GetActiveSubscriptionBySeatID(seat.ID)
	view := SeatView{
		Seat: seat,
	}
	if err == nil {
		view.Occupied = true
		view.ActiveSubscriptionID = activeSubscription.ID
		view.ActiveSubscriptionName = activeSubscription.Name
		view.ActivePriceYuan = cycle.FormatCents(activeSubscription.PricePerPersonCents)
		view.ActiveCostYuan = cycle.FormatCents(activeSubscription.CostCents)
		view.ActiveAgencyFeeYuan = cycle.FormatCents(activeSubscription.AgencyFeeCents)
		view.ActiveIsResale = activeSubscription.IsResale
		view.ActiveCronExpr = activeSubscription.CronExpr
		view.ActiveOffsetsText = cycle.FormatOffsets(activeSubscription.NotifyOffsets)
		view.ActiveRemark = activeSubscription.Remark
		view.ActiveTradeURL = activeSubscription.TradeURL
		view.ActiveCustomerEmail = activeSubscription.CustomerEmail
		view.ActiveCustomerWechat = activeSubscription.CustomerWechat
		view.ActiveAccountID = activeSubscription.AccountID
		if view.ActiveAccountID == 0 {
			view.ActiveAccountID = seat.AccountID
		}
		view.ActiveBoardedAt = activeSubscription.BoardedAt
	} else if err != sql.ErrNoRows {
		return SeatView{}, err
	}

	linkedCount, err := service.Store.CountSubscriptionsLinkedToSeat(seat.ID)
	if err != nil {
		return SeatView{}, err
	}
	view.LinkedSubscriptionCount = linkedCount
	// Free seats can be deleted; historical links are cleared on delete.
	view.CanDelete = !view.Occupied
	return view, nil
}

// CreateAccount validates and creates an account with initial seats.
func (service *SubscriptionService) CreateAccount(input CreateAccountInput) (int64, error) {
	name, err := validateAccountName(input.Name)
	if err != nil {
		return 0, err
	}
	seatNames, err := resolveInitialSeatNames(input)
	if err != nil {
		return 0, err
	}
	email, spaceName, openedAt, costCents, err := normalizeAccountDetails(
		input.Email,
		input.SpaceName,
		input.OpenedAt,
		input.CostYuan,
	)
	if err != nil {
		return 0, err
	}
	email = defaultAccountEmail(email, name)
	totalCostCents, err := normalizeOptionalAccountTotalCost(input.TotalCostYuan)
	if err != nil {
		return 0, err
	}
	initialCostCents := costCents
	if totalCostCents != nil {
		initialCostCents = *totalCostCents
	}

	accountID, err := service.Store.CreateAccount(model.Account{
		Name:                 name,
		Remark:               strings.TrimSpace(input.Remark),
		PaymentMethod:        strings.TrimSpace(input.PaymentMethod),
		Email:                email,
		SpaceName:            spaceName,
		OpenedAt:             openedAt,
		CostCents:            costCents,
		ZeroRenewalNextMonth: input.ZeroRenewalNextMonth,
	}, initialCostCents, cycle.FormatDate(service.now()))
	if err != nil {
		return 0, err
	}
	for _, seatName := range seatNames {
		if _, err := service.Store.CreateSeat(model.Seat{
			AccountID: accountID,
			Name:      seatName,
		}); err != nil {
			return 0, err
		}
	}
	return accountID, nil
}

// UpdateAccount updates account name, remark, and optional seat capacity.
// When SeatCount > 0, seat rows are resized to that count (cannot shrink below active occupancy).
func (service *SubscriptionService) UpdateAccount(accountID int64, input UpdateAccountInput) error {
	if _, err := service.Store.GetAccount(accountID); err != nil {
		if err == sql.ErrNoRows {
			return fmt.Errorf("账号不存在")
		}
		return err
	}
	name, err := validateAccountName(input.Name)
	if err != nil {
		return err
	}
	email, spaceName, openedAt, costCents, err := normalizeAccountDetails(
		input.Email,
		input.SpaceName,
		input.OpenedAt,
		input.CostYuan,
	)
	if err != nil {
		return err
	}
	email = defaultAccountEmail(email, name)
	totalCostCents, err := normalizeOptionalAccountTotalCost(input.TotalCostYuan)
	if err != nil {
		return err
	}
	if err := service.Store.UpdateAccount(model.Account{
		ID:                   accountID,
		Name:                 name,
		Remark:               strings.TrimSpace(input.Remark),
		PaymentMethod:        strings.TrimSpace(input.PaymentMethod),
		Email:                email,
		SpaceName:            spaceName,
		OpenedAt:             openedAt,
		CostCents:            costCents,
		ZeroRenewalNextMonth: input.ZeroRenewalNextMonth,
	}, totalCostCents, cycle.FormatDate(service.now())); err != nil {
		return err
	}
	if input.SeatCount > 0 {
		if err := service.resizeAccountSeats(accountID, input.SeatCount); err != nil {
			return err
		}
	}
	return nil
}

// resizeAccountSeats sets the number of seat rows under an account.
// Growing appends auto-named free seats; shrinking only removes free seats.
func (service *SubscriptionService) resizeAccountSeats(accountID int64, targetCount int) error {
	if targetCount < model.MinInitialSeatCount || targetCount > model.MaxInitialSeatCount {
		return fmt.Errorf("车位数量须为 %d～%d 的整数", model.MinInitialSeatCount, model.MaxInitialSeatCount)
	}
	seats, err := service.Store.ListSeatsByAccount(accountID)
	if err != nil {
		return err
	}
	currentCount := len(seats)
	if targetCount == currentCount {
		return nil
	}

	occupiedIDs := map[int64]struct{}{}
	for _, seat := range seats {
		_, err := service.Store.GetActiveSubscriptionBySeatID(seat.ID)
		if err == nil {
			occupiedIDs[seat.ID] = struct{}{}
			continue
		}
		if err != sql.ErrNoRows {
			return err
		}
	}
	usedCount := len(occupiedIDs)
	if targetCount < usedCount {
		return fmt.Errorf("车位数量不能少于当前占用数 %d", usedCount)
	}

	if targetCount > currentCount {
		// Prefer continuing 车位N numbering after the highest existing numeric suffix when possible.
		nextIndex := currentCount + 1
		for index := 0; index < targetCount-currentCount; index++ {
			seatName := fmt.Sprintf("车位%d", nextIndex+index)
			if _, err := service.Store.CreateSeat(model.Seat{
				AccountID: accountID,
				Name:      seatName,
			}); err != nil {
				return err
			}
		}
		return nil
	}

	// Shrink: delete free seats from the end so occupied seats stay put.
	toRemove := currentCount - targetCount
	for index := len(seats) - 1; index >= 0 && toRemove > 0; index-- {
		seat := seats[index]
		if _, occupied := occupiedIDs[seat.ID]; occupied {
			continue
		}
		if err := service.DeleteSeat(seat.ID); err != nil {
			return err
		}
		toRemove--
	}
	if toRemove > 0 {
		return fmt.Errorf("可释放的空闲车位不足，无法缩减到 %d", targetCount)
	}
	return nil
}

// DeleteAccount removes an account that has no active seat occupancy.
// Free seats are removed first; historical seat links on archived/soft-deleted
// subscriptions are cleared so bills keep subscription_id only.
func (service *SubscriptionService) DeleteAccount(accountID int64) error {
	if _, err := service.Store.GetAccount(accountID); err != nil {
		if err == sql.ErrNoRows {
			return fmt.Errorf("账号不存在")
		}
		return err
	}
	activeCount, err := service.Store.CountActiveSubscriptionsByAccount(accountID)
	if err != nil {
		return err
	}
	if activeCount > 0 {
		return fmt.Errorf("该账号仍有活跃订阅占用车位，请先下车后再删除")
	}
	afterSalesCount, err := service.Store.CountAfterSalesCasesByAccount(accountID)
	if err != nil {
		return err
	}
	if afterSalesCount > 0 {
		return fmt.Errorf("该账号已有售后退款记录，为保留历史凭据不可删除")
	}
	seats, err := service.Store.ListSeatsByAccount(accountID)
	if err != nil {
		return err
	}
	for _, seat := range seats {
		if err := service.DeleteSeat(seat.ID); err != nil {
			return err
		}
	}
	return service.Store.DeleteAccount(accountID)
}

// CreateSeat adds a named seat under an account.
func (service *SubscriptionService) CreateSeat(accountID int64, input CreateSeatInput) (int64, error) {
	if _, err := service.Store.GetAccount(accountID); err != nil {
		if err == sql.ErrNoRows {
			return 0, fmt.Errorf("账号不存在")
		}
		return 0, err
	}
	seatName := strings.TrimSpace(input.Name)
	if err := validateSeatName(seatName); err != nil {
		return 0, err
	}
	return service.Store.CreateSeat(model.Seat{
		AccountID: accountID,
		Name:      seatName,
	})
}

// UpdateSeat renames a seat.
func (service *SubscriptionService) UpdateSeat(seatID int64, input UpdateSeatInput) error {
	if _, err := service.Store.GetSeat(seatID); err != nil {
		if err == sql.ErrNoRows {
			return fmt.Errorf("车位不存在")
		}
		return err
	}
	seatName := strings.TrimSpace(input.Name)
	if err := validateSeatName(seatName); err != nil {
		return err
	}
	return service.Store.UpdateSeat(model.Seat{
		ID:   seatID,
		Name: seatName,
	})
}

// DeleteSeat removes a free seat. Historical subscriptions keep their bills via
// subscription_id; seat_id is cleared so the seat row can be dropped.
func (service *SubscriptionService) DeleteSeat(seatID int64) error {
	if _, err := service.Store.GetSeat(seatID); err != nil {
		if err == sql.ErrNoRows {
			return fmt.Errorf("车位不存在")
		}
		return err
	}
	active, err := service.Store.GetActiveSubscriptionBySeatID(seatID)
	if err == nil && active.ID > 0 {
		return fmt.Errorf("该车位已有活跃订阅，无法删除")
	}
	if err != nil && err != sql.ErrNoRows {
		return err
	}
	if err := service.Store.ClearSeatLinksForSeat(seatID); err != nil {
		return err
	}
	return service.Store.DeleteSeat(seatID)
}

// SeatOption is a selectable seat for subscription forms.
type SeatOption struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	AccountID int64  `json:"account_id"`
	Free      bool   `json:"free"`
}

// AccountOption is a selectable account with free/all seats for forms.
type AccountOption struct {
	ID                   int64        `json:"id"`
	Name                 string       `json:"name"`
	Remark               string       `json:"remark"`
	PaymentMethod        string       `json:"payment_method"`
	Email                string       `json:"email"`
	SpaceName            string       `json:"space_name"`
	OpenedAt             string       `json:"opened_at"`
	CostYuan             string       `json:"cost_yuan"`
	TotalCostYuan        string       `json:"total_cost_yuan"`
	ZeroRenewalNextMonth bool         `json:"zero_renewal_next_month"`
	SeatTotal            int          `json:"seat_total"`
	SeatUsed             int          `json:"seat_used"`
	IsFull               bool         `json:"is_full"`
	Seats                []SeatOption `json:"seats"`
}

// ListAccountOptionsForForm returns accounts and seats for the subscription dialog.
// includeSeatID keeps the currently assigned seat visible when editing.
func (service *SubscriptionService) ListAccountOptionsForForm(includeSeatID int64) ([]AccountOption, error) {
	accounts, err := service.Store.ListAccounts()
	if err != nil {
		return nil, err
	}
	options := make([]AccountOption, 0, len(accounts))
	for _, account := range accounts {
		allSeats, err := service.Store.ListSeatsByAccount(account.ID)
		if err != nil {
			return nil, err
		}
		includesCurrentSeat := false
		for _, seat := range allSeats {
			if seat.ID == includeSeatID {
				includesCurrentSeat = true
				break
			}
		}
		if account.BannedAt != "" && !includesCurrentSeat {
			continue
		}
		freeSeats, err := service.Store.ListFreeSeats(account.ID, includeSeatID)
		if err != nil {
			return nil, err
		}
		freeSet := map[int64]struct{}{}
		for _, seat := range freeSeats {
			freeSet[seat.ID] = struct{}{}
		}

		seatOptions := make([]SeatOption, 0, len(allSeats))
		usedCount := 0
		for _, seat := range allSeats {
			_, isFree := freeSet[seat.ID]
			if !isFree {
				usedCount++
			}
			// For create/edit, only list free seats (plus includeSeatID which is in freeSet).
			if !isFree {
				continue
			}
			seatOptions = append(seatOptions, SeatOption{
				ID:        seat.ID,
				Name:      seat.Name,
				AccountID: account.ID,
				Free:      true,
			})
		}
		options = append(options, AccountOption{
			ID:                   account.ID,
			Name:                 account.Name,
			Remark:               account.Remark,
			PaymentMethod:        account.PaymentMethod,
			Email:                account.Email,
			SpaceName:            account.SpaceName,
			OpenedAt:             account.OpenedAt,
			CostYuan:             cycle.FormatCents(account.CostCents),
			TotalCostYuan:        cycle.FormatCents(account.TotalCostCents),
			ZeroRenewalNextMonth: account.ZeroRenewalNextMonth,
			SeatTotal:            len(allSeats),
			SeatUsed:             usedCount,
			IsFull:               len(allSeats) > 0 && usedCount >= len(allSeats) && includeSeatID == 0,
			Seats:                seatOptions,
		})
	}
	return options, nil
}

func validateAccountName(raw string) (string, error) {
	name := strings.TrimSpace(raw)
	if name == "" {
		return "", fmt.Errorf("账号名称不能为空")
	}
	if len([]rune(name)) > model.MaxAccountNameLength {
		return "", fmt.Errorf("账号名称最多 %d 个字", model.MaxAccountNameLength)
	}
	return name, nil
}

func validateSeatName(name string) error {
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("车位名称不能为空")
	}
	if len([]rune(name)) > model.MaxSeatNameLength {
		return fmt.Errorf("车位名称最多 %d 个字", model.MaxSeatNameLength)
	}
	return nil
}

func normalizeAccountDetails(emailRaw, spaceNameRaw, openedAtRaw, costYuanRaw string) (string, string, string, int64, error) {
	email, err := normalizeAccountEmail(emailRaw)
	if err != nil {
		return "", "", "", 0, err
	}
	openedAt, err := normalizeAccountOpenedAt(openedAtRaw)
	if err != nil {
		return "", "", "", 0, err
	}
	costCents := int64(0)
	if strings.TrimSpace(costYuanRaw) != "" {
		costCents, err = cycle.ParseYuanToCents(costYuanRaw)
		if err != nil {
			return "", "", "", 0, fmt.Errorf("账号成本价无效: %w", err)
		}
	}
	return email, strings.TrimSpace(spaceNameRaw), openedAt, costCents, nil
}

func normalizeOptionalAccountTotalCost(raw *string) (*int64, error) {
	if raw == nil || strings.TrimSpace(*raw) == "" {
		return nil, nil
	}
	totalCostCents, err := cycle.ParseYuanToCents(*raw)
	if err != nil {
		return nil, fmt.Errorf("账号累计总成本无效: %w", err)
	}
	return &totalCostCents, nil
}

func normalizeAccountEmail(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", nil
	}
	address, err := mail.ParseAddress(raw)
	if err != nil {
		return "", fmt.Errorf("账号邮箱无效")
	}
	return strings.TrimSpace(address.Address), nil
}

func defaultAccountEmail(email string, accountName string) string {
	if email != "" {
		return email
	}
	address, err := mail.ParseAddress(strings.TrimSpace(accountName))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(address.Address)
}

func normalizeAccountOpenedAt(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", nil
	}
	parsed, err := time.ParseInLocation("2006-01-02", raw, cycle.Location)
	if err != nil {
		return "", fmt.Errorf("开通日期无效，请使用 YYYY-MM-DD")
	}
	return cycle.FormatDate(parsed), nil
}

// resolveInitialSeatNames builds the seat list for a new account.
// SeatNames wins when non-empty (tests/helpers); otherwise SeatCount generates 车位1…N.
func resolveInitialSeatNames(input CreateAccountInput) ([]string, error) {
	if names := normalizeSeatNames(input.SeatNames); len(names) > 0 {
		if len(names) > model.MaxInitialSeatCount {
			return nil, fmt.Errorf("初始车位数量须为 %d～%d", model.MinInitialSeatCount, model.MaxInitialSeatCount)
		}
		for _, seatName := range names {
			if err := validateSeatName(seatName); err != nil {
				return nil, err
			}
		}
		return names, nil
	}
	count := input.SeatCount
	if count < model.MinInitialSeatCount || count > model.MaxInitialSeatCount {
		return nil, fmt.Errorf("初始车位数量须为 %d～%d 的整数", model.MinInitialSeatCount, model.MaxInitialSeatCount)
	}
	names := make([]string, 0, count)
	for index := 1; index <= count; index++ {
		names = append(names, fmt.Sprintf("车位%d", index))
	}
	return names, nil
}

func normalizeSeatNames(rawNames []string) []string {
	result := make([]string, 0, len(rawNames))
	seen := map[string]struct{}{}
	for _, raw := range rawNames {
		name := strings.TrimSpace(raw)
		if name == "" {
			continue
		}
		if _, exists := seen[name]; exists {
			continue
		}
		seen[name] = struct{}{}
		result = append(result, name)
	}
	return result
}
