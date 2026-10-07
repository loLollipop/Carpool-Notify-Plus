package service

import (
	"context"
	"errors"
	"fmt"
	"log"
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
}

// Outbox rows are written in the business transaction; SMTP runs after commit.
// Each subscription uses its own email, never a grouped customer's contact or
// the SMTP sender's default operator recipients.
func (service *SubscriptionService) benefitEmailBatch(build func() []benefitEmail) db.BusinessEmailBatch {
	_, registry := service.runtimeConfigSnapshot()
	_, ok := registry.Get(model.ChannelSMTP)
	if !ok {
		// Sandbox services deliberately have no sender registry.
		return db.BusinessEmailBatch{}
	}
	now := service.now()
	return db.BusinessEmailBatch{Now: now, Build: func() []db.BusinessEmail {
		messages := build()
		pending := make([]db.BusinessEmail, 0, len(messages))
		for _, message := range messages {
			recipient, err := normalizeCustomerEmail(message.recipient)
			if err != nil {
				log.Printf("queue benefit email for subscription %d: %v", message.subscriptionID, err)
				continue
			}
			if recipient == "" {
				continue
			}
			pending = append(pending, db.BusinessEmail{SubscriptionID: message.subscriptionID,
				Recipient: recipient, Title: message.title, Body: message.body + "\n\n操作时间：" + cycle.FormatDateTime(now) +
					"\n本邮件记录上述时间的福利操作，后续如有变更，请以更新的通知为准。"})
		}
		return pending
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

func (service *SubscriptionService) extensionRevisionEmailBatch(input ReviseCustomerBenefitExtensionInput, result db.ReviseDueExtensionResult) db.BusinessEmailBatch {
	return service.benefitEmailBatch(func() []benefitEmail {
		title := "[Carpool Notify Plus] 延期福利修改通知"
		intro := "您好，您的延期福利已修改，请以本邮件中的到期日期为准。"
		if input.ExtensionDays == 0 {
			title = "[Carpool Notify Plus] 延期福利撤回通知"
			intro = "您好，您的延期福利已撤回，请以本邮件中的到期日期为准。"
		}
		return []benefitEmail{{
			subscriptionID: result.SubscriptionID, recipient: result.CustomerEmail, title: title,
			body: strings.Join([]string{intro, "", "客户邮箱：" + result.CustomerEmail,
				fmt.Sprintf("订阅编号：#%d", result.SubscriptionID),
				fmt.Sprintf("原延期天数：%d 天", result.PreviousDays),
				fmt.Sprintf("调整后延期天数：%d 天", input.ExtensionDays),
				"原到期日期：" + result.PreviousDueDate, "调整后到期日期：" + result.EffectiveDueDate,
				"", "如有疑问，请联系管理员。"}, "\n"),
		}}
	})
}

// ProcessBusinessEmails runs on each scheduler tick. A time budget bounds one
// tick, while unstarted jobs remain durable for subsequent ticks and restarts.
func (service *SubscriptionService) ProcessBusinessEmails(ctx context.Context) error {
	service.dueNotificationMu.Lock()
	defer service.dueNotificationMu.Unlock()
	_, registry := service.runtimeConfigSnapshot()
	sender, ok := registry.Get(model.ChannelSMTP)
	if !ok {
		return nil
	}
	batchCtx, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()
	var failures []error
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
		return addressed.SendHTMLTo(ctx, []string{message.recipient}, message.title, message.body,
			notify.BuildCustomerBenefitEmailHTML(message.body, message.title))
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
	message := benefitEmail{subscriptionID: subscription.ID, recipient: subscription.CustomerEmail}
	lines := []string{"您好，您的 ChatGPT Team 服务已获赠福利。", "",
		"客户邮箱：" + subscription.CustomerEmail,
		fmt.Sprintf("订阅编号：#%d", subscription.ID),
		"发放日期：" + record.BenefitDate,
	}
	if record.BenefitType == model.CustomerBenefitTypeExtension {
		message.title = "[Carpool Notify Plus] 赠送延期福利通知"
		previous, _ := time.ParseInLocation("2006-01-02", previousDueDate, cycle.Location)
		lines = append(lines,
			fmt.Sprintf("赠送天数：%d 天", record.ExtensionDays),
			"原到期日期："+previousDueDate,
			"调整后到期日期："+cycle.FormatDate(previous.AddDate(0, 0, record.ExtensionDays)),
			"", "本次延期已生效，无需额外付款。")
	} else {
		message.title = "[Carpool Notify Plus] 降价福利通知"
		lines = append(lines,
			"原每期价格：¥"+cycle.FormatCents(record.PriceBeforeCents),
			"每期优惠：¥"+cycle.FormatCents(record.PriceBeforeCents-record.PriceAfterCents),
			"调整后每期价格：¥"+cycle.FormatCents(record.PriceAfterCents),
			"生效日期："+record.PriceEffectiveDueDate,
			"", "优惠价从上述日期对应的续费账期开始使用，此后每期均按优惠价续费，直至另行调整。")
	}
	message.body = strings.Join(lines, "\n")
	return message
}
