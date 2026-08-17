package handler

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"carpool-notify/internal/config"
	"carpool-notify/internal/service"

	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
)

const sessionAuthKey = "authenticated"

// Server holds HTTP handlers and dependencies for the JSON API + SPA host.
type Server struct {
	Service        *service.SubscriptionService
	SandboxService *service.SubscriptionService
	SandboxMode    bool
	Config         config.Config
	PasswordHash   []byte
	// DistDir is the built SPA directory (web/dist); non-API routes fall back to its index.html.
	DistDir string

	configMu       sync.RWMutex
	settingsPageMu sync.Mutex
	loginMu        sync.Mutex
	loginFailures  map[string]loginFailureState
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
	api.GET("/redeem-settings", server.getRedeemSettings)
	api.POST("/redeem", server.postRedeemApplication)
	api.GET("/redeem/:token", server.getRedeemStatus)

	var sandboxServer *Server
	if server.SandboxService != nil {
		configuration := server.currentConfig()
		sandboxServer = &Server{
			Service:     server.SandboxService,
			SandboxMode: true,
			Config:      configuration,
		}
		sandboxPublic := api.Group("/sandbox")
		sandboxPublic.Use(server.requireSandboxAccess())
		sandboxPublic.GET("/redeem-settings", sandboxServer.getRedeemSettings)
		sandboxPublic.POST("/redeem", sandboxServer.postRedeemApplication)
		sandboxPublic.GET("/redeem/:token", sandboxServer.getRedeemStatus)
	}

	authorized := api.Group("")
	authorized.Use(server.requireAPIAuth())
	{
		authorized.POST("/logout", server.postLogout)

		server.registerBusinessRoutes(authorized)
		if sandboxServer != nil {
			sandbox := authorized.Group("/sandbox")
			sandbox.GET("/status", server.getSandboxStatus)
			sandbox.POST("/reset", server.postResetSandbox)
			sandbox.POST("/settings/test-notify", sandboxServer.postSettingsTestNotify)
			sandboxServer.registerBusinessRoutes(sandbox)
		}

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

func (server *Server) registerBusinessRoutes(routes *gin.RouterGroup) {
	routes.GET("/calendar", server.getCalendar)
	routes.GET("/dashboard", server.getDashboard)
	routes.GET("/goals", server.getGoals)
	routes.POST("/goals", server.postCreateGoal)
	routes.POST("/goals/market/refresh", server.postRefreshGoalMarket)
	routes.POST("/goals/pricing/bulk-next-price", server.postGoalBulkNextPrice)
	routes.PUT("/goals/:id", server.putUpdateGoal)
	routes.POST("/goals/:id/complete", server.postCompleteGoal)

	routes.GET("/redemptions", server.getRedemptions)
	routes.POST("/redemptions/:id/invite", server.postInviteRedemption)
	routes.POST("/redemptions/:id/reject", server.postRejectRedemption)
	routes.GET("/redemption-codes", server.getRedemptionCodes)
	routes.POST("/redemption-codes", server.postGenerateRedemptionCodes)
	routes.POST("/redemption-codes/:id/disable", server.postDisableRedemptionCode)
	routes.POST("/redemption-codes/:id/enable", server.postEnableRedemptionCode)
	routes.DELETE("/redemption-codes/:id", server.deleteRedemptionCode)

	routes.GET("/subscriptions", server.getSubscriptions)
	routes.POST("/subscriptions", server.postCreateSubscription)
	routes.PUT("/subscriptions/:id", server.putUpdateSubscription)
	routes.DELETE("/subscriptions/:id", server.deleteSubscription)
	routes.POST("/subscriptions/:id/archive", server.postArchiveSubscription)
	routes.POST("/subscriptions/:id/complete-one-month", server.postCompleteOneMonthRental)
	routes.POST("/subscriptions/:id/copy", server.postCopySubscription)
	routes.POST("/subscriptions/:id/test-notify", server.postTestNotify)
	routes.POST("/subscriptions/:id/send-customer-email", server.postSendCustomerEmail)
	routes.GET("/subscriptions/:id/reminder-preview", server.getReminderPreview)
	routes.GET("/subscriptions/:id/due-periods", server.getDuePeriods)
	routes.POST("/subscriptions/:id/due/:date/paid", server.postDuePaid)

	routes.GET("/accounts", server.getAccounts)
	routes.GET("/account-options", server.getAccountOptions)
	routes.POST("/accounts", server.postCreateAccount)
	routes.PUT("/accounts/:id", server.putUpdateAccount)
	routes.POST("/accounts/:id/ban", server.postBanAccount)
	routes.DELETE("/accounts/:id", server.deleteAccount)

	routes.GET("/after-sales", server.getAfterSales)
	routes.PUT("/after-sales/:id", server.putAfterSalesCase)
	routes.POST("/after-sales/:id/refunded", server.postAfterSalesRefunded)
	routes.POST("/after-sales/:id/reassign", server.postAfterSalesReassign)

	routes.GET("/bills", server.getBills)
	routes.PUT("/bills/:id", server.putUpdateBill)
	routes.DELETE("/bills/:id", server.deleteBill)
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
	clientIP := context.ClientIP()
	now := time.Now().UTC()
	if retryAfter := server.loginRetryAfter(clientIP, now); retryAfter > 0 {
		respondLoginRateLimited(context, retryAfter)
		return
	}
	context.Request.Body = http.MaxBytesReader(context.Writer, context.Request.Body, 4096)
	var body struct {
		Password string `json:"password"`
	}
	if err := context.ShouldBindJSON(&body); err != nil {
		respondError(context, http.StatusBadRequest, "无效的请求")
		return
	}
	if err := bcrypt.CompareHashAndPassword(server.PasswordHash, []byte(body.Password)); err != nil {
		server.recordLoginFailure(clientIP, now)
		respondError(context, http.StatusUnauthorized, "密码错误")
		return
	}
	server.clearLoginFailures(clientIP)
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
	if err := session.Save(); err != nil {
		respondError(context, http.StatusInternalServerError, "会话清除失败")
		return
	}
	respondOK(context, nil)
}

func (server *Server) currentConfig() config.Config {
	server.configMu.RLock()
	defer server.configMu.RUnlock()
	return server.Config
}

func (server *Server) applyConfig(configuration config.Config) {
	server.configMu.Lock()
	server.Config = configuration
	server.configMu.Unlock()
	server.Service.ApplyConfig(configuration)
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
	context.Header("Cache-Control", "no-store")
	payload["ok"] = true
	context.JSON(http.StatusOK, payload)
}

func respondError(context *gin.Context, status int, message string) {
	context.Header("Cache-Control", "no-store")
	context.JSON(status, gin.H{"ok": false, "error": message})
}
