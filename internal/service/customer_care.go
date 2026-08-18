package service

import (
	"database/sql"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"carpool-notify/internal/cycle"
	"carpool-notify/internal/db"
	"carpool-notify/internal/model"
)

const (
	customerBenefitCooldownDays    = 90
	customerBenefitLeadDays        = 7
	customerBenefitOutcomeGrace    = 7
	maximumBenefitSelection        = 200
	maximumBenefitNameRunes        = 80
	maximumBenefitNoteRunes        = 500
	maximumBenefitAmountCents      = int64(10_000_000)
	minimumBetaBinomialOutcomes    = 20
	minimumSurvivalOutcomes        = 100
	minimumSurvivalChurns          = 10
	minimumBGNBDRepeatCustomers    = 50
	minimumUpliftEvaluatedBenefits = 40
)

type CustomerBenefitCandidate struct {
	SubscriptionID         int64  `json:"subscription_id"`
	CustomerEmail          string `json:"customer_email"`
	CustomerWechat         string `json:"customer_wechat"`
	DisplayName            string `json:"display_name"`
	CustomerTier           string `json:"customer_tier"`
	SeatCount              int    `json:"seat_count"`
	CurrentCycleValueCents int64  `json:"current_cycle_value_cents"`
	RenewalCount           int    `json:"renewal_count"`
	RelationshipDays       int    `json:"relationship_days"`
	NextDueDate            string `json:"next_due_date"`
	LastPaidDate           string `json:"last_paid_date"`
	LastBenefitDate        string `json:"last_benefit_date"`
	NextEligibleDate       string `json:"next_eligible_date"`
	RecommendedDate        string `json:"recommended_date"`
	ReasonCode             string `json:"reason_code"`
	SuggestedBenefitType   string `json:"suggested_benefit_type"`
	Status                 string `json:"status"`
	Recommended            bool   `json:"recommended"`
	Selectable             bool   `json:"selectable"`
}

type CustomerBenefitView struct {
	model.CustomerBenefit
	Outcome string `json:"outcome"`
}

type CustomerCareSummary struct {
	CustomerCount            int   `json:"customer_count"`
	RecommendedCount         int   `json:"recommended_count"`
	UpcomingCount            int   `json:"upcoming_count"`
	BenefitCount             int   `json:"benefit_count"`
	TotalActualCostCents     int64 `json:"total_actual_cost_cents"`
	TotalPerceivedValueCents int64 `json:"total_perceived_value_cents"`
	EvaluatedBenefitCount    int   `json:"evaluated_benefit_count"`
	RenewedAfterBenefitCount int   `json:"renewed_after_benefit_count"`
}

type ForecastModelReadiness struct {
	Key             string `json:"key"`
	Status          string `json:"status"`
	CurrentSamples  int    `json:"current_samples"`
	RequiredSamples int    `json:"required_samples"`
	DetailCode      string `json:"detail_code"`
}

type PredictionReadiness struct {
	ActiveModel                 string                   `json:"active_model"`
	RenewalOutcomeCount         int                      `json:"renewal_outcome_count"`
	RenewalSuccessCount         int                      `json:"renewal_success_count"`
	ChurnOutcomeCount           int                      `json:"churn_outcome_count"`
	FirstCycleSubscriptionCount int                      `json:"first_cycle_subscription_count"`
	RepeatSubscriptionCount     int                      `json:"repeat_subscription_count"`
	RepeatCustomerCount         int                      `json:"repeat_customer_count"`
	EstimatedRenewalPercent     *int                     `json:"estimated_renewal_percent"`
	EstimateLowPercent          *int                     `json:"estimate_low_percent"`
	EstimateHighPercent         *int                     `json:"estimate_high_percent"`
	Models                      []ForecastModelReadiness `json:"models"`
}

type CustomerCareCenter struct {
	Summary    CustomerCareSummary        `json:"summary"`
	Candidates []CustomerBenefitCandidate `json:"candidates"`
	History    []CustomerBenefitView      `json:"history"`
	Prediction PredictionReadiness        `json:"prediction"`
}

type RecordCustomerBenefitsInput struct {
	SubscriptionIDs    []int64
	BenefitType        string
	BenefitName        string
	ActualCostYuan     string
	PerceivedValueYuan string
	BenefitDate        string
	Note               string
}

type customerBenefitGroup struct {
	ID      int64
	Members []PricingCandidate
}

