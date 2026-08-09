package handler

import (
	"net/http"

	"carpool-notify/internal/service"

	"github.com/gin-gonic/gin"
)

type redemptionRejectRequest struct {
	Reason string `json:"reason"`
}

func (server *Server) postRejectRedemption(context *gin.Context) {
	applicationID, ok := parseIDParam(context, "id", "无效的兑换申请 ID")
	if !ok {
		return
	}
	var request redemptionRejectRequest
	if err := context.ShouldBindJSON(&request); err != nil {
		respondError(context, http.StatusBadRequest, "无效的驳回内容")
		return
	}
	if err := server.Service.RejectRedemptionApplication(applicationID, service.RedemptionRejectInput{
		Reason: request.Reason,
	}); err != nil {
		respondError(context, http.StatusBadRequest, err.Error())
		return
	}
	respondOK(context, gin.H{"message": "兑换申请已驳回，兑换码已恢复可用"})
}
