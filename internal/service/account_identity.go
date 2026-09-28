package service

import (
	"strings"

	"carpool-notify/internal/model"
)

// AccountSpaceRole describes an account's position among Team spaces that
// share the same effective owner email.
type AccountSpaceRole string

const (
	AccountSpaceRoleStandalone AccountSpaceRole = "standalone"
	AccountSpaceRolePrimary    AccountSpaceRole = "primary"
	AccountSpaceRoleSecondary  AccountSpaceRole = "secondary"
)

// accountIdentityIndex resolves every display surface from the same account
// snapshot. Login email and persisted historical snapshots remain unchanged.
type accountDisplayIdentity struct {
	Serial int64
	Email  string
	Role   AccountSpaceRole
}

func (identity accountDisplayIdentity) roleForBusinessType(businessType string) AccountSpaceRole {
	if strings.EqualFold(strings.TrimSpace(businessType), model.SubscriptionBusinessPlus) {
		return AccountSpaceRoleStandalone
	}
	return identity.Role
}

type accountIdentityIndex struct {
	accounts   map[int64]model.Account
	identities map[int64]accountDisplayIdentity
}

func newAccountIdentityIndex(accounts []model.Account) accountIdentityIndex {
	index := accountIdentityIndex{
		accounts:   make(map[int64]model.Account, len(accounts)),
		identities: make(map[int64]accountDisplayIdentity, len(accounts)),
	}
	serials := accountDisplaySerials(accounts)
	groupCounts := make(map[string]int, len(accounts))
	primaryByGroup := make(map[string]int64, len(accounts))
	for _, account := range accounts {
		groupKey := strings.ToLower(strings.TrimSpace(accountGroupingEmail(account)))
		if groupKey == "" {
			continue
		}
		groupCounts[groupKey]++
		if primaryID, exists := primaryByGroup[groupKey]; !exists || account.ID < primaryID {
			primaryByGroup[groupKey] = account.ID
		}
	}
	for _, account := range accounts {
		index.accounts[account.ID] = account
		role := AccountSpaceRoleStandalone
		groupKey := strings.ToLower(strings.TrimSpace(accountGroupingEmail(account)))
		if groupKey != "" && groupCounts[groupKey] > 1 {
			role = AccountSpaceRoleSecondary
			if primaryByGroup[groupKey] == account.ID {
				role = AccountSpaceRolePrimary
			}
		}
		index.identities[account.ID] = accountDisplayIdentity{
			Serial: serials[account.ID],
			Email:  accountDisplayEmail(account),
			Role:   role,
		}
	}
	return index
}

func (index accountIdentityIndex) identity(accountID int64) accountDisplayIdentity {
	if identity, exists := index.identities[accountID]; exists {
		return identity
	}
	return accountDisplayIdentity{Serial: accountID, Role: AccountSpaceRoleStandalone}
}

// Optional supplied snapshots let single-item and batch builders share one
// implementation without loading accounts for every row in a list.
func (service *SubscriptionService) accountIdentities(supplied ...accountIdentityIndex) (accountIdentityIndex, error) {
	if len(supplied) > 0 {
		return supplied[0], nil
	}
	accounts, err := service.Store.ListAccounts()
	if err != nil {
		return accountIdentityIndex{}, err
	}
	return newAccountIdentityIndex(accounts), nil
}
