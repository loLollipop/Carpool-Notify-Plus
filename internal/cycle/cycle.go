package cycle

import (
        "fmt"
        "math"
        "sort"
        "strconv"
        "strings"
        "time"

        "github.com/robfig/cron/v3"
)

// Location is the fixed business timezone (Asia/Shanghai).
var Location *time.Location

func init() {
        location, err := time.LoadLocation("Asia/Shanghai")
        if err != nil {
                // Fallback keeps behavior deterministic even without tzdata.
                location = time.FixedZone("CST", 8*60*60)
        }
        Location = location
}

// Parser is a standard 5-field cron parser.
var Parser = cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)

const intervalPrefix = "interval:"

// OneMonthRentalExpression is the persisted billing expression for a Plus
// rental that ends after one 30-day term and cannot be renewed as another bill.
const OneMonthRentalExpression = "one-time:30d"

// IsOneMonthRentalExpression reports whether expression selects the fixed
// one-month Plus rental mode.
func IsOneMonthRentalExpression(expression string) bool {
        return strings.EqualFold(strings.TrimSpace(expression), OneMonthRentalExpression)
}

// BillingSchedule is a parsed billing cycle rule. It supports the legacy
// 5-field cron syntax plus fixed day intervals such as interval:30d.
type BillingSchedule struct {
        expression   string
        cronSchedule cron.Schedule
        intervalDays int
        anchor       time.Time
        hasAnchor    bool
}

// ParseCron validates and returns a schedule for the given expression.
func ParseCron(expression string) (cron.Schedule, error) {
        expression = strings.TrimSpace(expression)
        if expression == "" {
                return nil, fmt.Errorf("cron expression is required")
        }
        schedule, err := Parser.Parse(expression)
        if err != nil {
                return nil, fmt.Errorf("invalid cron expression: %w", err)
        }
        return schedule, nil
}

// ParseBillingSchedule validates and returns a billing schedule for cron or
// interval expressions. Interval expressions are anchored to boardedAt.
func ParseBillingSchedule(expression string, boardedAt string) (BillingSchedule, error) {
        expression = strings.TrimSpace(expression)
        if expression == "" {
                return BillingSchedule{}, fmt.Errorf("cycle expression is required")
        }

        anchor, hasAnchor, err := parseAnchorDate(boardedAt)
        if err != nil {
                return BillingSchedule{}, err
        }
        if IsOneMonthRentalExpression(expression) {
                if !hasAnchor {
                        return BillingSchedule{}, fmt.Errorf("boarded_at is required for one-month rental")
                }
                return BillingSchedule{
                        expression:   OneMonthRentalExpression,
                        intervalDays: 30,
                        anchor:       anchor,
                        hasAnchor:    true,
                }, nil
        }

        if days, ok, err := parseIntervalDays(expression); ok || err != nil {
                if err != nil {
                        return BillingSchedule{}, err
                }
                if !hasAnchor {
                        return BillingSchedule{}, fmt.Errorf("boarded_at is required for interval cycle")
                }
                return BillingSchedule{
                        expression:   expression,
                        intervalDays: days,
                        anchor:       anchor,
                        hasAnchor:    true,
                }, nil
        }

        schedule, err := ParseCron(expression)
        if err != nil {
                return BillingSchedule{}, err
        }
        return BillingSchedule{
                expression:   expression,
                cronSchedule: schedule,
                anchor:       anchor,
                hasAnchor:    hasAnchor,
        }, nil
}

// NextDue returns the next trigger time strictly after now.
func (schedule BillingSchedule) NextDue(now time.Time) time.Time {
        now = now.In(Location)
        if schedule.intervalDays > 0 {
                return schedule.nextIntervalDue(now)
        }

        nextOccurrence := schedule.cronSchedule.Next(now)
        for schedule.hasAnchor && nextOccurrence.Before(schedule.anchor) {
                nextOccurrence = schedule.cronSchedule.Next(StartOfDay(nextOccurrence).AddDate(0, 0, 1).Add(-time.Nanosecond))
        }
        return nextOccurrence
}

// NextDueTimes returns the next count trigger times strictly after now.
func (schedule BillingSchedule) NextDueTimes(now time.Time, count int) []time.Time {
        if count <= 0 {
                return nil
        }
        cursor := now.In(Location)
        times := make([]time.Time, 0, count)
        for len(times) < count {
                nextOccurrence := schedule.NextDue(cursor)
                times = append(times, nextOccurrence)
                if !nextOccurrence.After(cursor) {
                        cursor = cursor.Add(time.Minute)
                        continue
                }
                cursor = nextOccurrence
        }
        return times
}

