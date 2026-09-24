package service

import "carpool-notify/internal/model"

// accountIdentityIndex resolves every display surface from the same account
// snapshot. Login email and persisted historical snapshots remain unchanged.
type accountDisplayIdentity struct {
	Serial int64
	Email  string
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
	for _, account := range accounts {
		index.accounts[account.ID] = account
		index.identities[account.ID] = accountDisplayIdentity{Serial: serials[account.ID], Email: accountDisplayEmail(account)}
	}
	return index
}

func (index accountIdentityIndex) identity(accountID int64) accountDisplayIdentity {
	if identity, exists := index.identities[accountID]; exists {
		return identity
	}
	return accountDisplayIdentity{Serial: accountID}
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
