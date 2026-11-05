package service

import (
	"path/filepath"
	"testing"

	"carpool-notify/internal/db"
	"carpool-notify/internal/model"
)

func TestAccountDisplaySerialAnchorRenumbersOnlyFollowingLogicalAccounts(t *testing.T) {
	accounts := []model.Account{
		{ID: 60, Email: "after-gap@example.test"},
		{ID: 49, Email: "before@example.test"},
		{ID: 48, Email: "next@example.test", Remark: "source@example.test(99)"},
		{ID: 47, Email: "RosendoFramixi@outlook.com", DisplaySerialAnchor: 47},
		{ID: 46, Email: "before@example.test"},
		{ID: 61, Email: "rosendoframixi@outlook.com"},
	}
	identities := newAccountIdentityIndex(accounts)
	wants := map[int64]int64{46: 46, 47: 47, 48: 48, 49: 46, 60: 49, 61: 47}
	for id, want := range wants {
		if got := identities.identity(id).Serial; got != want {
			t.Errorf("serial for %d = %d, want %d", id, got, want)
		}
	}
	if identities.identity(61).Role != AccountSpaceRoleSecondary {
		t.Fatal("anchor must not change the secondary space role")
	}
}

func TestAccountDisplaySerialAnchorPersistsAndRejectsCollisions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "anchors.db")
	store, err := db.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	svc := &SubscriptionService{Store: store}
	first, err := svc.CreateAccount(CreateAccountInput{Name: "first@example.test", SeatCount: 1})
	if err != nil {
		t.Fatal(err)
	}
	anchor, err := svc.CreateAccount(CreateAccountInput{Name: "RosendoFramixi@outlook.com", Remark: "source@example.test（999）", SeatCount: 1})
	if err != nil {
		t.Fatal(err)
	}
	next, err := svc.CreateAccount(CreateAccountInput{Name: "next@example.test", SeatCount: 1})
	if err != nil {
		t.Fatal(err)
	}
	secondary, err := svc.CreateAccount(CreateAccountInput{Name: "rosendoframixi@outlook.com", SeatCount: 1})
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.SetAccountDisplaySerial(anchor, 47, anchor); err != nil {
		t.Fatal(err)
	}
	for id, want := range map[int64]int64{first: first, anchor: 47, next: 48, secondary: 47} {
		view, err := svc.GetAccountView(id)
		if err != nil || view.DisplaySerial != want {
			t.Fatalf("view %d serial = %d, want %d; %v", id, view.DisplaySerial, want, err)
		}
	}
	if err := svc.SetAccountDisplaySerial(anchor, 50, anchor); err == nil {
		t.Fatal("stale serial accepted")
	}
	if err := svc.SetAccountDisplaySerial(anchor, first, 47); err == nil {
		t.Fatal("collision accepted")
	}
	if err := svc.SetAccountDisplaySerial(secondary, 49, 47); err == nil {
		t.Fatal("secondary override accepted")
	}
	if err := svc.SetAccountDisplaySerial(anchor, 0, 47); err == nil {
		t.Fatal("zero accepted")
	}
	stored, err := store.GetAccount(anchor)
	if err != nil || stored.DisplaySerialAnchor != 47 || stored.Remark != "source@example.test（999）" {
		t.Fatalf("anchor or remark not retained: %#v, %v", stored, err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := db.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	svc.Store = reopened
	view, err := svc.GetAccountView(next)
	if err != nil || view.DisplaySerial != 48 {
		t.Fatalf("anchor lost after reopen: %d %v", view.DisplaySerial, err)
	}
}
