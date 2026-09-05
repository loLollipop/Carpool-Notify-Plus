package service

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"carpool-notify/internal/model"
)

const MaxAdminDisplayNameLength = 40

// GetAdminProfile returns the operator identity used by the management shell.
func (service *SubscriptionService) GetAdminProfile() (model.AdminProfile, error) {
	raw, err := service.Store.GetSetting(model.SettingAdminProfile)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && strings.TrimSpace(raw) == "") {
		return model.DefaultAdminProfile, nil
	}
	if err != nil {
		return model.AdminProfile{}, err
	}

	var profile model.AdminProfile
	if err := json.Unmarshal([]byte(raw), &profile); err != nil {
		return model.AdminProfile{}, fmt.Errorf("decode admin profile: %w", err)
	}
	return normalizeAdminProfile(profile)
}

// SaveAdminProfile validates and persists the operator display identity.
func (service *SubscriptionService) SaveAdminProfile(input model.AdminProfile) (model.AdminProfile, error) {
	profile, err := normalizeAdminProfile(input)
	if err != nil {
		return model.AdminProfile{}, err
	}
	encoded, err := json.Marshal(profile)
	if err != nil {
		return model.AdminProfile{}, fmt.Errorf("encode admin profile: %w", err)
	}
	if err := service.Store.SetSetting(model.SettingAdminProfile, string(encoded)); err != nil {
		return model.AdminProfile{}, err
	}
	return profile, nil
}

func normalizeAdminProfile(input model.AdminProfile) (model.AdminProfile, error) {
	input.DisplayName = strings.TrimSpace(input.DisplayName)
	if input.DisplayName == "" {
		return model.AdminProfile{}, fmt.Errorf("管理员名称不能为空")
	}
	if utf8.RuneCountInString(input.DisplayName) > MaxAdminDisplayNameLength {
		return model.AdminProfile{}, fmt.Errorf("管理员名称最多 %d 个字", MaxAdminDisplayNameLength)
	}
	return input, nil
}