func (service *SubscriptionService) buildCustomerCare(
	candidates []PricingCandidate,
) (CustomerCareCenter, error) {
	benefits, err := service.Store.ListCustomerBenefits()
	if err != nil {
		return CustomerCareCenter{}, err
	}
	bills, err := service.Store.ListBills()
	if err != nil {
		return CustomerCareCenter{}, err
	}
	afterSalesCases, err := service.Store.ListAfterSalesCases()
	if err != nil {
		return CustomerCareCenter{}, err
	}
	activeSubscriptions, err := service.Store.ListSubscriptions()
	if err != nil {
		return CustomerCareCenter{}, err
	}
	archivedSubscriptions, err := service.Store.ListArchivedSubscriptions()
	if err != nil {
		return CustomerCareCenter{}, err
	}

	refundedBillIDs := refundedBenefitBillIDs(afterSalesCases)
	views := buildCustomerBenefitViews(
		benefits,
		bills,
		refundedBillIDs,
		activeSubscriptions,
		archivedSubscriptions,
		service.now(),
	)
	groups := groupCustomerCareCandidates(candidates)
	careCandidates := make([]CustomerBenefitCandidate, 0, len(groups))
	for _, group := range groups {
		groupBenefits := benefitsForCustomerGroup(group, benefits)
		sort.SliceStable(groupBenefits, func(left int, right int) bool {
			if groupBenefits[left].BenefitDate != groupBenefits[right].BenefitDate {
				return groupBenefits[left].BenefitDate > groupBenefits[right].BenefitDate
			}
			return groupBenefits[left].ID > groupBenefits[right].ID
		})
		careCandidates = append(
			careCandidates,
			buildCustomerBenefitCandidate(group, groupBenefits, service.now()),
		)
	}
	sortCustomerBenefitCandidates(careCandidates)

	center := CustomerCareCenter{
		Candidates: careCandidates,
		History:    views,
		Prediction: buildPredictionReadiness(
			activeSubscriptions,
			archivedSubscriptions,
			bills,
			afterSalesCases,
			views,
		),
	}
	center.Summary.CustomerCount = len(careCandidates)
	center.Summary.BenefitCount = len(benefits)
	for _, candidate := range careCandidates {
		if candidate.Recommended {
			center.Summary.RecommendedCount++
		} else if candidate.Status == "upcoming" {
			center.Summary.UpcomingCount++
		}
	}
	for _, benefit := range benefits {
		center.Summary.TotalActualCostCents += benefit.ActualCostCents
		center.Summary.TotalPerceivedValueCents += benefit.PerceivedValueCents
	}
	for _, view := range views {
		if view.Outcome == "pending" {
			continue
		}
		center.Summary.EvaluatedBenefitCount++
		if view.Outcome == "renewed" {
			center.Summary.RenewedAfterBenefitCount++
		}
	}
	return center, nil
}

func benefitsForCustomerGroup(
	group customerBenefitGroup,
	benefits []model.CustomerBenefit,
) []model.CustomerBenefit {
	subscriptionIDs := make(map[int64]struct{}, len(group.Members))
	emails := make(map[string]struct{})
	wechats := make(map[string]struct{})
	for _, member := range group.Members {
		subscriptionIDs[member.SubscriptionID] = struct{}{}
		if email := normalizeCustomerIdentity(member.CustomerEmail); email != "" {
			emails[email] = struct{}{}
		}
		if wechat := normalizeCustomerIdentity(member.CustomerWechat); wechat != "" {
			wechats[wechat] = struct{}{}
		}
	}
	matched := make([]model.CustomerBenefit, 0)
	for _, benefit := range benefits {
		_, subscriptionMatches := subscriptionIDs[benefit.SubscriptionID]
		_, emailMatches := emails[normalizeCustomerIdentity(benefit.CustomerEmailSnapshot)]
		_, wechatMatches := wechats[normalizeCustomerIdentity(benefit.CustomerWechatSnapshot)]
		if subscriptionMatches ||
			(normalizeCustomerIdentity(benefit.CustomerEmailSnapshot) != "" && emailMatches) ||
			(normalizeCustomerIdentity(benefit.CustomerWechatSnapshot) != "" && wechatMatches) {
			matched = append(matched, benefit)
		}
	}
	return matched
}

