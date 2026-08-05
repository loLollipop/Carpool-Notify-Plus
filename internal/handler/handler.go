package handler

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"carpool-notify/internal/config"
	"carpool-notify/internal/service"

	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
)

const sessionAuthKey = "authenticated"

// Server holds HTTP handlers and dependencies for the JSON API + SPA host.
type Server struct {
	Service      *service.SubscriptionService
	Config       config.Config
	PasswordHash []byte
	// DistDir is the built SPA directory (web/dist); non-API routes fall back to its index.html.
	DistDir string
}

// NewServer constructs a Server and hashes the login password.
func NewServer(subscriptionService *service.SubscriptionService, configuration config.Config, distDir string) (*Server, error) {
	passwordHash, err := bcrypt.GenerateFromPassword([]byte(configuration.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("hash password: %w", err)
	}
	return &Server{
		Service:      subscriptionService,
		Config:       configuration,
		PasswordHash: passwordHash,
		DistDir:      distDir,
	}, nil
}

// RegisterRoutes wires the JSON API, the export download, and the SPA fallback.
func (server *Server) RegisterRoutes(router *gin.Engine) {
	api := router.Group("/api")
	api.POST("/login", server.postLogin)
	api.GET("/session", server.getSession)
	api.POST("/redeem", server.postRedeemApplication)
	api.GET("/redeem/:token", server.getRedeemStatus)

	authorized := api.Group("")
	authorized.Use(server.requireAPIAuth())
	{
		authorized.POST("/logout", server.postLogout)

		authorized.GET("/calendar", server.getCalendar)
		authorized.GET("/dashboard", server.getDashboard)

		authorized.GET("/redemptions", server.getRedemptions)
		authorized.POST("/redemptions/:id/invite", server.postInviteRedemption)
		authorized.GET("/redemption-codes", server.getRedemptionCodes)
		authorized.POST("/redemption-codes", server.postGenerateRedemptionCodes)
		authorized.POST("/redemption-codes/:id/disable", server.postDisableRedemptionCode)
		authorized.POST("/redemption-codes/:id/enable", server.postEnableRedemptionCode)

		authorized.GET("/subscriptions", server.getSubscriptions)
		authorized.POST("/subscriptions", server.postCreateSubscription)
		authorized.PUT("/subscriptions/:id", server.putUpdateSubscription)
		authorized.DELETE("/subscriptions/:id", server.deleteSubscription)
		authorized.POST("/subscriptions/:id/archive", server.postArchiveSubscription)
		authorized.POST("/subscriptions/:id/copy", server.postCopySubscription)
		authorized.POST("/subscriptions/:id/test-notify", server.postTestNotify)
		authorized.POST("/subscriptions/:id/send-customer-email", server.postSendCustomerEmail)
		authorized.GET("/subscriptions/:id/reminder-preview", server.getReminderPreview)
		authorized.GET("/subscriptions/:id/due-periods", server.getDuePeriods)
		authorized.POST("/subscriptions/:id/due/:date/paid", server.postDuePaid)

		authorized.GET("/accounts", server.getAccounts)
		authorized.GET("/account-options", server.getAccountOptions)
		authorized.POST("/accounts", server.postCreateAccount)
		authorized.PUT("/accounts/:id", server.putUpdateAccount)
		authorized.DELETE("/accounts/:id", server.deleteAccount)

		authorized.GET("/bills", server.getBills)
		authorized.PUT("/bills/:id", server.putUpdateBill)
		authorized.DELETE("/bills/:id", server.deleteBill)

		authorized.GET("/settings", server.getSettings)
		authorized.PUT("/settings", server.putSettings)
		authorized.POST("/settings/test-notify", server.postSettingsTestNotify)
		authorized.POST("/settings/template-preview", server.postSettingsTemplatePreview)

		authorized.GET("/cron/preview", server.getCronPreview)
	}

	// Export keeps its non-/api path so the download URL stays stable.
	export := router.Group("")
	export.Use(server.requireAPIAuth())
	export.GET("/export", server.getExport)

	server.registerSPA(router)
}

func (server *Server) requireAPIAuth() gin.HandlerFunc {
	return func(context *gin.Context) {
		session := sessions.Default(context)
		if session.Get(sessionAuthKey) != true {
			context.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"ok": false, "error": "未登录"})
			return
		}
		context.Next()
	}
}

func (server *Server) getSession(context *gin.Context) {
	session := sessions.Default(context)
	context.JSON(http.StatusOK, gin.H{
		"ok":            true,
		"authenticated": session.Get(sessionAuthKey) == true,
	})
}

func (server *Server) postLogin(context *gin.Context) {
	var body struct {
		Password string `json:"password"`
	}
	if err := context.ShouldBindJSON(&body); err != nil {
		respondError(context, http.StatusBadRequest, "无效的请求")
		return
	}
	if err := bcrypt.CompareHashAndPassword(server.PasswordHash, []byte(body.Password)); err != nil {
		respondError(context, http.StatusUnauthorized, "密码错误")
		return
	}
	session := sessions.Default(context)
	session.Set(sessionAuthKey, true)
	if err := session.Save(); err != nil {
		respondError(context, http.StatusInternalServerError, "会话保存失败")
		return
	}
	respondOK(context, nil)
}

func (server *Server) postLogout(context *gin.Context) {
	session := sessions.Default(context)
	session.Clear()
	_ = session.Save()
	respondOK(context, nil)
}

// registerSPA serves files from DistDir and falls back to index.html for app routes.
func (server *Server) registerSPA(router *gin.Engine) {
	router.NoRoute(func(context *gin.Context) {
		requestPath := context.Request.URL.Path
		if strings.HasPrefix(requestPath, "/api/") {
			context.JSON(http.StatusNotFound, gin.H{"ok": false, "error": "not found"})
			return
		}
		if context.Request.Method != http.MethodGet && context.Request.Method != http.MethodHead {
			context.JSON(http.StatusNotFound, gin.H{"ok": false, "error": "not found"})
			return
		}

		relative := strings.TrimPrefix(filepath.Clean("/"+requestPath), "/")
		if relative != "" && relative != "." && relative != "index.html" {
			fullPath := filepath.Join(server.DistDir, relative)
			if info, err := os.Stat(fullPath); err == nil && !info.IsDir() {
				if strings.HasPrefix(requestPath, "/assets/") {
					// Vite asset filenames are content-hashed, safe to cache forever.
					context.Header("Cache-Control", "public, max-age=31536000, immutable")
				}
				context.File(fullPath)
				return
			}
		}

		indexPath := filepath.Join(server.DistDir, "index.html")
		if _, err := os.Stat(indexPath); err != nil {
			context.String(http.StatusServiceUnavailable, "前端未构建：请在 web/app 目录运行 npm run build")
			return
		}
		context.Header("Cache-Control", "no-cache")
		context.File(indexPath)
	})
}

func respondOK(context *gin.Context, payload gin.H) {
	if payload == nil {
		payload = gin.H{}
	}
	payload["ok"] = true
	context.JSON(http.StatusOK, payload)
}

func respondError(context *gin.Context, status int, message string) {
	context.JSON(status, gin.H{"ok": false, "error": message})
}
