package service

import (
	"crypto/rand"
	"crypto/subtle"
	"database/sql"
	"encoding/base64"
	"fmt"
	"strings"
	"time"

	"carpool-notify/internal/cycle"
	"carpool-notify/internal/model"
)

const (
	sandboxTokenSetting    = "sandbox_access_token"
	sandboxSeededAtSetting = "sandbox_seeded_at"
)

// SandboxAccount describes one fixture account and its intended rehearsal.
type SandboxAccount struct {
	ID      int64  `json:"id"`
	Name    string `json:"name"`
	Purpose string `json:"purpose"`
}

// SandboxStatus is the operator-facing state of the isolated rehearsal data.
type SandboxStatus struct {
	Ready             bool             `json:"ready"`
	AccessToken       string           `json:"access_token"`
	SeededAt          string           `json:"seeded_at"`
	RedemptionCodes   []string         `json:"redemption_codes"`
	Accounts          []SandboxAccount `json:"accounts"`
	SubscriptionCount int              `json:"subscription_count"`
}

// ResetSandboxFixtures clears only the rehearsal database and seeds complete,
// realistic workflows for redemption, cancellation, account bans and after-sales.
func (service *SubscriptionService) ResetSandboxFixtures() (SandboxStatus, error) {
	if err := service.Store.SetSetting(sandboxTokenSetting, ""); err != nil {
		return SandboxStatus{}, err
	}
	if err := service.Store.SetSetting(sandboxSeededAtSetting, ""); err != nil {
		return SandboxStatus{}, err
	}
	if err := service.Store.ResetBusinessData(); err != nil {
		return SandboxStatus{}, err
	}

	now := service.now()
	openedAt := cycle.FormatDate(now.AddDate(0, 0, -20))
	createAccount := func(name, email, spaceName, purpose string, seatCount int) (SandboxAccount, error) {
		accountID, err := service.CreateAccount(CreateAccountInput{
			Name:          name,
			Remark:        "业务演练数据，可随时在系统工具中重置",
			PaymentMethod: "沙盒演练",
			Email:         email,
			SpaceName:     spaceName,
			OpenedAt:      openedAt,
			CostYuan:      "20.00",
			SeatCount:     seatCount,
		})
		if err != nil {
			return SandboxAccount{}, err
		}
		return SandboxAccount{ID: accountID, Name: name, Purpose: purpose}, nil
	}

	redeemAccount, err := createAccount(
		"沙盒兑换主号",
		"sandbox-redeem@example.com",
		"兑换流程演练空间",
		"处理兑换申请并分配空间",
		5,
	)
	if err != nil {
		return SandboxStatus{}, fmt.Errorf("seed redemption account: %w", err)
	}
	cancelAccount, err := createAccount(
		"沙盒退订主号",
		"sandbox-cancel@example.com",
		"提前退订演练空间",
		"对客户发起提前退订并完成退款",
		5,
	)
	if err != nil {
		return SandboxStatus{}, fmt.Errorf("seed cancellation account: %w", err)
	}
	banAccount, err := createAccount(
		"沙盒封禁主号",
		"sandbox-banned@example.com",
		"账号封禁演练空间",
		"封禁母号并批量生成售后事项",
		5,
	)
	if err != nil {
		return SandboxStatus{}, fmt.Errorf("seed banned account: %w", err)
	}
	replacementAccount, err := createAccount(
		"沙盒备用主号",
		"sandbox-replacement@example.com",
		"售后换车演练空间",
		"接收选择换空间的售后客户",
		5,
	)
	if err != nil {
		return SandboxStatus{}, fmt.Errorf("seed replacement account: %w", err)
	}

	createPaidCustomer := func(
		name, email, wechat string,
		accountID int64,
		boardedDaysAgo int,
		price string,
	) error {
		_, err := service.CreateWithInitialBill(CreateInput{
			Name:             name,
			PriceYuan:        price,
			CronExpr:         "interval:30d",
			NotifyOffsetsRaw: "3",
			Remark:           "沙盒演练客户",
			CustomerEmail:    email,
			CustomerWechat:   wechat,
			AccountID:        accountID,
			BoardedAt:        cycle.FormatDate(now.AddDate(0, 0, -boardedDaysAgo)),
		})
		return err
	}

	if err := createPaidCustomer(
		"提前退订演练客户",
		"cancel-demo@example.com",
		"sandbox_cancel_demo",
		cancelAccount.ID,
		10,
		"30.00",
	); err != nil {
		return SandboxStatus{}, fmt.Errorf("seed cancellation customer: %w", err)
	}
	if err := createPaidCustomer(
		"封禁售后演练客户 A",
		"ban-demo-a@example.com",
		"sandbox_ban_a",
		banAccount.ID,
		8,
		"28.00",
	); err != nil {
		return SandboxStatus{}, fmt.Errorf("seed banned customer A: %w", err)
	}
	if err := createPaidCustomer(
		"封禁售后演练客户 B",
		"ban-demo-b@example.com",
		"sandbox_ban_b",
		banAccount.ID,
		18,
		"35.00",
	); err != nil {
		return SandboxStatus{}, fmt.Errorf("seed banned customer B: %w", err)
	}

	codes, err := service.GenerateRedemptionCodes(RedemptionCodeGenerateInput{
		Count: 3,
		Note:  "沙盒兑换流程测试",
	})
	if err != nil {
		return SandboxStatus{}, fmt.Errorf("seed redemption codes: %w", err)
	}
	if len(codes) == 0 {
		return SandboxStatus{}, fmt.Errorf("seed redemption codes: no codes created")
	}

	accessToken, err := newSandboxAccessToken()
	if err != nil {
		return SandboxStatus{}, err
	}
	seededAt := now.Format(time.RFC3339)
	if err := service.Store.SetSetting(sandboxTokenSetting, accessToken); err != nil {
		return SandboxStatus{}, err
	}
	if err := service.Store.SetSetting(sandboxSeededAtSetting, seededAt); err != nil {
		return SandboxStatus{}, err
	}

	return SandboxStatus{
		Ready:             true,
		AccessToken:       accessToken,
		SeededAt:          seededAt,
		RedemptionCodes:   sandboxCodeStrings(codes),
		Accounts:          []SandboxAccount{redeemAccount, cancelAccount, banAccount, replacementAccount},
		SubscriptionCount: 3,
	}, nil
}