func groupCustomerCareCandidates(candidates []PricingCandidate) []customerBenefitGroup {
	grouped := make(map[int64][]PricingCandidate)
	for index, candidate := range candidates {
		groupID := candidate.CustomerGroupID
		if groupID <= 0 {
			groupID = -int64(index + 1)
		}
		grouped[groupID] = append(grouped[groupID], candidate)
	}
	groups := make([]customerBenefitGroup, 0, len(grouped))
	for groupID, members := range grouped {
		sort.SliceStable(members, func(left int, right int) bool {
			return members[left].SubscriptionID < members[right].SubscriptionID
		})
		groups = append(groups, customerBenefitGroup{ID: groupID, Members: members})
	}
	sort.SliceStable(groups, func(left int, right int) bool {
		return groups[left].Members[0].SubscriptionID < groups[right].Members[0].SubscriptionID
	})
	return groups
}

func buildCustomerBenefitCandidate(
	group customerBenefitGroup,
	benefits []model.CustomerBenefit,
	now time.Time,
) CustomerBenefitCandidate {
	representative := group.Members[0]
	candidate := CustomerBenefitCandidate{
		SubscriptionID:         representative.SubscriptionID,
		CustomerEmail:          representative.CustomerEmail,
		CustomerWechat:         representative.CustomerWechat,
		DisplayName:            representative.Name,
		CustomerTier:           representative.CustomerTier,
		SeatCount:              len(group.Members),
		CurrentCycleValueCents: representative.CustomerGroupCurrentPriceCents,
		Selectable:             true,
		Status:                 "hold",
		ReasonCode:             "manual_review",
		SuggestedBenefitType:   model.CustomerBenefitTypeManual,
	}
	if candidate.CurrentCycleValueCents <= 0 {
		for _, member := range group.Members {
			candidate.CurrentCycleValueCents += maxInt64(member.CurrentPriceCents, 0)
		}
	}
	for _, member := range group.Members {
		if candidate.CustomerEmail == "" && member.CustomerEmail != "" {
			candidate.CustomerEmail = member.CustomerEmail
		}
		if candidate.CustomerWechat == "" && member.CustomerWechat != "" {
			candidate.CustomerWechat = member.CustomerWechat
		}
		candidate.RenewalCount = maxInt(candidate.RenewalCount, member.RenewalCount)
		candidate.RelationshipDays = maxInt(candidate.RelationshipDays, member.RelationshipDays)
		candidate.NextDueDate = earlierNonEmptyDate(candidate.NextDueDate, member.NextDueDate)
		candidate.LastPaidDate = laterDate(candidate.LastPaidDate, member.LastPaidDate)
	}
	if strings.TrimSpace(candidate.CustomerEmail) != "" {
		candidate.DisplayName = candidate.CustomerEmail
	} else if strings.TrimSpace(candidate.CustomerWechat) != "" {
		candidate.DisplayName = candidate.CustomerWechat
	}

	today := cycle.StartOfDay(now)
	lastBenefit := model.CustomerBenefit{}
	if len(benefits) > 0 {
		lastBenefit = benefits[0]
		candidate.LastBenefitDate = lastBenefit.BenefitDate
		if deliveredAt, err := time.ParseInLocation("2006-01-02", lastBenefit.BenefitDate, cycle.Location); err == nil {
			candidate.NextEligibleDate = cycle.FormatDate(
				cycle.StartOfDay(deliveredAt).AddDate(0, 0, customerBenefitCooldownDays),
			)
		}
	}

	activeAfterSales := false
	recoveringAfterSales := false
	latestIncreaseDate := ""
	increaseAccepted := false
	for _, member := range group.Members {
		if member.BlockedCode == "after_sales" || member.BlockedCode == "account_banned" {
			activeAfterSales = true
		}
		if member.BlockedCode == "after_sales_recovery" {
			recoveringAfterSales = true
		}
		if member.PaidPeriodsAfterIncrease > 0 {
			increaseAccepted = true
			latestIncreaseDate = laterDate(latestIncreaseDate, member.LastPriceIncreaseDate)
		}
	}
	if activeAfterSales {
		candidate.Selectable = false
		candidate.Status = "blocked"
		candidate.ReasonCode = "service_in_progress"
		return candidate
	}
	if recoveringAfterSales && !hasBenefitAfter(
		benefits,
		model.CustomerBenefitTypeServiceRecovery,
		latestRecoveryStart(group.Members),
	) {
		markBenefitRecommendation(
			&candidate,
			today,
			"service_recovery",
			model.CustomerBenefitTypeServiceRecovery,
			today,
		)
		return candidate
	}
	if candidate.RenewalCount <= 0 {
		candidate.Status = "observe"
		candidate.ReasonCode = "first_cycle_observe"
		candidate.SuggestedBenefitType = model.CustomerBenefitTypeRenewalMilestone
		candidate.RecommendedDate = candidate.NextDueDate
		return candidate
	}
	if candidate.NextEligibleDate != "" && candidate.NextEligibleDate > cycle.FormatDate(today) {
		candidate.Status = "cooldown"
		candidate.ReasonCode = "benefit_cooldown"
		candidate.RecommendedDate = candidate.NextEligibleDate
		return candidate
	}
	if increaseAccepted && !hasBenefitAfter(
		benefits,
		model.CustomerBenefitTypePriceIncrease,
		latestIncreaseDate,
	) {
		markBenefitRecommendation(
			&candidate,
			today,
			"increase_accepted",
			model.CustomerBenefitTypePriceIncrease,
			today,
		)
		return candidate
	}
	if candidate.CustomerTier == "optimize" && candidate.SeatCount <= 1 {
		candidate.Status = "hold"
		candidate.ReasonCode = "optimize_no_subsidy"
		return candidate
	}

	recommendedAt := benefitDateBeforeNextRenewal(candidate.NextDueDate, today)
	reasonCode := "repeat_retention"
	benefitType := model.CustomerBenefitTypeLoyaltyCare
	if candidate.RenewalCount == 1 && !hasBenefitType(benefits, model.CustomerBenefitTypeRenewalMilestone) {
		reasonCode = "first_renewal"
		benefitType = model.CustomerBenefitTypeRenewalMilestone
		if lastPaidAt, err := time.ParseInLocation("2006-01-02", candidate.LastPaidDate, cycle.Location); err == nil &&
			today.Sub(cycle.StartOfDay(lastPaidAt)) <= 14*24*time.Hour {
			recommendedAt = today
		}
	} else if candidate.CustomerTier == "core" || candidate.SeatCount > 1 {
		reasonCode = "core_retention"
	}
	markBenefitRecommendation(&candidate, today, reasonCode, benefitType, recommendedAt)
	return candidate
}

