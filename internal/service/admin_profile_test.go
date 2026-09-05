package service

import (
	"path/filepath"
	"strings"
	"testing"

	"carpool-notify/internal/db"
	"carpool-notify/internal/model"
)

func TestAdminProfileDefaultsAndPersists(t *testing.T) {
	store, err := db.Open(filepath.Join(t.TempDir(), "profile.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	service := &SubscriptionService{Store: store}

	profile, err := service.GetAdminProfile()
	if err != nil {
		t.Fatal(err)
	}
	if profile != model.DefaultAdminProfile {
		t.Fatalf("default profile = %#v", profile)
	}

	profile, err = service.SaveAdminProfile(model.AdminProfile{DisplayName: "  Lollipop  "})
	if err != nil {
		t.Fatal(err)
	}
	if profile.DisplayName != "Lollipop" {
		t.Fatalf("saved display name = %q", profile.DisplayName)
	}
	stored, err := service.GetAdminProfile()
	if err != nil {
		t.Fatal(err)
	}
	if stored != profile {
		t.Fatalf("stored profile = %#v, want %#v", stored, profile)
	}
}

func TestAdminProfileRejectsInvalidNames(t *testing.T) {
	service := &SubscriptionService{}
	for _, displayName := range []string{"   ", strings.Repeat("名", MaxAdminDisplayNameLength+1)} {
		if _, err := service.SaveAdminProfile(model.AdminProfile{DisplayName: displayName}); err == nil {
			t.Fatalf("SaveAdminProfile(%q) unexpectedly succeeded", displayName)
		}
	}
}
