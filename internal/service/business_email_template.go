package service

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"reflect"
	"strings"
	"text/template"
	"text/template/parse"
	"time"

	"carpool-notify/internal/cycle"
	"carpool-notify/internal/db"
	"carpool-notify/internal/model"
	"carpool-notify/internal/notify"
)

// BusinessEmailTemplateData deliberately excludes private notes and internal costs.
type BusinessEmailTemplateData struct {
	CustomerEmail                                          string
	SubscriptionID                                         int64
	OperationTime                                          string
	AmountPaid, PeriodStart, PeriodEnd, CycleDesc          string
	BenefitType, BenefitDate                               string
	ExtensionDays, PreviousExtensionDays                   int
	PreviousDueDate, NextDueDate                           string
	PreviousPrice, DiscountAmount, NewPrice, EffectiveDate string
}

func IsBusinessEmailTemplateKind(kind string) bool {
	switch kind {
	case "customer_renewal_success", "customer_benefit_extension", "customer_benefit_discount", "customer_benefit_revision", "customer_benefit_revoke":
		return true
	}
	return false
}

func BusinessEmailSubject(kind string) string {
	switch kind {
	case "customer_renewal_success":
		return "[Carpool Notify Plus] 续费成功通知"
	case "customer_benefit_extension":
		return "[Carpool Notify Plus] 赠送延期福利通知"
	case "customer_benefit_discount":
		return "[Carpool Notify Plus] 降价福利通知"
	case "customer_benefit_revision":
		return "[Carpool Notify Plus] 延期福利修改通知"
	default:
		return "[Carpool Notify Plus] 延期福利撤回通知"
	}
}

func BusinessEmailHTML(kind, body, title string) string {
	if kind == "customer_renewal_success" {
		return notify.BuildCustomerRenewalSuccessEmailHTML(body, title)
	}
	return notify.BuildCustomerBenefitEmailHTML(body, title)
}

func (service *SubscriptionService) businessEmailTemplate(key, fallback string) (string, error) {
	raw, err := service.Store.GetSetting(key)
	if err == sql.ErrNoRows || (err == nil && strings.TrimSpace(raw) == "") {
		return fallback, nil
	}
	return raw, err
}
func (service *SubscriptionService) GetRenewalSuccessCustomerEmailTemplate() (string, error) {
	return service.businessEmailTemplate(model.SettingRenewalSuccessCustomerEmailTemplate, model.DefaultRenewalSuccessCustomerEmailTemplate)
}
func (service *SubscriptionService) GetCustomerBenefitEmailTemplate() (string, error) {
	return service.businessEmailTemplate(model.SettingCustomerBenefitEmailTemplate, model.DefaultCustomerBenefitEmailTemplate)
}

type BusinessEmailTemplateSettings struct {
	RenewalSuccess  *string
	CustomerBenefit *string
}

func businessEmailSettingValues(options []BusinessEmailTemplateSettings) (map[string]string, error) {
	values := map[string]string{}
	for _, option := range options {
		for key, body := range map[string]*string{model.SettingRenewalSuccessCustomerEmailTemplate: option.RenewalSuccess, model.SettingCustomerBenefitEmailTemplate: option.CustomerBenefit} {
			if body == nil {
				continue
			}
			normalized := strings.TrimSpace(*body)
			if err := validateBusinessEmailTemplate(normalized); err != nil {
				return nil, err
			}
			values[key] = normalized
		}
	}
	return values, nil
}

