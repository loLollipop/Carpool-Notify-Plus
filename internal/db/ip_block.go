package db

import (
	"database/sql"
	"fmt"
	"strings"
	"time"
)

const maxActiveIPBlocks = 8192

// IPBlock is a persisted IP address or prefix block.
type IPBlock struct {
	ClientKey    string
	BlockedUntil time.Time
	Reason       string
}

// Expirations use Unix nanoseconds so ordering is numeric and independent of
// timezone or the fractional precision of formatted timestamps.
func (store *Store) ListActiveIPBlocks(now time.Time) ([]IPBlock, error) {
	rows, err := store.database.Query(`
        SELECT client_key, blocked_until, reason
        FROM security_ip_blocks WHERE blocked_until > ?`, now.UnixNano())
	if err != nil {
		return nil, fmt.Errorf("list active IP blocks: %w", err)
	}
	defer rows.Close()
	blocks := make([]IPBlock, 0)
	for rows.Next() {
		var block IPBlock
		var until int64
		if err := rows.Scan(&block.ClientKey, &until, &block.Reason); err != nil {
			return nil, fmt.Errorf("scan active IP block: %w", err)
		}
		block.BlockedUntil = time.Unix(0, until).UTC()
		blocks = append(blocks, block)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate active IP blocks: %w", err)
	}
	return blocks, nil
}

// PutIPBlock atomically prunes expired rows and adds or extends a ban. Existing
// active bans are never evicted or shortened to admit another client.
func (store *Store) PutIPBlock(clientKey string, blockedUntil time.Time, reason string, now time.Time) error {
	if strings.TrimSpace(clientKey) == "" {
		return fmt.Errorf("put IP block: client key is empty")
	}
	if !blockedUntil.After(now) {
		return fmt.Errorf("put IP block: expiration must be later than now")
	}
	tx, err := store.database.Begin()
	if err != nil {
		return fmt.Errorf("begin IP block update: %w", err)
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM security_ip_blocks WHERE blocked_until <= ?`, now.UnixNano()); err != nil {
		return fmt.Errorf("clean expired IP blocks: %w", err)
	}
	var existing int64
	err = tx.QueryRow(`SELECT blocked_until FROM security_ip_blocks WHERE client_key = ?`, clientKey).Scan(&existing)
	switch err {
	case nil:
		blockedUntil = time.Unix(0, max(existing, blockedUntil.UnixNano()))
	case sql.ErrNoRows:
		var count int
		if err := tx.QueryRow(`SELECT COUNT(*) FROM security_ip_blocks`).Scan(&count); err != nil {
			return fmt.Errorf("count active IP blocks: %w", err)
		}
		if count >= maxActiveIPBlocks {
			return fmt.Errorf("put IP block: active IP block capacity reached (%d)", maxActiveIPBlocks)
		}
	default:
		return fmt.Errorf("read existing IP block: %w", err)
	}
	if _, err := tx.Exec(`
        INSERT INTO security_ip_blocks(client_key, blocked_until, reason) VALUES (?, ?, ?)
        ON CONFLICT(client_key) DO UPDATE SET
            blocked_until = MAX(security_ip_blocks.blocked_until, excluded.blocked_until),
            reason = excluded.reason`, clientKey, blockedUntil.UnixNano(), reason); err != nil {
		return fmt.Errorf("save IP block: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit IP block update: %w", err)
	}
	return nil
}

func (store *Store) DeleteIPBlock(clientKey string) error {
	if _, err := store.database.Exec(`DELETE FROM security_ip_blocks WHERE client_key = ?`, clientKey); err != nil {
		return fmt.Errorf("delete IP block: %w", err)
	}
	return nil
}