// LastDue returns the most recent trigger time at or before now.
func (schedule BillingSchedule) LastDue(now time.Time) (time.Time, bool) {
        now = now.In(Location)
        if schedule.intervalDays > 0 {
                return schedule.lastIntervalDue(now)
        }
        last, found := LastDue(schedule.cronSchedule, now)
        if !found {
                return time.Time{}, false
        }
        if schedule.hasAnchor && last.Before(schedule.anchor) {
                return time.Time{}, false
        }
        return last, true
}

// IsDueDate reports whether dueDate is one occurrence of this schedule.
func (schedule BillingSchedule) IsDueDate(dueDate string) (bool, error) {
        day, err := time.ParseInLocation("2006-01-02", strings.TrimSpace(dueDate), Location)
        if err != nil {
                return false, err
        }
        day = StartOfDay(day)
        if schedule.intervalDays > 0 {
                if day.Before(schedule.anchor) {
                        return false, nil
                }
                diffDays := int(day.Sub(schedule.anchor).Hours() / 24)
                return diffDays%schedule.intervalDays == 0, nil
        }
        firstOccurrence := schedule.cronSchedule.Next(day.Add(-time.Minute))
        return FormatDate(firstOccurrence) == FormatDate(day), nil
}

func (schedule BillingSchedule) nextIntervalDue(now time.Time) time.Time {
        now = now.In(Location)
        if now.Before(schedule.anchor) {
                return schedule.anchor
        }
        interval := time.Duration(schedule.intervalDays) * 24 * time.Hour
        elapsed := now.Sub(schedule.anchor)
        intervals := int(elapsed/interval) + 1
        if intervals < 0 {
                intervals = 0
        }
        return schedule.anchor.AddDate(0, 0, intervals*schedule.intervalDays)
}

func (schedule BillingSchedule) lastIntervalDue(now time.Time) (time.Time, bool) {
        now = now.In(Location)
        if now.Before(schedule.anchor) {
                return time.Time{}, false
        }
        interval := time.Duration(schedule.intervalDays) * 24 * time.Hour
        intervals := int(now.Sub(schedule.anchor) / interval)
        last := schedule.anchor.AddDate(0, 0, intervals*schedule.intervalDays)
        for last.After(now) && intervals > 0 {
                intervals--
                last = schedule.anchor.AddDate(0, 0, intervals*schedule.intervalDays)
        }
        return last, true
}

func parseAnchorDate(raw string) (time.Time, bool, error) {
        raw = strings.TrimSpace(raw)
        if raw == "" {
                return time.Time{}, false, nil
        }
        anchor, err := time.ParseInLocation("2006-01-02", raw, Location)
        if err != nil {
                return time.Time{}, false, fmt.Errorf("invalid boarded_at: %w", err)
        }
        return StartOfDay(anchor), true, nil
}

func parseIntervalDays(expression string) (int, bool, error) {
        normalized := strings.ToLower(strings.TrimSpace(expression))
        if !strings.HasPrefix(normalized, intervalPrefix) {
                return 0, false, nil
        }
        raw := strings.TrimPrefix(normalized, intervalPrefix)
        if !strings.HasSuffix(raw, "d") {
                return 0, true, fmt.Errorf("invalid interval cycle %q", expression)
        }
        rawDays := strings.TrimSuffix(raw, "d")
        if rawDays == "" {
                return 0, true, fmt.Errorf("invalid interval cycle %q", expression)
        }
        days, err := strconv.Atoi(rawDays)
        if err != nil || days <= 0 {
                return 0, true, fmt.Errorf("invalid interval cycle %q", expression)
        }
        if days > 3650 {
                return 0, true, fmt.Errorf("interval cycle is too large: %d days", days)
        }
        return days, true, nil
}

// NextDue returns the next trigger time after now in Asia/Shanghai.
func NextDue(schedule cron.Schedule, now time.Time) time.Time {
        return schedule.Next(now.In(Location))
}

// NextDueTimes returns the next count trigger times strictly after now in Asia/Shanghai.
func NextDueTimes(schedule cron.Schedule, now time.Time, count int) []time.Time {
        if count <= 0 {
                return nil
        }
        now = now.In(Location)
        cursor := now
        times := make([]time.Time, 0, count)
        for len(times) < count {
                nextOccurrence := schedule.Next(cursor)
                times = append(times, nextOccurrence)
                if !nextOccurrence.After(cursor) {
                        cursor = cursor.Add(time.Minute)
                        continue
                }
                cursor = nextOccurrence
        }
        return times
}