// GetSandboxStatus reports persisted fixture state after process restarts.
func (service *SubscriptionService) GetSandboxStatus() (SandboxStatus, error) {
	accessToken, err := service.Store.GetSetting(sandboxTokenSetting)
	if err == sql.ErrNoRows || strings.TrimSpace(accessToken) == "" {
		return SandboxStatus{Ready: false}, nil
	}
	if err != nil {
		return SandboxStatus{}, err
	}
	seededAt, err := service.Store.GetSetting(sandboxSeededAtSetting)
	if err != nil && err != sql.ErrNoRows {
		return SandboxStatus{}, err
	}

	codeViews, err := service.ListRedemptionCodesView()
	if err != nil {
		return SandboxStatus{}, err
	}
	accounts, err := service.ListAccountsView()
	if err != nil {
		return SandboxStatus{}, err
	}
	subscriptions, err := service.ListView()
	if err != nil {
		return SandboxStatus{}, err
	}

	status := SandboxStatus{
		Ready:             true,
		AccessToken:       strings.TrimSpace(accessToken),
		SeededAt:          strings.TrimSpace(seededAt),
		RedemptionCodes:   sandboxCodeStrings(codeViews),
		SubscriptionCount: len(subscriptions),
	}
	purposes := map[string]string{
		"沙盒兑换主号": "处理兑换申请并分配空间",
		"沙盒退订主号": "对客户发起提前退订并完成退款",
		"沙盒封禁主号": "封禁母号并批量生成售后事项",
		"沙盒备用主号": "接收选择换空间的售后客户",
	}
	for _, account := range accounts {
		status.Accounts = append(status.Accounts, SandboxAccount{
			ID:      account.Account.ID,
			Name:    account.Account.Name,
			Purpose: purposes[account.Account.Name],
		})
	}
	return status, nil
}

// ValidateSandboxAccessToken protects public rehearsal redemption endpoints.
func (service *SubscriptionService) ValidateSandboxAccessToken(candidate string) (bool, error) {
	stored, err := service.Store.GetSetting(sandboxTokenSetting)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	stored = strings.TrimSpace(stored)
	candidate = strings.TrimSpace(candidate)
	if stored == "" || candidate == "" || len(stored) != len(candidate) {
		return false, nil
	}
	return subtle.ConstantTimeCompare([]byte(stored), []byte(candidate)) == 1, nil
}

func sandboxCodeStrings(views []RedemptionCodeView) []string {
	codes := make([]string, 0, len(views))
	for _, view := range views {
		if view.Code.Status == model.RedemptionCodeStatusUnused {
			codes = append(codes, view.Code.Code)
		}
	}
	return codes
}

func newSandboxAccessToken() (string, error) {
	raw := make([]byte, 24)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("generate sandbox token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}