func markBenefitRecommendation(
	candidate *CustomerBenefitCandidate,
	today time.Time,
	reasonCode string,
	benefitType string,
	recommendedAt time.Time,
) {
	if recommendedAt.IsZero() {
		recommendedAt = today
	}
	candidate.ReasonCode = reasonCode
	candidate.SuggestedBenefitType = benefitType
	candidate.RecommendedDate = cycle.FormatDate(recommendedAt)
	if cycle.StartOfDay(recommendedAt).After(today) {
		candidate.Status = "upcoming"
		return
	}
	candidate.Status = "recommended"
	candidate.Recommended = true
}

func benefitDateBeforeNextRenewal(nextDueDate string, today time.Time) time.Time {
	dueAt, err := time.ParseInLocation("2006-01-02", strings.TrimSpace(nextDueDate), cycle.Location)
	if err != nil {
		return today
	}
	recommendedAt := cycle.StartOfDay(dueAt).AddDate(0, 0, -customerBenefitLeadDays)
	if recommendedAt.Before(today) {
		return today
	}
	return recommendedAt
}

func latestRecoveryStart(members []PricingCandidate) string {
	latest := ""
	for _, member := range members {
		if member.BlockedCode != "after_sales_recovery" || member.NextReviewDate == "" {
			continue
		}
		reviewAt, err := time.ParseInLocation("2006-01-02", member.NextReviewDate, cycle.Location)
		if err != nil {
			continue
		}
		latest = laterDate(latest, cycle.FormatDate(reviewAt.AddDate(0, 0, -repricingAfterSalesRecoveryDays)))
	}
	return latest
}

func hasBenefitType(benefits []model.CustomerBenefit, benefitType string) bool {
	for _, benefit := range benefits {
		if benefit.BenefitType == benefitType {
			return true
		}
	}
	return false
}

func hasBenefitAfter(benefits []model.CustomerBenefit, benefitType string, date string) bool {
	for _, benefit := range benefits {
		if benefit.BenefitType == benefitType && (date == "" || benefit.BenefitDate >= date) {
			return true
		}
	}
	return false
}

