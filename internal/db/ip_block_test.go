package db

import (
	"fmt"
	"path/filepath"
	"testing"
	"time"
)

func TestIPBlocksPersistAndFilterExpiration(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ip-blocks.db")
	now := time.Date(2026, time.October, 17, 12, 0, 0, 0, time.FixedZone("test", 8*60*60))
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.PutIPBlock("192.0.2.1", now.Add(time.Hour), "persistent", now); err != nil {
		t.Fatal(err)
	}
	if err := store.PutIPBlock("2001:db8::/32", now, "expires at boundary", now.Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	store, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	blocks, err := store.ListActiveIPBlocks(now)
	if err != nil {
		t.Fatal(err)
	}
	if len(blocks) != 1 || blocks[0].ClientKey != "192.0.2.1" || blocks[0].Reason != "persistent" || !blocks[0].BlockedUntil.Equal(now.Add(time.Hour).UTC()) {
		t.Fatalf("unexpected active blocks: %#v", blocks)
	}
	if err := store.PutIPBlock("new", now.Add(time.Minute), "new", now); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := store.database.QueryRow(`SELECT COUNT(*) FROM security_ip_blocks WHERE client_key = '2001:db8::/32'`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("expired row was not cleaned by PutIPBlock: count=%d", count)
	}
}

func TestIPBlockDoesNotShortenAndValidatesBoundaries(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "ip-block-boundaries.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	now := time.Date(2026, time.October, 17, 12, 0, 0, 0, time.UTC)
	long := now.Add(time.Hour)
	if err := store.PutIPBlock("key", long, "first", now); err != nil {
		t.Fatal(err)
	}
	if err := store.PutIPBlock("key", now.Add(time.Minute), "updated reason", now); err != nil {
		t.Fatal(err)
	}
	blocks, err := store.ListActiveIPBlocks(now)
	if err != nil {
		t.Fatal(err)
	}
	if len(blocks) != 1 || !blocks[0].BlockedUntil.Equal(long) || blocks[0].Reason != "updated reason" {
		t.Fatalf("existing expiration should not shrink; got %#v", blocks)
	}
	if err := store.PutIPBlock("", long, "", now); err == nil {
		t.Fatal("empty key was accepted")
	}
	if err := store.PutIPBlock(" ", long, "", now); err == nil {
		t.Fatal("whitespace-only key was accepted")
	}
	if err := store.PutIPBlock("expired", now, "", now); err == nil {
		t.Fatal("expiration equal to now was accepted")
	}
	if err := store.PutIPBlock("expired", now.Add(-time.Nanosecond), "", now); err == nil {
		t.Fatal("past expiration was accepted")
	}
	blocks, err = store.ListActiveIPBlocks(long)
	if err != nil {
		t.Fatal(err)
	}
	if len(blocks) != 0 {
		t.Fatalf("block at exact expiration should be inactive: %#v", blocks)
	}
}

func TestIPBlockCapacityRejectsNewKeyButAllowsUpdate(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "ip-block-capacity.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	now := time.Date(2026, time.October, 17, 12, 0, 0, 0, time.UTC)
	until := now.Add(time.Hour)
	tx, err := store.database.Begin()
	if err != nil {
		t.Fatal(err)
	}
	stmt, err := tx.Prepare(`INSERT INTO security_ip_blocks(client_key, blocked_until, reason) VALUES (?, ?, '')`)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < maxActiveIPBlocks; i++ {
		if _, err := stmt.Exec(fmt.Sprintf("key-%d", i), until.UnixNano()); err != nil {
			t.Fatal(err)
		}
	}
	if err := stmt.Close(); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if err := store.PutIPBlock("key-0", now.Add(30*time.Minute), "updated", now); err != nil {
		t.Fatalf("existing block update failed at capacity: %v", err)
	}
	if err := store.PutIPBlock("overflow", until, "", now); err == nil {
		t.Fatal("new key was accepted over capacity")
	}
	var count int
	if err := store.database.QueryRow(`SELECT COUNT(*) FROM security_ip_blocks`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != maxActiveIPBlocks {
		t.Fatalf("capacity changed: got %d rows", count)
	}
}

func TestIPBlockFractionalExpiryResetAndSafeDeletion(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "ip-block-precision.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	now := time.Date(2026, time.October, 17, 12, 0, 0, 0, time.UTC)
	key := "'; DROP TABLE security_ip_blocks; --"
	until := now.Add(time.Second + time.Nanosecond)
	if err := store.PutIPBlock(key, until, "probe", now); err != nil {
		t.Fatal(err)
	}
	if err := store.ResetBusinessData(); err != nil {
		t.Fatal(err)
	}
	blocks, err := store.ListActiveIPBlocks(until.Add(-time.Nanosecond))
	if err != nil || len(blocks) != 1 || !blocks[0].BlockedUntil.Equal(until) {
		t.Fatalf("reset or timestamp comparison lost active ban: %#v %v", blocks, err)
	}
	blocks, err = store.ListActiveIPBlocks(until)
	if err != nil || len(blocks) != 0 {
		t.Fatalf("exact fractional expiry still active: %#v %v", blocks, err)
	}
	if err := store.DeleteIPBlock(key); err != nil {
		t.Fatal(err)
	}
	blocks, err = store.ListActiveIPBlocks(now)
	if err != nil || len(blocks) != 0 {
		t.Fatalf("safe deletion failed: %#v %v", blocks, err)
	}
}
