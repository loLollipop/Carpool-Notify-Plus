package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"carpool-notify/internal/config"
	"carpool-notify/internal/db"
	"carpool-notify/internal/handler"
	"carpool-notify/internal/scheduler"
	"carpool-notify/internal/service"

	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
)

func main() {
	configuration, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}
	log.Printf("config loaded from %s", configuration.ConfigPath)

	store, err := db.Open(configuration.DatabasePath)
	if err != nil {
		log.Fatalf("database: %v", err)
	}
	defer store.Close()

	sandboxStore, err := db.Open(sandboxDatabasePath(configuration.DatabasePath))
	if err != nil {
		log.Fatalf("sandbox database: %v", err)
	}
	defer sandboxStore.Close()

	subscriptionService := &service.SubscriptionService{
		Store:  store,
		Config: configuration,
		Notify: service.NewNotifyRegistry(configuration),
	}
	sandboxService := &service.SubscriptionService{
		Store:  sandboxStore,
		Config: configuration,
	}
	for label, currentService := range map[string]*service.SubscriptionService{
		"database":         subscriptionService,
		"sandbox database": sandboxService,
	} {
		repaired, repairErr := currentService.NormalizeScheduledNextPriceEffectiveDates()
		if repairErr != nil {
			log.Fatalf("%s scheduled next prices: %v", label, repairErr)
		}
		if repaired > 0 {
			log.Printf("%s: corrected %d postponed next-price effective date(s)", label, repaired)
		}
	}

	workingDirectory, err := os.Getwd()
	if err != nil {
		log.Fatalf("workdir: %v", err)
	}
	distDir := filepath.Join(workingDirectory, "web", "dist")

	httpServer, err := handler.NewServer(subscriptionService, configuration, distDir)
	if err != nil {
		log.Fatalf("handler: %v", err)
	}
	defer httpServer.Close()
	httpServer.SandboxService = sandboxService

	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	if err := configureTrustedProxies(router, configuration.TrustedProxies); err != nil {
		log.Fatalf("trusted proxies: %v", err)
	}
	router.Use(
		handler.SecurityHeaders(),
		handler.RequestLogger(),
		gin.Recovery(),
	)

	sessionStore := cookie.NewStore([]byte(configuration.SessionSecret))
	sessionStore.Options(sessions.Options{
		Path:     "/",
		MaxAge:   7 * 24 * 3600,
		HttpOnly: true,
		Secure:   configuration.SessionCookieSecure,
		SameSite: http.SameSiteLaxMode,
	})
	router.Use(sessions.Sessions("carpool_session", sessionStore))
	httpServer.RegisterRoutes(router)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	schedulerRunner := &scheduler.Runner{Service: subscriptionService, Interval: time.Minute}
	schedulerDone := make(chan struct{})
	go func() {
		defer close(schedulerDone)
		schedulerRunner.Start(ctx)
	}()

	server := &http.Server{
		Addr:              configuration.ListenAddress,
		Handler:           router,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	go func() {
		log.Printf("listening on %s", configuration.ListenAddress)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen: %v", err)
		}
	}()

	<-ctx.Done()
	log.Printf("shutting down")
	shutdownContext, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := shutdownServer(shutdownContext, server, schedulerDone); err != nil {
		// Do not close databases underneath unfinished handlers or background work.
		log.Fatalf("graceful shutdown: %v", err)
	}
}

func shutdownServer(ctx context.Context, server *http.Server, schedulerDone <-chan struct{}) error {
	if err := server.Shutdown(ctx); err != nil {
		return err
	}
	select {
	case <-schedulerDone:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func configureTrustedProxies(router *gin.Engine, trustedProxies []string) error {
	router.RemoteIPHeaders = []string{"X-Forwarded-For"}
	return router.SetTrustedProxies(trustedProxies)
}

func sandboxDatabasePath(databasePath string) string {
	directory := filepath.Dir(databasePath)
	filename := filepath.Base(databasePath)
	extension := filepath.Ext(filename)
	name := strings.TrimSuffix(filename, extension)
	if extension == "" {
		extension = ".db"
	}
	return filepath.Join(directory, name+".sandbox"+extension)
}
