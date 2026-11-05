package handler

import (
	"errors"
	"net/http"

	"carpool-notify/internal/db"
	"carpool-notify/internal/service"
	"github.com/gin-gonic/gin"
)

func (server *Server) putAccountAvailability(context *gin.Context) {
	id, ok := parseIDParam(context, "id", "无效的账号 ID")
	if !ok {
		return
	}
	context.Request.Body = http.MaxBytesReader(context.Writer, context.Request.Body, 4096)
	var request struct {
		Paused *bool `json:"paused"`
	}
	if err := context.ShouldBindJSON(&request); err != nil || request.Paused == nil {
		respondError(context, http.StatusBadRequest, "请明确选择暂停或恢复使用")
		return
	}
	if err := server.Service.SetAccountPaused(id, *request.Paused); err != nil {
		respondError(context, http.StatusBadRequest, err.Error())
		return
	}
	respondOK(context, gin.H{"paused": *request.Paused})
}

func (server *Server) postDeferRenewal(context *gin.Context) {
	id, ok := parseIDParam(context, "id", "无效的客户 ID")
	if !ok {
		return
	}
	context.Request.Body = http.MaxBytesReader(context.Writer, context.Request.Body, 4096)
	var request service.DeferRenewalInput
	if err := context.ShouldBindJSON(&request); err != nil {
		respondError(context, http.StatusBadRequest, "无效的搁置请求")
		return
	}
	if err := server.Service.DeferSubscriptionRenewal(id, request); err != nil {
		respondDeferralError(context, err)
		return
	}
	respondOK(context, gin.H{"message": "续费已搁置，原席位保留"})
}

func (server *Server) deleteDeferRenewal(context *gin.Context) {
	id, ok := parseIDParam(context, "id", "无效的客户 ID")
	if !ok {
		return
	}
	if err := server.Service.RestoreSubscriptionRenewal(id); err != nil {
		respondDeferralError(context, err)
		return
	}
	respondOK(context, gin.H{"message": "已恢复续费跟进"})
}

func (server *Server) postCompleteDeferred(context *gin.Context) {
	context.Request.Body = http.MaxBytesReader(context.Writer, context.Request.Body, 256<<10)
	var request struct {
		Subscriptions []service.CompleteDeferredInput `json:"subscriptions"`
	}
	if err := context.ShouldBindJSON(&request); err != nil {
		respondError(context, http.StatusBadRequest, "无效的移出确认")
		return
	}
	count, err := server.Service.CompleteDeferredSubscriptions(request.Subscriptions)
	if err != nil {
		respondDeferralError(context, err)
		return
	}
	respondOK(context, gin.H{"completed_count": count})
}

func respondDeferralError(context *gin.Context, err error) {
	status := http.StatusBadRequest
	if errors.Is(err, db.ErrSubscriptionStateChanged) {
		status = http.StatusConflict
	}
	respondError(context, status, err.Error())
}
