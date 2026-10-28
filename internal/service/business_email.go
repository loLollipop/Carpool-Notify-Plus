package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"carpool-notify/internal/cycle"
	"carpool-notify/internal/db"
	"carpool-notify/internal/model"
	"carpool-notify/internal/notify"
)

// Keep the stored renewal setting for backwards compatibility. Both public
// application forms now notify this same private operator address.
func (service *SubscriptionService) sendApplicationAlert(title, body string) error {
	recipient, err := service.GetRenewalApplicationAlertEmail()
	if err != nil || recipient == "" {
		return err
	}
	_, registry := service.runtimeConfigSnapshot()
	sender, ok := registry.Get(model.ChannelSMTP)
	if !ok {
		return fmt.Errorf("申请提醒邮箱已设置，但 SMTP 发送器未配置")
	}
	addressed, ok := sender.(smtpAddressedSender)
	if !ok {
		return fmt.Errorf("SMTP 发送器不支持指定申请提醒收件人")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return addressed.SendTo(ctx, []string{recipient}, title, body)
}

func (service *SubscriptionService) sendRedemptionApplicationAlert(application model.RedemptionApplication) error {
	return service.sendApplicationAlert("[Carpool Notify Plus] 新的兑换申请", strings.Join([]string{
		"有一条新的兑换申请等待处理。", "",
		"客户邮箱：" + application.CustomerEmail,
		"联系方式：" + application.CustomerContact,
		"客户备注：" + application.RequestNote,
		"提交时间：" + cycle.FormatDateTime(service.now()), "",
		"请前往管理后台的“兑换申请”核对并处理邀请。",
	}, "\n"))
}

type benefitEmail struct {
	subscriptionID int64
	recipient      string
	title          string
	body           string
	data           BusinessEmailTemplateData
}

// Outbox rows are written in the business transaction; SMTP runs after commit.
// Each subscription uses its own email, never a grouped customer's contact or
// the SMTP sender's default operator recipients.
func (service *SubscriptionService) benefitEmailBatch(build func() []benefitEmail) db.BusinessEmailBatch {
	// Load settings before opening a transaction; Build must use only snapshots.
	body, err := service.GetCustomerBenefitEmailTemplate()
	return service.benefitEmailBatchWithTemplate(build, body, err)
}

func (service *SubscriptionService) benefitEmailBatchWithTemplate(build func() []benefitEmail, body string, templateErr error) db.BusinessEmailBatch {
	_, registry := service.runtimeConfigSnapshot()
	if _, ok := registry.Get(model.ChannelSMTP); !ok {
		return db.BusinessEmailBatch{}
	}
	now := service.now()
	return db.BusinessEmailBatch{Now: now, BuildChecked: func() ([]db.BusinessEmail, error) {
		messages := build()
		pending := make([]db.BusinessEmail, 0, len(messages))
		for _, message := range messages {
			recipient, err := normalizeCustomerEmail(message.recipient)
			if err != nil {
				return nil, err
			}
			if recipient == "" {
				continue
			}
			if templateErr != nil {
				return nil, templateErr
			}
			data := message.data
			data.CustomerEmail, data.SubscriptionID = recipient, message.subscriptionID
			data.OperationTime = cycle.FormatDateTime(now)
			rendered, err := renderBusinessEmailTemplate(body, data)
			if err != nil {
				return nil, err
			}
			pending = append(pending, db.BusinessEmail{SubscriptionID: message.subscriptionID, Recipient: recipient, Title: message.title, Body: rendered})
		}
		return pending, nil
	}}
}

func (service *SubscriptionService) priceBenefitEmailBatch(previous, updated model.Subscription) db.BusinessEmailBatch {
	return service.benefitEmailBatch(func() []benefitEmail {
		if message, needed := scheduledPriceBenefitEmail(previous, updated, cycle.FormatDate(service.now())); needed {
			return []benefitEmail{message}
		}
		return nil
	})
}

func (service *SubscriptionService) extensionRevisionEmailBatch(input ReviseCustomerBenefitExtensionInput, result db.ReviseDueExtensionResult, body string, templateErr error) db.BusinessEmailBatch {
	return service.benefitEmailBatchWithTemplate(func() []benefitEmail {
		kind, benefitType := "customer_benefit_revision", "extension_revision"
		if input.ExtensionDays == 0 {
			kind, benefitType = "customer_benefit_revoke", "extension_revoke"
		}
		return []benefitEmail{{subscriptionID: result.SubscriptionID, recipient: result.CustomerEmail,
			title: BusinessEmailSubject(kind), data: BusinessEmailTemplateData{
				BenefitType: benefitType, BenefitDate: cycle.FormatDate(service.now()), PreviousExtensionDays: result.PreviousDays,
				ExtensionDays: input.ExtensionDays, PreviousDueDate: result.PreviousDueDate, NextDueDate: result.EffectiveDueDate,
			}}}
	}, body, templateErr)
}

// ProcessBusinessEmails runs on each scheduler tick. A time budget bounds one
// tick, while unstarted jobs remain durable for subsequent ticks and restarts.
func (service *SubscriptionService) ProcessBusinessEmails(ctx context.Context) error {
	service.dueNotificationMu.Lock()
	defer service.dueNotificationMu.Unlock()
	if ctx.Err() != nil {
		return nil
	}
	var failures []error
	if err := service.Store.DeleteExpiredBusinessEmailHistory(ctx, service.now()); err != nil {
		failures = append(failures, fmt.Errorf("clean business email history: %w", err))
	}
	_, registry := service.runtimeConfigSnapshot()
	sender, ok := registry.Get(model.ChannelSMTP)
	if !ok {
		return errors.Join(failures...)
	}
	batchCtx, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()
	for batchCtx.Err() == nil {
		messages, err := service.Store.PendingBusinessEmails(service.now(), 8)
		if err != nil {
			return errors.Join(append(failures, err)...)
		}
		if len(messages) == 0 {
			break
		}
		var workers sync.WaitGroup
		var failureMu sync.Mutex
		for _, message := range messages {
			workers.Add(1)
			go func(message db.BusinessEmail) {
				defer workers.Done()
				if batchCtx.Err() != nil {
					return
				}
				sendErr := service.deliverBenefitEmail(batchCtx, sender, benefitEmail{
					subscriptionID: message.SubscriptionID, recipient: message.Recipient,
					title: message.Title, body: message.Body,
				})
				persistErr := service.Store.FinishBusinessEmail(message.ID, message.AttemptCount+1, sendErr, service.now())
				if err := errors.Join(sendErr, persistErr); err != nil {
					failureMu.Lock()
					failures = append(failures, fmt.Errorf("benefit email %d: %w", message.ID, err))
					failureMu.Unlock()
				}
			}(message)
		}
		workers.Wait()
		if len(failures) > 0 {
			break
		}
	}
	return errors.Join(failures...)
}

func (service *SubscriptionService) deliverBenefitEmail(batchCtx context.Context, sender notify.Sender, message benefitEmail) error {
	ctx, cancel := context.WithTimeout(batchCtx, 10*time.Second)
	defer cancel()
	if err := ctx.Err(); err != nil {
		return err
	}
	if addressed, ok := sender.(smtpHTMLAddressedSender); ok {
		kind := "customer_benefit_extension"
		if strings.Contains(message.title, "续费成功") {
			kind = "customer_renewal_success"
		}
		return addressed.SendHTMLTo(ctx, []string{message.recipient}, message.title, message.body,
			BusinessEmailHTML(kind, message.body, message.title))
	}
	if addressed, ok := sender.(smtpAddressedSender); ok {
		return addressed.SendTo(ctx, []string{message.recipient}, message.title, message.body)
	}
	return fmt.Errorf("SMTP 发送器不支持指定福利通知收件人")
}

func scheduledPriceBenefitEmail(previous, updated model.Subscription, benefitDate string) (benefitEmail, bool) {
	if updated.BusinessType != model.SubscriptionBusinessTeam || updated.IsResale || updated.SeatID <= 0 ||
		updated.NextPriceCents == nil || *updated.NextPriceCents >= updated.PricePerPersonCents ||
		(previous.NextPriceCents != nil && *previous.NextPriceCents == *updated.NextPriceCents &&
			previous.PricePerPersonCents == updated.PricePerPersonCents &&
			previous.NextPriceEffectiveDueDate == updated.NextPriceEffectiveDueDate) {
		return benefitEmail{}, false
	}
	return customerBenefitEmail(model.CustomerBenefit{
		BenefitType: model.CustomerBenefitTypePriceDiscount, BenefitDate: benefitDate,
		PriceBeforeCents: updated.PricePerPersonCents, PriceAfterCents: *updated.NextPriceCents,
		PriceEffectiveDueDate: updated.NextPriceEffectiveDueDate,
	}, updated, ""), true
}

func customerBenefitEmail(record model.CustomerBenefit, subscription model.Subscription, previousDueDate string) benefitEmail {
	message := benefitEmail{subscriptionID: subscription.ID, recipient: subscription.CustomerEmail,
		data: BusinessEmailTemplateData{BenefitType: record.BenefitType, BenefitDate: record.BenefitDate}}
	if record.BenefitType == model.CustomerBenefitTypeExtension {
		message.title = BusinessEmailSubject("customer_benefit_extension")
		previous, _ := time.ParseInLocation("2006-01-02", previousDueDate, cycle.Location)
		message.data.ExtensionDays = record.ExtensionDays
		message.data.PreviousDueDate = previousDueDate
		message.data.NextDueDate = cycle.FormatDate(previous.AddDate(0, 0, record.ExtensionDays))
	} else {
		message.title = BusinessEmailSubject("customer_benefit_discount")
		message.data.PreviousPrice = cycle.FormatCents(record.PriceBeforeCents)
		message.data.DiscountAmount = cycle.FormatCents(record.PriceBeforeCents - record.PriceAfterCents)
		message.data.NewPrice = cycle.FormatCents(record.PriceAfterCents)
		message.data.EffectiveDate = record.PriceEffectiveDueDate
	}
	return message
}
