package service

import (
	"fmt"
	"sync"
	"testing"

	"carpool-notify/internal/config"
	"carpool-notify/internal/notify"
)

func TestApplyConfigPublishesConsistentRuntimeSnapshot(t *testing.T) {
	configuration := func(index int) config.Config {
		return config.Config{
			SMTPHost:     fmt.Sprintf("smtp-%d.example.com", index),
			SMTPPort:     587,
			SMTPUsername: "operator",
			SMTPPassword: "secret",
			SMTPFrom:     "operator@example.com",
			SMTPTo:       "owner@example.com",
			IYUUToken:    fmt.Sprintf("iyuu-%d", index),
			GotifyURL:    fmt.Sprintf("https://gotify-%d.example.com", index),
			GotifyToken:  fmt.Sprintf("gotify-%d", index),
		}
	}

	service := &SubscriptionService{}
	service.ApplyConfig(configuration(0))

	const iterations = 2000
	var waitGroup sync.WaitGroup
	waitGroup.Add(2)
	go func() {
		defer waitGroup.Done()
		for index := 1; index <= iterations; index++ {
			service.ApplyConfig(configuration(index))
		}
	}()
	go func() {
		defer waitGroup.Done()
		for index := 0; index < iterations; index++ {
			current, registry := service.runtimeConfigSnapshot()
			sender, ok := registry.SMTP.(notify.SMTPSender)
			if !ok {
				t.Errorf("SMTP sender type = %T, want notify.SMTPSender", registry.SMTP)
				return
			}
			if sender.Host != current.SMTPHost || sender.Password != current.SMTPPassword {
				t.Errorf("mixed runtime snapshot: config=%q/%q sender=%q/%q", current.SMTPHost, current.SMTPPassword, sender.Host, sender.Password)
				return
			}
		}
	}()
	waitGroup.Wait()
}
