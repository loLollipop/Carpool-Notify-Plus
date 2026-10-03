package cycle

import (
	"fmt"
	"sort"
	"time"
)

// DueExtension shifts this logical billing boundary and every later boundary.
// The original schedule and already paid bill dates are never rewritten.
type DueExtension struct {
	BaseDueDate   string
	ExtensionDays int
}

// WithExtensions returns an immutable effective-date view of a base schedule.
func (schedule BillingSchedule) WithExtensions(extensions []DueExtension) (BillingSchedule, error) {
	schedule.extensions = nil
	ordered := append([]DueExtension(nil), extensions...)
	sort.SliceStable(ordered, func(i, j int) bool { return ordered[i].BaseDueDate < ordered[j].BaseDueDate })
	for _, extension := range ordered {
		valid, err := schedule.IsDueDate(extension.BaseDueDate)
		if err != nil || !valid || extension.ExtensionDays < 1 || extension.ExtensionDays > 365 {
			return BillingSchedule{}, fmt.Errorf("invalid billing extension at %s", extension.BaseDueDate)
		}
	}
	schedule.extensions = ordered
	return schedule, nil
}

func (schedule BillingSchedule) baseSchedule() BillingSchedule {
	schedule.extensions = nil
	return schedule
}

// EffectiveDue maps one original trigger time to its permanently shifted time.
func (schedule BillingSchedule) EffectiveDue(base time.Time) time.Time {
	days := 0
	for _, extension := range schedule.extensions {
		if extension.BaseDueDate > FormatDate(base) {
			break
		}
		days += extension.ExtensionDays
	}
	return base.In(Location).AddDate(0, 0, days)
}

// BaseDueDate resolves an effective bill key to its original logical boundary.
func (schedule BillingSchedule) BaseDueDate(date string) (string, error) {
	day, err := time.ParseInLocation("2006-01-02", date, Location)
	if err != nil {
		return "", err
	}
	base := schedule.baseSchedule()
	days := 0
	for index := 0; index <= len(schedule.extensions); index++ {
		candidate := day.AddDate(0, 0, -days)
		candidateDate := FormatDate(candidate)
		inSegment := (index == 0 || candidateDate >= schedule.extensions[index-1].BaseDueDate) &&
			(index == len(schedule.extensions) || candidateDate < schedule.extensions[index].BaseDueDate)
		if inSegment {
			valid, checkErr := base.IsDueDate(candidateDate)
			if checkErr != nil {
				return "", checkErr
			}
			if valid {
				return candidateDate, nil
			}
		}
		if index < len(schedule.extensions) {
			days += schedule.extensions[index].ExtensionDays
		}
	}
	return "", fmt.Errorf("%s is not an effective billing date", date)
}

func (schedule BillingSchedule) nextEffectiveDue(now time.Time) time.Time {
	base := schedule.baseSchedule()
	days := 0
	for index := 0; index <= len(schedule.extensions); index++ {
		cursor := now.In(Location).AddDate(0, 0, -days)
		if index > 0 {
			lower, _ := time.ParseInLocation("2006-01-02", schedule.extensions[index-1].BaseDueDate, Location)
			if cursor.Before(lower) {
				cursor = lower.Add(-time.Nanosecond)
			}
		}
		candidate := base.NextDue(cursor)
		if candidate.IsZero() {
			return candidate
		}
		if index == len(schedule.extensions) || FormatDate(candidate) < schedule.extensions[index].BaseDueDate {
			return candidate.AddDate(0, 0, days)
		}
		days += schedule.extensions[index].ExtensionDays
	}
	return time.Time{}
}

func (schedule BillingSchedule) lastEffectiveDue(now time.Time) (time.Time, bool) {
	base := schedule.baseSchedule()
	days := 0
	var last time.Time
	for index := 0; index <= len(schedule.extensions); index++ {
		cursor := now.In(Location).AddDate(0, 0, -days)
		if index < len(schedule.extensions) {
			upper, _ := time.ParseInLocation("2006-01-02", schedule.extensions[index].BaseDueDate, Location)
			if !cursor.Before(upper) {
				cursor = upper.Add(-time.Nanosecond)
			}
		}
		candidate, found := base.LastDue(cursor)
		if found && (index == 0 || FormatDate(candidate) >= schedule.extensions[index-1].BaseDueDate) {
			effective := candidate.AddDate(0, 0, days)
			if effective.After(last) {
				last = effective
			}
		}
		if index < len(schedule.extensions) {
			days += schedule.extensions[index].ExtensionDays
		}
	}
	return last, !last.IsZero()
}

// FirstUnpaid uses the same ledger boundary for cards, renewal and extensions.
// Legacy rows without bills start at the next boundary; a delivered extension
// pins that unpaid boundary so it stays visible even after becoming overdue.
// Historical base dates preserve that pin after the corresponding extension
// is revoked or superseded and therefore no longer shifts the active schedule.
func (schedule BillingSchedule) FirstUnpaid(now time.Time, paidDates []string, historicalBaseDates ...string) (time.Time, error) {
	next := schedule.NextDue(now)
	start := next
	paid := make(map[string]struct{}, len(paidDates))
	for _, date := range paidDates {
		paid[date] = struct{}{}
	}
	if len(paidDates) > 0 {
		if last, found := schedule.LastDue(now); found {
			start = last
		}
		for _, date := range paidDates {
			day, err := time.ParseInLocation("2006-01-02", date, Location)
			if err != nil {
				return time.Time{}, err
			}
			if day.Before(start) {
				start = day
			}
		}
		start = schedule.NextDue(StartOfDay(start).Add(-time.Nanosecond))
	}
	anchorDates := append([]string(nil), historicalBaseDates...)
	if len(schedule.extensions) > 0 {
		anchorDates = append(anchorDates, schedule.extensions[0].BaseDueDate)
	}
	base := schedule.baseSchedule()
	for _, anchorDate := range anchorDates {
		if anchorDate == "" {
			continue
		}
		anchor, err := time.ParseInLocation("2006-01-02", anchorDate, Location)
		if err != nil {
			return time.Time{}, fmt.Errorf("invalid unpaid billing anchor %q: %w", anchorDate, err)
		}
		anchor = base.NextDue(anchor.Add(-time.Nanosecond))
		anchor = schedule.EffectiveDue(anchor)
		if anchor.Before(start) {
			start = anchor
		}
	}
	for range len(paidDates) + 1 {
		if _, exists := paid[FormatDate(start)]; !exists {
			return start, nil
		}
		next = schedule.NextDue(start)
		if !next.After(start) {
			return time.Time{}, fmt.Errorf("billing schedule did not advance")
		}
		start = next
	}
	return time.Time{}, fmt.Errorf("unable to find unpaid billing date")
}