func earlierNonEmptyDate(left string, right string) string {
	if left == "" || (right != "" && right < left) {
		return right
	}
	return left
}

func laterDate(left string, right string) string {
	if right > left {
		return right
	}
	return left
}

func sortCustomerBenefitCandidates(candidates []CustomerBenefitCandidate) {
	statusOrder := map[string]int{
		"recommended": 0,
		"upcoming":    1,
		"observe":     2,
		"cooldown":    3,
		"hold":        4,
		"blocked":     5,
	}
	sort.SliceStable(candidates, func(left int, right int) bool {
		leftOrder := statusOrder[candidates[left].Status]
		rightOrder := statusOrder[candidates[right].Status]
		if leftOrder != rightOrder {
			return leftOrder < rightOrder
		}
		if candidates[left].CurrentCycleValueCents != candidates[right].CurrentCycleValueCents {
			return candidates[left].CurrentCycleValueCents > candidates[right].CurrentCycleValueCents
		}
		return candidates[left].SubscriptionID < candidates[right].SubscriptionID
	})
}

func refundedBenefitBillIDs(cases []model.AfterSalesCase) map[int64]struct{} {
	refunded := make(map[int64]struct{})
	for _, caseItem := range cases {
		if caseItem.Status == model.AfterSalesStatusRefunded && caseItem.BillID > 0 {
			refunded[caseItem.BillID] = struct{}{}
		}
	}
	return refunded
}

func buildCustomerBenefitViews(
	benefits []model.CustomerBenefit,
	bills []model.Bill,
	refundedBillIDs map[int64]struct{},
	activeSubscriptions []model.Subscription,
	archivedSubscriptions []model.Subscription,
	now time.Time,
) []CustomerBenefitView {
	allSubscriptions := append(
		append(make([]model.Subscription, 0, len(activeSubscriptions)+len(archivedSubscriptions)), activeSubscriptions...),
		archivedSubscriptions...,
	)
	billsBySubscription := make(map[int64][]model.Bill)
	for _, bill := range bills {
		if _, refunded := refundedBillIDs[bill.ID]; refunded {
			continue
		}
		billsBySubscription[bill.SubscriptionID] = append(billsBySubscription[bill.SubscriptionID], bill)
	}
	views := make([]CustomerBenefitView, 0, len(benefits))
	for _, benefit := range benefits {
		outcome := customerBenefitOutcome(benefit, allSubscriptions, billsBySubscription, now)
		views = append(views, CustomerBenefitView{CustomerBenefit: benefit, Outcome: outcome})
	}
	return views
}

func customerBenefitOutcome(
	benefit model.CustomerBenefit,
	subscriptions []model.Subscription,
	billsBySubscription map[int64][]model.Bill,
	now time.Time,
) string {
	dueAt, err := time.ParseInLocation("2006-01-02", benefit.NextDueDateSnapshot, cycle.Location)
	if err != nil {
		return "pending"
	}
	matchingSubscriptionIDs := map[int64]struct{}{benefit.SubscriptionID: {}}
	email := normalizeCustomerIdentity(benefit.CustomerEmailSnapshot)
	wechat := normalizeCustomerIdentity(benefit.CustomerWechatSnapshot)
	for _, subscription := range subscriptions {
		if (email != "" && normalizeCustomerIdentity(subscription.CustomerEmail) == email) ||
			(wechat != "" && normalizeCustomerIdentity(subscription.CustomerWechat) == wechat) {
			matchingSubscriptionIDs[subscription.ID] = struct{}{}
		}
	}
	for subscriptionID := range matchingSubscriptionIDs {
		for _, bill := range billsBySubscription[subscriptionID] {
			if bill.DueDate >= benefit.NextDueDateSnapshot {
				return "renewed"
			}
		}
	}
	if cycle.StartOfDay(now).Before(cycle.StartOfDay(dueAt).AddDate(0, 0, customerBenefitOutcomeGrace)) {
		return "pending"
	}
	return "not_renewed"
}