func renderBusinessEmailTemplate(body string, data BusinessEmailTemplateData) (string, error) {
	parsed, err := template.New("business_email").Option("missingkey=error").Parse(body)
	if err != nil {
		return "", fmt.Errorf("业务邮件模板语法错误: %w", err)
	}
	var output bytes.Buffer
	if err := parsed.Execute(&output, data); err != nil {
		return "", fmt.Errorf("业务邮件模板执行错误: %w", err)
	}
	return output.String(), nil
}
func validateBusinessEmailTemplate(body string) error {
	if strings.TrimSpace(body) == "" {
		return fmt.Errorf("业务邮件模板不能为空")
	}
	parsed, err := template.New("business_email").Parse(body)
	if err != nil {
		return fmt.Errorf("业务邮件模板语法错误: %w", err)
	}
	fields := reflect.TypeOf(BusinessEmailTemplateData{})
	var walk func(parse.Node) error
	walk = func(node parse.Node) error {
		if node == nil || reflect.ValueOf(node).IsNil() {
			return nil
		}
		var children []parse.Node
		switch n := node.(type) {
		case *parse.ListNode:
			children = append(children, n.Nodes...)
		case *parse.ActionNode:
			children = append(children, n.Pipe)
		case *parse.IfNode:
			children = append(children, n.Pipe, n.List, n.ElseList)
		case *parse.RangeNode:
			children = append(children, n.Pipe, n.List, n.ElseList)
		case *parse.WithNode:
			children = append(children, n.Pipe, n.List, n.ElseList)
		case *parse.TemplateNode:
			children = append(children, n.Pipe)
		case *parse.PipeNode:
			for _, c := range n.Cmds {
				children = append(children, c)
			}
		case *parse.CommandNode:
			children = append(children, n.Args...)
		case *parse.ChainNode:
			return fmt.Errorf("业务邮件模板不支持嵌套字段")
		case *parse.VariableNode:
			if len(n.Ident) > 1 {
				if len(n.Ident) != 2 {
					return fmt.Errorf("业务邮件模板不支持嵌套字段")
				}
				if _, ok := fields.FieldByName(n.Ident[1]); !ok {
					return fmt.Errorf("业务邮件模板包含未知字段: %s", n.Ident[1])
				}
			}
		case *parse.FieldNode:
			if len(n.Ident) != 1 {
				return fmt.Errorf("业务邮件模板不支持嵌套字段")
			}
			if _, ok := fields.FieldByName(n.Ident[0]); !ok {
				return fmt.Errorf("业务邮件模板包含未知字段: %s", n.Ident[0])
			}
		}
		for _, child := range children {
			if err := walk(child); err != nil {
				return err
			}
		}
		return nil
	}
	for _, entry := range parsed.Templates() {
		if err := walk(entry.Tree.Root); err != nil {
			return err
		}
	}
	for _, kind := range []string{"customer_renewal_success", "customer_benefit_extension", "customer_benefit_discount", "customer_benefit_revision", "customer_benefit_revoke"} {
		if _, err := renderBusinessEmailTemplate(body, sampleBusinessEmailData(kind, "customer@example.com", "2026-10-28 12:00:00")); err != nil {
			return err
		}
	}
	return nil
}
func sampleBusinessEmailData(kind, recipient, operationTime string) BusinessEmailTemplateData {
	data := BusinessEmailTemplateData{CustomerEmail: recipient, SubscriptionID: 1001, OperationTime: operationTime,
		AmountPaid: "88.00", PeriodStart: "2026-11-01", PeriodEnd: "2026-12-01", CycleDesc: "每 30 天",
		BenefitDate: "2026-10-28", ExtensionDays: 7, PreviousExtensionDays: 7, PreviousDueDate: "2026-11-01", NextDueDate: "2026-11-08",
		PreviousPrice: "88.00", DiscountAmount: "10.00", NewPrice: "78.00", EffectiveDate: "2026-11-01"}
	switch kind {
	case "customer_renewal_success":
		data.NextDueDate = data.PeriodEnd
	case "customer_benefit_extension":
		data.BenefitType = "extension"
	case "customer_benefit_discount":
		data.BenefitType = "price_discount"
	case "customer_benefit_revision":
		data.BenefitType = "extension_revision"
		data.ExtensionDays = 3
		data.PreviousDueDate = "2026-11-08"
		data.NextDueDate = "2026-11-04"
	case "customer_benefit_revoke":
		data.BenefitType = "extension_revoke"
		data.ExtensionDays = 0
		data.PreviousDueDate = "2026-11-08"
		data.NextDueDate = "2026-11-01"
	}
	return data
}
func (service *SubscriptionService) sendTestBusinessEmail(ctx context.Context, recipient, kind string) error {
	var body string
	var err error
	if kind == "customer_renewal_success" {
		body, err = service.GetRenewalSuccessCustomerEmailTemplate()
	} else {
		body, err = service.GetCustomerBenefitEmailTemplate()
	}
	if err != nil {
		return err
	}
	data := sampleBusinessEmailData(kind, recipient, cycle.FormatDateTime(service.now()))
	rendered, err := renderBusinessEmailTemplate(body, data)
	if err != nil {
		return err
	}
	configuration, registry := service.runtimeConfigSnapshot()
	if !configuration.SMTPConfigured() {
		return fmt.Errorf("SMTP 未配置（需 host/port/from/username/password）")
	}
	sender, ok := registry.Get(model.ChannelSMTP)
	if !ok {
		return fmt.Errorf("SMTP 发送器未就绪，请保存通知配置后重试")
	}
	return service.deliverBenefitEmail(ctx, sender, benefitEmail{recipient: recipient, title: "[测试] " + BusinessEmailSubject(kind), body: "【虚拟示例，仅用于邮件送达测试】\n\n" + rendered})
}

func (service *SubscriptionService) renewalSuccessEmailBatch(subscription model.Subscription, dueDate string) (db.BusinessEmailBatch, error) {
	// Resale bills record our agency fee, not the customer's payment amount.
	if isPlusSubscription(subscription) || subscription.IsResale || strings.TrimSpace(subscription.CustomerEmail) == "" {
		return db.BusinessEmailBatch{}, nil
	}
	_, registry := service.runtimeConfigSnapshot()
	if _, ok := registry.Get(model.ChannelSMTP); !ok {
		return db.BusinessEmailBatch{}, nil
	}
	body, err := service.GetRenewalSuccessCustomerEmailTemplate()
	if err != nil {
		return db.BusinessEmailBatch{}, err
	}
	recipient, err := normalizeCustomerEmail(subscription.CustomerEmail)
	if err != nil {
		return db.BusinessEmailBatch{}, err
	}
	schedule, err := subscription.BillingSchedule()
	if err != nil {
		return db.BusinessEmailBatch{}, err
	}
	dueAt, err := time.ParseInLocation("2006-01-02", dueDate, cycle.Location)
	if err != nil {
		return db.BusinessEmailBatch{}, err
	}
	end := schedule.NextDue(cycle.StartOfDay(dueAt).AddDate(0, 0, 1).Add(-time.Nanosecond))
	data := BusinessEmailTemplateData{CustomerEmail: recipient, SubscriptionID: subscription.ID,
		OperationTime: cycle.FormatDateTime(service.now()), AmountPaid: cycle.FormatCents(billAmountCentsForDueDate(subscription, dueDate)),
		PeriodStart: dueDate, PeriodEnd: cycle.FormatDate(end), NextDueDate: cycle.FormatDate(end), CycleDesc: cycle.DescribeCron(subscription.CronExpr)}
	rendered, err := renderBusinessEmailTemplate(body, data)
	if err != nil {
		return db.BusinessEmailBatch{}, err
	}
	return db.BusinessEmailBatch{Now: service.now(), Build: func() []db.BusinessEmail {
		return []db.BusinessEmail{{SubscriptionID: subscription.ID, Recipient: recipient, Title: BusinessEmailSubject("customer_renewal_success"), Body: rendered}}
	}}, nil
}
