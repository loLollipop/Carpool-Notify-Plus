package service

import (
	"fmt"
	"sort"

	"carpool-notify/internal/model"
)

// SetAccountDisplaySerial anchors one logical account and numbers subsequent
// accounts in import order. Secondary spaces keep their primary owner's number.
func (service *SubscriptionService) SetAccountDisplaySerial(accountID, serial, expectedSerial int64) error {
	if serial < 1 || serial > 9007199254740991 {
		return fmt.Errorf("序号必须是有效的正整数")
	}
	accounts, err := service.Store.ListAccounts()
	if err != nil {
		return err
	}
	identities := newAccountIdentityIndex(accounts)
	if identities.identity(accountID).Serial != expectedSerial || expectedSerial <= 0 {
		return fmt.Errorf("账号序号已变化，请刷新后重试")
	}
	if identities.identity(accountID).Role == AccountSpaceRoleSecondary {
		return fmt.Errorf("副空间共用主空间序号，请调整主空间")
	}
	candidate := append([]model.Account(nil), accounts...)
	found := false
	for index := range candidate {
		if candidate[index].ID == accountID {
			candidate[index].DisplaySerialAnchor = serial
			found = true
		}
	}
	if !found {
		return fmt.Errorf("账号不存在")
	}
	updated := newAccountIdentityIndex(candidate)
	sort.Slice(candidate, func(left, right int) bool { return candidate[left].ID < candidate[right].ID })
	serialOwners := make(map[int64]int64, len(candidate))
	var lastSerial int64
	for _, account := range candidate {
		identity := updated.identity(account.ID)
		if identity.Role == AccountSpaceRoleSecondary {
			continue
		}
		if identity.Serial <= 0 || identity.Serial > 9007199254740991 {
			return fmt.Errorf("后续账号序号超出有效范围")
		}
		if owner, exists := serialOwners[identity.Serial]; exists {
			return fmt.Errorf("序号 %d 与账号 %d 重复，请调整编号基准", identity.Serial, owner)
		}
		if identity.Serial <= lastSerial {
			return fmt.Errorf("编号基准与其他账号的顺序冲突，请选择更大的序号")
		}
		serialOwners[identity.Serial] = account.ID
		lastSerial = identity.Serial
	}
	return service.Store.SetAccountDisplaySerialAnchor(accountID, serial, accounts)
}