// FormatDateTime formats a time as YYYY-MM-DD HH:MM in Asia/Shanghai.
func FormatDateTime(moment time.Time) string {
        return moment.In(Location).Format("2006-01-02 15:04")
}

// LastDue returns the most recent trigger time at or before now.
// It walks forward from a lookback window; returns false if none found.
func LastDue(schedule cron.Schedule, now time.Time) (time.Time, bool) {
        now = now.In(Location)
        cursor := now.AddDate(-3, 0, 0)
        var last time.Time
        found := false
        for range 20000 {
                nextOccurrence := schedule.Next(cursor)
                if nextOccurrence.After(now) {
                        break
                }
                last = nextOccurrence
                found = true
                if !nextOccurrence.After(cursor) {
                        cursor = cursor.Add(time.Minute)
                        continue
                }
                cursor = nextOccurrence
        }
        return last, found
}

// SendAt returns the planned send time for a due occurrence and offset days (09:00).
func SendAt(dueAt time.Time, offsetDays int) time.Time {
        dueDate := StartOfDay(dueAt.In(Location))
        sendDay := dueDate.AddDate(0, 0, -offsetDays)
        return time.Date(sendDay.Year(), sendDay.Month(), sendDay.Day(), 9, 0, 0, 0, Location)
}

// StartOfDay returns midnight for the calendar day of t in Asia/Shanghai.
func StartOfDay(moment time.Time) time.Time {
        moment = moment.In(Location)
        return time.Date(moment.Year(), moment.Month(), moment.Day(), 0, 0, 0, 0, Location)
}

// FormatDate formats a time as YYYY-MM-DD in Asia/Shanghai.
func FormatDate(moment time.Time) string {
        return moment.In(Location).Format("2006-01-02")
}

// DaysRemaining returns whole days from today to the next due calendar day.
func DaysRemaining(nextDue time.Time, now time.Time) int {
        today := StartOfDay(now)
        dueDay := StartOfDay(nextDue)
        hours := dueDay.Sub(today).Hours()
        return int(hours / 24)
}

// DescribeCron returns a short Chinese description for common patterns.
func DescribeCron(expression string) string {
        expression = strings.TrimSpace(expression)
        if IsOneMonthRentalExpression(expression) {
                return "仅租一个月（30 天）"
        }
        if days, ok, err := parseIntervalDays(expression); ok {
                if err != nil {
                        return expression
                }
                switch days {
                case 30:
                        return "月付（每 30 天）"
                case 90:
                        return "季付（每 90 天）"
                case 180:
                        return "半年付（每 180 天）"
                case 365:
                        return "年付（每 365 天）"
                default:
                        return fmt.Sprintf("每 %d 天", days)
                }
        }
        fields := strings.Fields(expression)
        if len(fields) != 5 {
                return expression
        }

        minute, hour, dayOfMonth, month, dayOfWeek := fields[0], fields[1], fields[2], fields[3], fields[4]

        if month == "*" && dayOfMonth == "*" && dayOfWeek == "*" {
                if minute == "0" && hour == "0" {
                        return "每天"
                }
                return fmt.Sprintf("每天 %s:%s", padTwo(hour), padTwo(minute))
        }

        if month == "*" && dayOfMonth == "*" && dayOfWeek != "*" && !strings.ContainsAny(dayOfWeek, "-,*/") {
                weekdayName := weekdayChinese(dayOfWeek)
                if weekdayName != "" {
                        if minute == "0" && hour == "0" {
                                return "每" + weekdayName
                        }
                        return fmt.Sprintf("每%s %s:%s", weekdayName, padTwo(hour), padTwo(minute))
                }
        }

        if month == "*" && dayOfWeek == "*" && dayOfMonth != "*" && !strings.ContainsAny(dayOfMonth, "-,*/") {
                if minute == "0" && hour == "0" {
                        return fmt.Sprintf("每月 %s 日", dayOfMonth)
                }
                return fmt.Sprintf("每月 %s 日 %s:%s", dayOfMonth, padTwo(hour), padTwo(minute))
        }

        if dayOfWeek == "*" && dayOfMonth == "1" && minute == "0" && hour == "0" {
                switch month {
                case "*/3":
                        return "季付（每 3 个月 1 日）"
                case "1,7":
                        return "半年付（1 月 / 7 月 1 日）"
                case "1":
                        return "年付（每年 1 月 1 日）"
                }
        }

        return expression
}

