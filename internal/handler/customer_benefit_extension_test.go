package handler

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"carpool-notify/internal/model"
	"carpool-notify/internal/service"
)

func createBenefitExtensionTarget(t *testing.T, server *Server) service.SubscriptionView {
	t.Helper()
	accountID, err := server.Service.CreateAccount(service.CreateAccountInput{
		Name: "benefit-owner@example.com", OpenedAt: "2026-08-01", SeatNames: []string{"seat-1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	seats, err := server.Service.Store.ListSeatsByAccount(accountID)
	if err != nil || len(seats) != 1 {
		t.Fatalf("seats = %#v, %v", seats, err)
	}
	subscriptionID, err := server.Service.Create(service.CreateInput{
		Name: "benefit-customer", CustomerEmail: "benefit-customer@example.com",
		PriceYuan: "100.00", CronExpr: "interval:30d", BoardedAt: "2026-08-01", SeatID: seats[0].ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := server.Service.Store.SetDuePaid(subscriptionID, "2026-08-01", true, 10000); err != nil {
		t.Fatal(err)
	}
	views, err := server.Service.ListView()
	if err != nil || len(views) != 1 {
		t.Fatalf("review views = %#v, %v", views, err)
	}
	return views[0]
}

func TestCustomerBenefitExtensionRevisionHTTPStatus(t *testing.T) {
	server, router := subscriptionEditTestServer(t)
	router.POST("/goals/customer-benefits", server.postGoalCustomerBenefits)
	router.PUT("/goals/customer-benefits/:id/extension", server.putGoalCustomerBenefitExtension)
	view := createBenefitExtensionTarget(t, server)
	response := subscriptionEditRequest(t, router, http.MethodPost, "/goals/customer-benefits",
		benefitExtensionPayload(view, "handler-extension-revision-source"))
	if response.Code != http.StatusOK {
		t.Fatalf("create response: %d %s", response.Code, response.Body.String())
	}
	benefits, err := server.Service.Store.ListCustomerBenefits()
	if err != nil || len(benefits) != 1 {
		t.Fatalf("benefits = %#v, %v", benefits, err)
	}
	path := fmt.Sprintf("/goals/customer-benefits/%d/extension", benefits[0].ID)
	operationKey := "handler-extension-revision-operation"
	response = subscriptionEditRequest(t, router, http.MethodPut, path, map[string]any{
		"extension_days": 11, "reason": "补足服务时间", "operation_key": operationKey,
	})
	if response.Code != http.StatusOK {
		t.Fatalf("edit response: %d %s", response.Code, response.Body.String())
	}
	response = subscriptionEditRequest(t, router, http.MethodPut, path, map[string]any{
		"extension_days": 12, "reason": "补足服务时间", "operation_key": operationKey,
	})
	if response.Code != http.StatusConflict {
		t.Fatalf("conflict response: %d %s", response.Code, response.Body.String())
	}
}

func benefitExtensionPayload(view service.SubscriptionView, operationKey string) map[string]any {
	return map[string]any{
		"subscription_ids": []int64{view.Subscription.ID},
		"benefit_type":     model.CustomerBenefitTypeExtension,
		"operation_key":    operationKey,
		"extension_days":   9,
		"benefit_date":     "2026-08-20",
		"extension_review_snapshots": []map[string]any{{
			"subscription_id":     view.Subscription.ID,
			"expected_updated_at": view.Subscription.UpdatedAt.Format(time.RFC3339Nano),
			"expected_due_date":   view.NextDueDate,
		}},
	}
}

func TestPostCustomerBenefitExtensionUsesBrowserReviewSnapshot(t *testing.T) {
	for _, test := range []struct {
		name  string
		stale bool
	}{
		{name: "current snapshot succeeds"},
		{name: "payment after dialog opened is rejected", stale: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			server, router := subscriptionEditTestServer(t)
			router.POST("/goals/customer-benefits", server.postGoalCustomerBenefits)
			view := createBenefitExtensionTarget(t, server)
			payload := benefitExtensionPayload(view, "handler-benefit-extension-"+strings.ReplaceAll(test.name, " ", "-"))
			if test.stale {
				if err := server.Service.SetDuePaid(view.Subscription.ID, view.NextDueDate, true); err != nil {
					t.Fatal(err)
				}
			}

			response := subscriptionEditRequest(t, router, http.MethodPost, "/goals/customer-benefits", payload)
			if test.stale {
				if response.Code != http.StatusConflict || !strings.Contains(response.Body.String(), "刷新") {
					t.Fatalf("stale response: %d %s", response.Code, response.Body.String())
				}
				benefits, err := server.Service.Store.ListCustomerBenefits()
				if err != nil || len(benefits) != 0 {
					t.Fatalf("stale benefits = %#v, %v", benefits, err)
				}
				events, err := server.Service.Store.ListSubscriptionDueExtensions()
				if err != nil || len(events) != 0 {
					t.Fatalf("stale extensions = %#v, %v", events, err)
				}
				return
			}
			if response.Code != http.StatusOK {
				t.Fatalf("current response: %d %s", response.Code, response.Body.String())
			}
			events, err := server.Service.Store.ListSubscriptionDueExtensions()
			if err != nil || len(events) != 1 || events[0].PreviousEffectiveDueDate != view.NextDueDate {
				t.Fatalf("current extensions = %#v, %v", events, err)
			}
		})
	}
}

func TestPostCustomerBenefitExtensionLegacyAliasesRequireReviewSnapshot(t *testing.T) {
	aliases := []string{
		model.CustomerBenefitTypeManual,
		model.CustomerBenefitTypeRenewalMilestone,
		model.CustomerBenefitTypeLoyaltyCare,
		model.CustomerBenefitTypeServiceRecovery,
	}
	for _, alias := range aliases {
		for _, stale := range []bool{false, true} {
			name := alias + "/missing-snapshot"
			if stale {
				name = alias + "/payment-after-dialog-opened"
			}
			t.Run(name, func(t *testing.T) {
				server, router := subscriptionEditTestServer(t)
				router.POST("/goals/customer-benefits", server.postGoalCustomerBenefits)
				view := createBenefitExtensionTarget(t, server)
				payload := benefitExtensionPayload(view, "handler-benefit-alias-"+strings.ReplaceAll(name, "/", "-"))
				payload["benefit_type"] = alias
				if stale {
					if err := server.Service.SetDuePaid(view.Subscription.ID, view.NextDueDate, true); err != nil {
						t.Fatal(err)
					}
				} else {
					delete(payload, "extension_review_snapshots")
				}

				response := subscriptionEditRequest(t, router, http.MethodPost, "/goals/customer-benefits", payload)
				wantStatus := http.StatusBadRequest
				if stale {
					wantStatus = http.StatusConflict
				}
				if response.Code != wantStatus || !strings.Contains(response.Body.String(), "刷新") {
					t.Fatalf("legacy extension response: %d %s", response.Code, response.Body.String())
				}
				benefits, err := server.Service.Store.ListCustomerBenefits()
				if err != nil || len(benefits) != 0 {
					t.Fatalf("legacy extension benefits = %#v, %v", benefits, err)
				}
				events, err := server.Service.Store.ListSubscriptionDueExtensions()
				if err != nil || len(events) != 0 {
					t.Fatalf("legacy extension events = %#v, %v", events, err)
				}
			})
		}
	}
}
