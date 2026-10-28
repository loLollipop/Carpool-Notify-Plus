package db

import (
	"strings"

	"carpool-notify/internal/model"
)

// ListApprovedMultiPeriodRenewalEndDates returns only the completion dates
// needed to distinguish one explicit multi-period purchase from separate
// one-period renewals when rendering subscription progress.
func (store *Store) ListApprovedMultiPeriodRenewalEndDates(
	subscriptionIDs []int64,
) (map[int64]map[string]struct{}, error) {
	result := make(map[int64]map[string]struct{})
	if len(subscriptionIDs) == 0 {
		return result, nil
	}

	placeholders := make([]string, len(subscriptionIDs))
	arguments := make([]any, 0, len(subscriptionIDs)+1)
	arguments = append(arguments, model.RenewalStatusApproved)
	for index, subscriptionID := range subscriptionIDs {
		placeholders[index] = "?"
		arguments = append(arguments, subscriptionID)
	}
	rows, err := store.database.Query(`
		SELECT subscription_id, period_end_date
		FROM renewal_applications
		WHERE status = ?
		  AND period_count > 1
		  AND period_end_date != ''
		  AND subscription_id IN (`+strings.Join(placeholders, ",")+`)`,
		arguments...,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var subscriptionID int64
		var periodEndDate string
		if err := rows.Scan(&subscriptionID, &periodEndDate); err != nil {
			return nil, err
		}
		periodEndDate = strings.TrimSpace(periodEndDate)
		if periodEndDate == "" {
			continue
		}
		if result[subscriptionID] == nil {
			result[subscriptionID] = make(map[string]struct{})
		}
		result[subscriptionID][periodEndDate] = struct{}{}
	}
	return result, rows.Err()
}
