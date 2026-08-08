package handler

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

func (server *Server) requireSandboxAccess() gin.HandlerFunc {
	return func(context *gin.Context) {
		if server.SandboxService == nil {
			context.AbortWithStatusJSON(http.StatusNotFound, gin.H{"ok": false, "error": "演练环境未启用"})
			return
		}
		valid, err := server.SandboxService.ValidateSandboxAccessToken(
			strings.TrimSpace(context.Query("sandbox_token")),
		)
		if err != nil {
			context.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"ok": false, "error": err.Error()})
			return
		}
		if !valid {
			context.AbortWithStatusJSON(http.StatusForbidden, gin.H{"ok": false, "error": "测试链接无效，请在系统工具中重新打开"})
			return
		}
		context.Next()
	}
}

func (server *Server) getSandboxStatus(context *gin.Context) {
	if server.SandboxService == nil {
		respondError(context, http.StatusNotFound, "演练环境未启用")
		return
	}
	status, err := server.SandboxService.GetSandboxStatus()
	if err != nil {
		respondError(context, http.StatusInternalServerError, err.Error())
		return
	}
	respondOK(context, gin.H{"sandbox": status})
}

func (server *Server) postResetSandbox(context *gin.Context) {
	if server.SandboxService == nil {
		respondError(context, http.StatusNotFound, "演练环境未启用")
		return
	}
	status, err := server.SandboxService.ResetSandboxFixtures()
	if err != nil {
		respondError(context, http.StatusInternalServerError, err.Error())
		return
	}
	respondOK(context, gin.H{
		"sandbox": status,
		"message": "演练环境已重置，正式数据未受影响",
	})
}
