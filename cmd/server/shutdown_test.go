package main

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"
)

func TestShutdownWaitsForScheduler(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	done := make(chan struct{})
	result := make(chan error, 1)
	go func() { result <- shutdownServer(ctx, &http.Server{}, done) }()
	select {
	case err := <-result:
		t.Fatalf("shutdown returned before the scheduler stopped: %v", err)
	case <-time.After(20 * time.Millisecond):
	}
	close(done)
	if err := <-result; err != nil {
		t.Fatal(err)
	}
}

func TestShutdownHonorsDeadlineForStuckScheduler(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if err := shutdownServer(ctx, &http.Server{}, make(chan struct{})); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("shutdown = %v, want deadline exceeded", err)
	}
}