func buildPredictionReadiness(
	activeSubscriptions []model.Subscription,
	archivedSubscriptions []model.Subscription,
	bills []model.Bill,
	afterSalesCases []model.AfterSalesCase,
	benefitViews []CustomerBenefitView,
) PredictionReadiness {
	refundedBillIDs := refundedBenefitBillIDs(afterSalesCases)
	billCounts := make(map[int64]int)
	for _, bill := range bills {
		if _, refunded := refundedBillIDs[bill.ID]; refunded {
			continue
		}
		billCounts[bill.SubscriptionID]++
	}
	involuntaryChurn := make(map[int64]struct{})
	for _, caseItem := range afterSalesCases {
		if caseItem.Source == model.AfterSalesSourceAccountBan {
			involuntaryChurn[caseItem.SubscriptionID] = struct{}{}
		}
	}

	readiness := PredictionReadiness{ActiveModel: "evidence_only"}
	for _, subscription := range activeSubscriptions {
		if subscription.BusinessType != model.SubscriptionBusinessTeam || subscription.IsResale {
			continue
		}
		paidPeriods := billCounts[subscription.ID]
		if paidPeriods == 1 {
			readiness.FirstCycleSubscriptionCount++
		}
		if paidPeriods > 1 {
			readiness.RepeatSubscriptionCount++
			readiness.RenewalSuccessCount += paidPeriods - 1
		}
	}
	for _, subscription := range archivedSubscriptions {
		if subscription.BusinessType != model.SubscriptionBusinessTeam || subscription.IsResale {
			continue
		}
		paidPeriods := billCounts[subscription.ID]
		if paidPeriods > 1 {
			readiness.RepeatSubscriptionCount++
			readiness.RenewalSuccessCount += paidPeriods - 1
		}
		if paidPeriods > 0 {
			if _, involuntary := involuntaryChurn[subscription.ID]; !involuntary {
				readiness.ChurnOutcomeCount++
			}
		}
	}
	readiness.RenewalOutcomeCount = readiness.RenewalSuccessCount + readiness.ChurnOutcomeCount
	allSubscriptions := append(
		append(make([]model.Subscription, 0, len(activeSubscriptions)+len(archivedSubscriptions)), activeSubscriptions...),
		archivedSubscriptions...,
	)
	readiness.RepeatCustomerCount = countRepeatCustomerGroups(allSubscriptions, billCounts)
	if readiness.RenewalOutcomeCount >= minimumBetaBinomialOutcomes {
		mean, low, high := betaPosteriorApproximation(
			readiness.RenewalSuccessCount,
			readiness.ChurnOutcomeCount,
		)
		readiness.ActiveModel = "beta_binomial"
		readiness.EstimatedRenewalPercent = &mean
		readiness.EstimateLowPercent = &low
		readiness.EstimateHighPercent = &high
	}

	evaluatedBenefits := 0
	for _, benefit := range benefitViews {
		if benefit.Outcome != "pending" {
			evaluatedBenefits++
		}
	}
	betaStatus := readinessStatus(readiness.RenewalOutcomeCount, minimumBetaBinomialOutcomes)
	survivalStatus := readinessStatus(readiness.RenewalOutcomeCount, minimumSurvivalOutcomes)
	survivalDetail := "need_outcomes"
	if readiness.RenewalOutcomeCount >= minimumSurvivalOutcomes &&
		readiness.ChurnOutcomeCount < minimumSurvivalChurns {
		survivalStatus = "collecting"
		survivalDetail = "need_churn_variation"
	} else if survivalStatus == "ready" {
		survivalDetail = "primary_later"
	}
	bgStatus := readinessStatus(readiness.RepeatCustomerCount, minimumBGNBDRepeatCustomers)
	upliftStatus := readinessStatus(evaluatedBenefits, minimumUpliftEvaluatedBenefits)
	upliftDetail := "need_benefit_outcomes"
	if upliftStatus == "ready" {
		upliftStatus = "needs_control"
		upliftDetail = "need_control_group"
	}
	readiness.Models = []ForecastModelReadiness{
		{
			Key:             "beta_binomial",
			Status:          betaStatus,
			CurrentSamples:  readiness.RenewalOutcomeCount,
			RequiredSamples: minimumBetaBinomialOutcomes,
			DetailCode:      "small_sample_primary",
		},
		{
			Key:             "discrete_survival",
			Status:          survivalStatus,
			CurrentSamples:  readiness.RenewalOutcomeCount,
			RequiredSamples: minimumSurvivalOutcomes,
			DetailCode:      survivalDetail,
		},
		{
			Key:             "bg_nbd",
			Status:          bgStatus,
			CurrentSamples:  readiness.RepeatCustomerCount,
			RequiredSamples: minimumBGNBDRepeatCustomers,
			DetailCode:      "value_auxiliary",
		},
		{
			Key:             "uplift",
			Status:          upliftStatus,
			CurrentSamples:  evaluatedBenefits,
			RequiredSamples: minimumUpliftEvaluatedBenefits,
			DetailCode:      upliftDetail,
		},
	}
	return readiness
}

