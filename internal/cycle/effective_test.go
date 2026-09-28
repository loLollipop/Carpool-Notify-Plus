package cycle_test

import (
	"carpool-notify/internal/cycle"
	"testing"
	"time"
)

func TestEffectiveBillingScheduleMappings(t *testing.T) {
	for _, test := range []struct{ expression, base, first, second string }{
		{"interval:30d", "2026-10-01", "2026-10-10", "2026-11-09"},
		{"interval:90d", "2026-11-30", "2026-12-09", "2027-03-09"},
		{"0 0 1 * *", "2026-10-01", "2026-10-10", "2026-11-10"},
		{"30 9 1 * *", "2026-10-01", "2026-10-10", "2026-11-10"},
	} {
		t.Run(test.expression, func(t *testing.T) {
			base, err := cycle.ParseBillingSchedule(test.expression, "2026-09-01")
			if err != nil {
				t.Fatal(err)
			}
			schedule, err := base.WithExtensions([]cycle.DueExtension{{BaseDueDate: test.base, ExtensionDays: 9}})
			if err != nil {
				t.Fatal(err)
			}
			day, _ := time.ParseInLocation("2006-01-02", test.base, cycle.Location)
			first := schedule.NextDue(day.Add(-time.Nanosecond))
			if cycle.FormatDate(first) != test.first || cycle.FormatDate(schedule.NextDue(first)) != test.second {
				t.Fatalf("next = %v, %v", first, schedule.NextDue(first))
			}
			if last, ok := schedule.LastDue(first); !ok || !last.Equal(first) {
				t.Fatalf("last = %v, %v", last, ok)
			}
			if last, ok := schedule.LastDue(first.Add(-time.Nanosecond)); !ok || !last.Before(day) {
				t.Fatalf("previous = %v, %v", last, ok)
			}
			if valid, err := schedule.IsDueDate(test.base); err != nil || valid {
				t.Fatalf("old due accepted %v, %v", valid, err)
			}
			if mapped, err := schedule.BaseDueDate(test.first); err != nil || mapped != test.base {
				t.Fatalf("reverse %q, %v", mapped, err)
			}
			if valid, err := schedule.IsDueDate(test.first); err != nil || !valid {
				t.Fatalf("new due rejected %v, %v", valid, err)
			}
			cumulative, err := base.WithExtensions([]cycle.DueExtension{{BaseDueDate: test.base, ExtensionDays: 9}, {BaseDueDate: test.base, ExtensionDays: 9}})
			if err != nil {
				t.Fatal(err)
			}
			if got := cumulative.NextDue(day.Add(-time.Nanosecond)); cycle.FormatDate(got) != cycle.FormatDate(first.AddDate(0, 0, 9)) {
				t.Fatalf("cumulative next = %v", got)
			}
		})
	}
}
