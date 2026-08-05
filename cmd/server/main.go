package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"carpool-notify/internal/config"
	"carpool-notify/internal/db"
	"carpool-notify/internal/handler"
	"carpool-notify/internal/notify"
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

	notifyRegistry := notify.Registry{}
	if configuration.GotifyConfigured() {
		notifyRegistry.Gotify = notify.GotifySender{
			BaseURL: configuration.GotifyURL,
			Token:   configuration.GotifyToken,
		}
	}
	if configuration.IYUUConfigured() {
		notifyRegistry.IYUU = notify.IYUUSender{Token: configuration.IYUUToken}
	}
	if configuration.SMTPConfigured() {
		notifyRegistry.SMTP = notify.SMTPSender{
			Host:     configuration.SMTPHost,
			Port:     configuration.SMTPPort,
			Username: configuration.SMTPUsername,
			Password: configuration.SMTPPassword,
			From:     configuration.SMTPFrom,
			To:       notify.ParseSMTPRecipients(configuration.SMTPTo),
		}
	}

	subscriptionService := &service.SubscriptionService{
		Store:  store,
		Config: configuration,
		Notify: notifyRegistry,
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

	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.Use(gin.Recovery(), gin.Logger())

	sessionStore := cookie.NewStore([]byte(configuration.SessionSecret))
	sessionStore.Options(sessions.Options{
		Path:     "/",
		MaxAge:   7 * 24 * 3600,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
	router.Use(sessions.Sessions("carpool_session", sessionStore))
	httpServer.RegisterRoutes(router)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	schedulerRunner := &scheduler.Runner{Service: subscriptionService, Interval: time.Minute}
	go schedulerRunner.Start(ctx)

	server := &http.Server{
		Addr:              configuration.ListenAddress,
		Handler:           router,
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		log.Printf("listening on %s", configuration.ListenAddress)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen: %v", err)
		}
	}()

	<-ctx.Done()
	log.Printf("shutting down")
	shutdownContext, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = server.Shutdown(shutdownContext)
}