func countRepeatCustomerGroups(
	subscriptions []model.Subscription,
	billCounts map[int64]int,
) int {
	teamSubscriptions := make([]model.Subscription, 0, len(subscriptions))
	for _, subscription := range subscriptions {
		if subscription.BusinessType == model.SubscriptionBusinessTeam && !subscription.IsResale {
			teamSubscriptions = append(teamSubscriptions, subscription)
		}
	}
	parents := make([]int, len(teamSubscriptions))
	for index := range parents {
		parents[index] = index
	}
	var find func(int) int
	find = func(index int) int {
		if parents[index] != index {
			parents[index] = find(parents[index])
		}
		return parents[index]
	}
	union := func(left int, right int) {
		leftRoot := find(left)
		rightRoot := find(right)
		if leftRoot != rightRoot {
			parents[rightRoot] = leftRoot
		}
	}
	byEmail := make(map[string]int)
	byWechat := make(map[string]int)
	for index, subscription := range teamSubscriptions {
		if email := normalizeCustomerIdentity(subscription.CustomerEmail); email != "" {
			if previous, exists := byEmail[email]; exists {
				union(index, previous)
			} else {
				byEmail[email] = index
			}
		}
		if wechat := normalizeCustomerIdentity(subscription.CustomerWechat); wechat != "" {
			if previous, exists := byWechat[wechat]; exists {
				union(index, previous)
			} else {
				byWechat[wechat] = index
			}
		}
	}
	repeatGroups := make(map[int]struct{})
	for index, subscription := range teamSubscriptions {
		if billCounts[subscription.ID] > 1 {
			repeatGroups[find(index)] = struct{}{}
		}
	}
	return len(repeatGroups)
}

func readinessStatus(current int, required int) string {
	if current >= required {
		return "ready"
	}
	return "collecting"
}

// betaPosteriorApproximation uses Jeffreys' Beta(0.5, 0.5) prior. The interval
// is a bounded normal approximation to the posterior and is only exposed after
// the minimum sample gate, avoiding false precision from the first few cycles.
func betaPosteriorApproximation(successes int, failures int) (int, int, int) {
	alpha := float64(successes) + 0.5
	beta := float64(failures) + 0.5
	mean := alpha / (alpha + beta)
	variance := alpha * beta / (math.Pow(alpha+beta, 2) * (alpha + beta + 1))
	margin := 1.96 * math.Sqrt(variance)
	low := math.Max(0, mean-margin)
	high := math.Min(1, mean+margin)
	return int(math.Round(mean * 100)), int(math.Round(low * 100)), int(math.Round(high * 100))
}