func weekdayChinese(token string) string {
        mapping := map[string]string{
                "0": "周日", "7": "周日",
                "1": "周一", "2": "周二", "3": "周三",
                "4": "周四", "5": "周五", "6": "周六",
                "SUN": "周日", "MON": "周一", "TUE": "周二",
                "WED": "周三", "THU": "周四", "FRI": "周五", "SAT": "周六",
        }
        return mapping[strings.ToUpper(token)]
}

func padTwo(value string) string {
        if len(value) == 1 {
                return "0" + value
        }
        return value
}

// ParseOffsets parses "3,1,0" into a sorted unique non-negative int slice.
// Empty input is allowed and means no reminders are scheduled.
func ParseOffsets(raw string) ([]int, error) {
        raw = strings.TrimSpace(raw)
        if raw == "" {
                return []int{}, nil
        }
        parts := strings.Split(raw, ",")
        seen := map[int]struct{}{}
        offsets := make([]int, 0, len(parts))
        for _, part := range parts {
                part = strings.TrimSpace(part)
                if part == "" {
                        continue
                }
                value, err := strconv.Atoi(part)
                if err != nil {
                        return nil, fmt.Errorf("invalid offset %q", part)
                }
                if value < 0 {
                        return nil, fmt.Errorf("offset must be non-negative: %d", value)
                }
                if _, exists := seen[value]; exists {
                        continue
                }
                seen[value] = struct{}{}
                offsets = append(offsets, value)
        }
        if len(offsets) == 0 {
                return nil, fmt.Errorf("notify offsets are required")
        }
        sort.Sort(sort.Reverse(sort.IntSlice(offsets)))
        return offsets, nil
}

// FormatOffsets renders offsets as comma-separated text.
func FormatOffsets(offsets []int) string {
        parts := make([]string, 0, len(offsets))
        for _, offset := range offsets {
                parts = append(parts, strconv.Itoa(offset))
        }
        return strings.Join(parts, ",")
}

// ParseYuanToCents converts a yuan amount string to integer cents.
func ParseYuanToCents(raw string) (int64, error) {
        raw = strings.TrimSpace(raw)
        if raw == "" {
                return 0, fmt.Errorf("price is required")
        }
        if strings.HasPrefix(raw, "-") {
                return 0, fmt.Errorf("price must be non-negative")
        }
        parts := strings.Split(raw, ".")
        if len(parts) > 2 {
                return 0, fmt.Errorf("invalid price %q", raw)
        }
        yuanPart := parts[0]
        if yuanPart == "" {
                yuanPart = "0"
        }
        if strings.IndexFunc(yuanPart, func(character rune) bool {
                return character < '0' || character > '9'
        }) >= 0 {
                return 0, fmt.Errorf("invalid price %q", raw)
        }
        yuanValue, err := strconv.ParseInt(yuanPart, 10, 64)
        if err != nil {
                return 0, fmt.Errorf("invalid price %q", raw)
        }
        var centsPart int64
        if len(parts) == 2 {
                fraction := parts[1]
                if len(fraction) > 2 {
                        return 0, fmt.Errorf("price supports at most 2 decimal places")
                }
                if fraction == "" && parts[0] == "" {
                        return 0, fmt.Errorf("invalid price %q", raw)
                }
                if strings.IndexFunc(fraction, func(character rune) bool {
                        return character < '0' || character > '9'
                }) >= 0 {
                        return 0, fmt.Errorf("invalid price %q", raw)
                }
                for len(fraction) < 2 {
                        fraction += "0"
                }
                centsPart, err = strconv.ParseInt(fraction, 10, 64)
                if err != nil {
                        return 0, fmt.Errorf("invalid price %q", raw)
                }
        }
        if yuanValue > (math.MaxInt64-centsPart)/100 {
                return 0, fmt.Errorf("price is too large")
        }
        total := yuanValue*100 + centsPart
        return total, nil
}

// FormatCents formats integer cents as a two-decimal yuan string.
func FormatCents(cents int64) string {
        negative := cents < 0
        magnitude := uint64(cents)
        if negative {
                // -(MinInt64) overflows int64. Converting via -(n+1)+1 keeps the
                // full magnitude representable as uint64.
                magnitude = uint64(-(cents + 1)) + 1
        }
        whole := magnitude / 100
        fraction := magnitude % 100
        result := fmt.Sprintf("%d.%02d", whole, fraction)
        if negative {
                return "-" + result
        }
        return result
}

// Now returns the current time in Asia/Shanghai.
func Now() time.Time {
        return time.Now().In(Location)
}
