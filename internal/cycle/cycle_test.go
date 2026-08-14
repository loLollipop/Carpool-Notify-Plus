package cycle_test

import (
        "testing"
        "time"

        "carpool-notify/internal/cycle"
)

func TestParseYuanToCents(t *testing.T) {
        cases := []struct {
                input    string
                expected int64
        }{
                {"20", 2000},
                {"20.00", 2000},
                {"20.5", 2050},
                {"20.50", 2050},
                {"0.01", 1},
        }
        for _, testCase := range cases {
                actual, err := cycle.ParseYuanToCents(testCase.input)
                if err != nil {
                        t.Fatalf("%q: unexpected error: %v", testCase.input, err)
                }
                if actual != testCase.expected {
                        t.Fatalf("%q: got %d want %d", testCase.input, actual, testCase.expected)
                }
        }
}

func TestParseYuanToCentsRejectsMalformedAndOverflowingAmounts(t *testing.T) {
        for _, input := range []string{
                "--1",
                "1.-1",
                ".",
                "184467440737095517.00",
        } {
                if cents, err := cycle.ParseYuanToCents(input); err == nil {
                        t.Fatalf("%q: got %d cents, want an error", input, cents)
                }
        }
}

func TestParseOffsets(t *testing.T) {
        offsets, err := cycle.ParseOffsets("3, 1,0,3")
        if err != nil {
                t.Fatal(err)
        }
        if len(offsets) != 3 || offsets[0] != 3 || offsets[1] != 1 || offsets[2] != 0 {
                t.Fatalf("unexpected offsets: %#v", offsets)
        }
}

func TestDescribeCron(t *testing.T) {
        if got := cycle.DescribeCron("0 0 1 * *"); got != "每月 1 日" {
                t.Fatalf("got %q", got)
        }
        if got := cycle.DescribeCron("0 0 * * 1"); got != "每周一" {
                t.Fatalf("got %q", got)
        }
}

func TestNextDueTimes(t *testing.T) {
        schedule, err := cycle.ParseCron("0 0 1 * *")
        if err != nil {
                t.Fatal(err)
        }
        anchor := time.Date(2026, 3, 15, 12, 0, 0, 0, cycle.Location)
        times := cycle.NextDueTimes(schedule, anchor, 5)
        if len(times) != 5 {
                t.Fatalf("want 5 times, got %d", len(times))
        }
        if cycle.FormatDate(times[0]) != "2026-04-01" {
                t.Fatalf("first: %s", cycle.FormatDate(times[0]))
        }
	for index := 1; index < len(times); index++ {
		if !times[index].After(times[index-1]) {
			t.Fatalf("times not increasing: %#v", times)
		}
	}
}

func TestIntervalBillingScheduleAnchorsToBoardedAt(t *testing.T) {
        schedule, err := cycle.ParseBillingSchedule("interval:30d", "2026-07-20")
        if err != nil {
                t.Fatal(err)
        }
        anchor := time.Date(2026, 8, 3, 12, 0, 0, 0, cycle.Location)
        times := schedule.NextDueTimes(anchor, 4)
        got := make([]string, 0, len(times))
        for _, dueAt := range times {
                got = append(got, cycle.FormatDate(dueAt))
        }
        want := []string{"2026-08-19", "2026-09-18", "2026-10-18", "2026-11-17"}
        for index := range want {
                if got[index] != want[index] {
                        t.Fatalf("times = %#v, want %#v", got, want)
                }
        }

        if last, found := schedule.LastDue(anchor); !found || cycle.FormatDate(last) != "2026-07-20" {
                t.Fatalf("last due = %v, found %v", last, found)
        }
        if ok, err := schedule.IsDueDate("2026-08-19"); err != nil || !ok {
                t.Fatalf("2026-08-19 should be due, ok=%v err=%v", ok, err)
        }
	if ok, err := schedule.IsDueDate("2026-08-20"); err != nil || ok {
		t.Fatalf("2026-08-20 should not be due, ok=%v err=%v", ok, err)
	}
}

func TestOneMonthRentalBillingExpression(t *testing.T) {
        schedule, err := cycle.ParseBillingSchedule(cycle.OneMonthRentalExpression, "2026-07-01")
        if err != nil {
                t.Fatal(err)
        }
        firstDue := schedule.NextDue(time.Date(2026, time.June, 30, 23, 59, 59, 0, cycle.Location))
        if got := cycle.FormatDate(firstDue); got != "2026-07-01" {
                t.Fatalf("first due = %s, want 2026-07-01", got)
        }
        if got := cycle.DescribeCron(cycle.OneMonthRentalExpression); got != "仅租一个月（30 天）" {
                t.Fatalf("description = %q", got)
        }
        if !cycle.IsOneMonthRentalExpression(" ONE-TIME:30D ") {
                t.Fatal("one-month expression should be case-insensitive and trim whitespace")
        }
}