func (service *SubscriptionService) RecordCustomerBenefits(
	input RecordCustomerBenefitsInput,
) (int, error) {
	for _, subscriptionID := range input.SubscriptionIDs {
		if subscriptionID <= 0 {
			return 0, fmt.Errorf("包含无效的订阅 ID")
		}
	}
	subscriptionIDs := uniquePositiveIDs(input.SubscriptionIDs)
	if len(subscriptionIDs) == 0 {
		return 0, fmt.Errorf("请至少选择一位客户")
	}
	if len(subscriptionIDs) > maximumBenefitSelection {
		return 0, fmt.Errorf("单次最多登记 %d 位客户", maximumBenefitSelection)
	}
	benefitType := strings.TrimSpace(input.BenefitType)
	if !validCustomerBenefitType(benefitType) {
		return 0, fmt.Errorf("福利分类无效")
	}
	benefitName := strings.TrimSpace(input.BenefitName)
	if benefitName == "" {
		return 0, fmt.Errorf("请填写福利名称")
	}
	if len([]rune(benefitName)) > maximumBenefitNameRunes {
		return 0, fmt.Errorf("福利名称不能超过 %d 个字符", maximumBenefitNameRunes)
	}
	note := strings.TrimSpace(input.Note)
	if len([]rune(note)) > maximumBenefitNoteRunes {
		return 0, fmt.Errorf("备注不能超过 %d 个字符", maximumBenefitNoteRunes)
	}
	actualCostCents, err := parseOptionalNonNegativeYuan(input.ActualCostYuan)
	if err != nil || actualCostCents > maximumBenefitAmountCents {
		return 0, fmt.Errorf("单人实际成本无效")
	}
	perceivedValueCents, err := parseOptionalNonNegativeYuan(input.PerceivedValueYuan)
	if err != nil || perceivedValueCents > maximumBenefitAmountCents {
		return 0, fmt.Errorf("单人感知价值无效")
	}
	benefitDate := strings.TrimSpace(input.BenefitDate)
	benefitAt, err := time.ParseInLocation("2006-01-02", benefitDate, cycle.Location)
	if err != nil || cycle.StartOfDay(benefitAt).After(cycle.StartOfDay(service.now())) {
		return 0, fmt.Errorf("发放日期无效，不能晚于今天")
	}

	pricingCandidates, err := service.buildPricingCandidates(nil, 0)
	if err != nil {
		return 0, err
	}
	care, err := service.buildCustomerCare(pricingCandidates)
	if err != nil {
		return 0, err
	}
	candidatesByID := make(map[int64]CustomerBenefitCandidate, len(care.Candidates))
	for _, candidate := range care.Candidates {
		candidatesByID[candidate.SubscriptionID] = candidate
	}

	createdAt := service.now().UTC()
	batchID := fmt.Sprintf("care-%d", createdAt.UnixNano())
	records := make([]model.CustomerBenefit, 0, len(subscriptionIDs))
	for _, subscriptionID := range subscriptionIDs {
		candidate, exists := candidatesByID[subscriptionID]
		if !exists || !candidate.Selectable {
			return 0, fmt.Errorf("所选客户状态已变化，请刷新后重试")
		}
		subscription, getErr := service.Store.GetSubscription(subscriptionID)
		if getErr != nil {
			return 0, fmt.Errorf("所选客户状态已变化，请刷新后重试")
		}
		records = append(records, model.CustomerBenefit{
			BatchID:                   batchID,
			SubscriptionID:            subscriptionID,
			BenefitType:               benefitType,
			BenefitName:               benefitName,
			ActualCostCents:           actualCostCents,
			PerceivedValueCents:       perceivedValueCents,
			BenefitDate:               benefitDate,
			NextDueDateSnapshot:       candidate.NextDueDate,
			CustomerEmailSnapshot:     candidate.CustomerEmail,
			CustomerWechatSnapshot:    candidate.CustomerWechat,
			CustomerTierSnapshot:      candidate.CustomerTier,
			CustomerGroupSizeSnapshot: candidate.SeatCount,
			CurrentPriceCentsSnapshot: subscription.PricePerPersonCents,
			RenewalCountSnapshot:      candidate.RenewalCount,
			RecommendationCode:        candidate.ReasonCode,
			Note:                      note,
			CreatedAt:                 createdAt,
		})
	}
	if err := service.Store.CreateCustomerBenefits(records); err != nil {
		switch {
		case errors.Is(err, db.ErrCustomerBenefitAlreadyRecorded):
			return 0, fmt.Errorf("所选客户今天已登记过相同福利")
		case errors.Is(err, sql.ErrNoRows):
			return 0, fmt.Errorf("所选客户状态已变化，请刷新后重试")
		default:
			return 0, err
		}
	}
	return len(records), nil
}

func parseOptionalNonNegativeYuan(raw string) (int64, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, nil
	}
	value, err := cycle.ParseYuanToCents(raw)
	if err != nil || value < 0 {
		return 0, fmt.Errorf("invalid non-negative amount")
	}
	return value, nil
}

func validCustomerBenefitType(value string) bool {
	switch value {
	case model.CustomerBenefitTypeRenewalMilestone,
		model.CustomerBenefitTypeLoyaltyCare,
		model.CustomerBenefitTypePriceIncrease,
		model.CustomerBenefitTypeServiceRecovery,
		model.CustomerBenefitTypeManual:
		return true
	default:
		return false
	}
}

func uniquePositiveIDs(values []int64) []int64 {
	seen := make(map[int64]struct{}, len(values))
	unique := make([]int64, 0, len(values))
	for _, value := range values {
		if value <= 0 {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		unique = append(unique, value)
	}
	return unique
}
