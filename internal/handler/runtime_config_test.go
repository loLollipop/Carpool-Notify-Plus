package handler

import (
	"fmt"
	"sync"
	"testing"

	"carpool-notify/internal/config"
	"carpool-notify/internal/service"
)

func TestServerConfigSnapshotIsSafeDuringRuntimeUpdates(t *testing.T) {
	server := &Server{Service: &service.SubscriptionService{}}
	server.applyConfig(config.Config{ConfigPath: "config-0.toml"})

	const iterations = 2000
	var waitGroup sync.WaitGroup
	waitGroup.Add(2)
	go func() {
		defer waitGroup.Done()
		for index := 1; index <= iterations; index++ {
			server.applyConfig(config.Config{ConfigPath: fmt.Sprintf("config-%d.toml", index)})
		}
	}()
	go func() {
		defer waitGroup.Done()
		for index := 0; index < iterations; index++ {
			if current := server.currentConfig(); current.ConfigPath == "" {
				t.Error("observed empty runtime config")
				return
			}
		}
	}()
	waitGroup.Wait()
}
