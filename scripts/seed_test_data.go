//go:build ignore

// One-shot local seeder:
//
//	go run scripts/seed_test_data.go
package main

import (
	"database/sql"
	"fmt"
	"log"
	"math/rand"
	"path/filepath"
	"strings"
	"time"

	"carpool-notify/internal/config"
	"carpool-notify/internal/db"
	"carpool-notify/internal/service"

	_ "modernc.org/sqlite"
)

const (
	accountCount      = 50
	subscriptionCount = 120
	targetBillYuan    = 10080 // bills total exactly this many yuan
)

func main() {
	configuration, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	if err := wipeBusinessData(configuration.DatabasePath); err != nil {
		log.Fatalf("wipe: %v", err)
	}

	store, err := db.Open(configuration.DatabasePath)
	if err != nil {
		log.Fatalf("database: %v", err)
	}
	defer store.Close()

	svc := &service.SubscriptionService{Store: store, Config: configuration}
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))

	payments := []string{"Visa", "万事达卡"}
	firstNames := []string{"阿木", "小北", "林深", "青禾", "南栀", "柚子", "晚星", "白鹭", "江城", "清欢", "予安", "知夏", "顾言", "沈舟", "陆远"}
	offsets := []string{"0", "1,0", "3,1,0", "7,3,1,0", "2,0"}

	// Spread due days across 1–28 so the month calendar isn't clumped.
	dueDays := make([]int, subscriptionCount)
	for i := range dueDays {
		dueDays[i] = (i % 28) + 1
	}
	rng.Shuffle(len(dueDays), func(i, j int) { dueDays[i], dueDays[j] = dueDays[j], dueDays[i] })

	seatPlan := distributeSeats(accountCount, subscriptionCount, rng)
	accountIDs := make([]int64, 0, accountCount)
	for i := 0; i < accountCount; i++ {
		capacity := seatPlan[i] + rng.Intn(3)
		if capacity < seatPlan[i] {
			capacity = seatPlan[i]
		}
		if capacity < 1 {
			capacity = 1
		}
		accountID, err := svc.CreateAccount(service.CreateAccountInput{
			Name:          fmt.Sprintf("ChatGPT 账号-%02d", i+1),
			Remark:        []string{"Team 主号", "共享席位", "美区账号", "本地演示", ""}[rng.Intn(5)],
			PaymentMethod: payments[rng.Intn(len(payments))],
			SeatCount:     capacity,
		})
		if err != nil {
			log.Fatalf("create account: %v", err)
		}
		accountIDs = append(accountIDs, accountID)
	}

	billCents := partitionCents(int64(targetBillYuan)*100, subscriptionCount, rng, 800)

	type seatSlot struct {
		accountID int64
	}
	slots := make([]seatSlot, 0, subscriptionCount)
	for i, accountID := range accountIDs {
		for j := 0; j < seatPlan[i]; j++ {
			slots = append(slots, seatSlot{accountID: accountID})
		}
	}
	rng.Shuffle(len(slots), func(i, j int) { slots[i], slots[j] = slots[j], slots[i] })

	createdSubs := 0
	createdBills := 0
	var billSum int64
	dayHistogram := map[int]int{}

	for i, slot := range slots {
		priceCents := billCents[i]
		costCents := priceCents * int64(5+rng.Intn(14)) / 100 // 5–18% → profit ≥ 82%
		priceYuan := fmt.Sprintf("%d.%02d", priceCents/100, priceCents%100)
		costYuan := fmt.Sprintf("%d.%02d", costCents/100, costCents%100)
		person := firstNames[rng.Intn(len(firstNames))]
		email := fmt.Sprintf("%s%03d@example.com", []string{"amy", "bob", "chen", "dawn", "eli", "faye", "gus"}[rng.Intn(7)], rng.Intn(900)+100)
		dueDay := dueDays[i]
		cronExpr := fmt.Sprintf("0 0 %d * *", dueDay)
		dayHistogram[dueDay]++
		// Board early enough that the current month still shows occurrences.
		boarded := time.Now().AddDate(0, -3-rng.Intn(6), -rng.Intn(20)).Format("2006-01-02")

		subID, err := svc.Create(service.CreateInput{
			Name:             "",
			PriceYuan:        priceYuan,
			CostYuan:         costYuan,
			CronExpr:         cronExpr,
			NotifyOffsetsRaw: offsets[rng.Intn(len(offsets))],
			Remark:           fmt.Sprintf("%s · ChatGPT 拼车 %d", person, rng.Intn(1000)),
			TradeURL:         fmt.Sprintf("https://trade.example.com/o/%d", 100000+rng.Intn(899999)),
			CustomerEmail:    email,
			AccountID:        slot.accountID,
			BoardedAt:        boarded,
		})
		if err != nil {
			log.Fatalf("create subscription #%d: %v", i+1, err)
		}
		createdSubs++

		periods, err := svc.ListDuePeriodOptions(subID, "")
		if err != nil || len(periods) == 0 {
			log.Fatalf("no due periods for subscription %d: %v", subID, err)
		}
		dueDate := ""
		for _, period := range periods {
			if !period.Paid {
				dueDate = period.StartDate
				break
			}
		}
		if dueDate == "" {
			dueDate = periods[0].StartDate
		}
		if err := svc.SetDuePaid(subID, dueDate, true); err != nil {
			log.Fatalf("mark paid sub=%d due=%s: %v", subID, dueDate, err)
		}
		bill, err := store.GetBillByOccurrence(subID, dueDate)
		if err != nil {
			log.Fatalf("get bill: %v", err)
		}
		if err := svc.UpdateBill(bill.ID, service.BillEditInput{
			AmountYuan: priceYuan,
			Note:       "测试种子账单",
		}); err != nil {
			log.Fatalf("update bill amount: %v", err)
		}
		createdBills++
		billSum += priceCents
	}

	absDB, _ := filepath.Abs(configuration.DatabasePath)
	fmt.Printf("reset + seeded %s\n", absDB)
	fmt.Printf("accounts=%d subscriptions=%d bills=%d total=¥%d.%02d (target ¥%d)\n",
		accountCount, createdSubs, createdBills, billSum/100, billSum%100, targetBillYuan)
	minDay, maxDay := subscriptionCount, 0
	for day := 1; day <= 28; day++ {
		count := dayHistogram[day]
		if count < minDay {
			minDay = count
		}
		if count > maxDay {
			maxDay = count
		}
	}
	fmt.Printf("due days 1–28: %d–%d subscriptions/day\n", minDay, maxDay)
	if billSum != int64(targetBillYuan)*100 {
		log.Fatalf("bill total mismatch: got %d cents want %d", billSum, targetBillYuan*100)
	}
}

func wipeBusinessData(databasePath string) error {
	database, err := sql.Open("sqlite", databasePath)
	if err != nil {
		return err
	}
	defer database.Close()
	if _, err := database.Exec(`PRAGMA foreign_keys = OFF`); err != nil {
		return err
	}
	statements := []string{
		`DELETE FROM notification_log`,
		`DELETE FROM paid_due_occurrences`,
		`DELETE FROM bills`,
		`DELETE FROM subscriptions`,
		`DELETE FROM seats`,
		`DELETE FROM accounts`,
		`DELETE FROM sqlite_sequence WHERE name IN ('accounts','seats','subscriptions','bills','notification_log')`,
	}
	for _, statement := range statements {
		if _, err := database.Exec(statement); err != nil {
			if strings.Contains(statement, "sqlite_sequence") {
				continue
			}
			return fmt.Errorf("%s: %w", statement, err)
		}
	}
	if _, err := database.Exec(`PRAGMA foreign_keys = ON`); err != nil {
		return err
	}
	return nil
}

func distributeSeats(accounts int, totalSubs int, rng *rand.Rand) []int {
	plan := make([]int, accounts)
	for i := range plan {
		plan[i] = 1
	}
	remaining := totalSubs - accounts
	if remaining < 0 {
		for i := range plan {
			if i < totalSubs {
				plan[i] = 1
			} else {
				plan[i] = 0
			}
		}
		return plan
	}
	for remaining > 0 {
		i := rng.Intn(accounts)
		if plan[i] >= 6 {
			continue
		}
		plan[i]++
		remaining--
	}
	return plan
}

func partitionCents(total int64, n int, rng *rand.Rand, minCents int64) []int64 {
	if int64(n)*minCents > total {
		minCents = total / int64(n)
		if minCents < 1 {
			minCents = 1
		}
	}
	parts := make([]int64, n)
	for i := range parts {
		parts[i] = minCents
	}
	remaining := total - minCents*int64(n)
	for remaining > 0 {
		i := rng.Intn(n)
		chunk := int64(1 + rng.Intn(3000))
		if chunk > remaining {
			chunk = remaining
		}
		parts[i] += chunk
		remaining -= chunk
	}
	return parts
}
