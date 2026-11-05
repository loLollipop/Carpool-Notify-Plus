package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

func (server *Server) putAccountSerial(context *gin.Context) {
	id, ok := parseIDParam(context, "id", "无效的账号 ID")
	if !ok {
		return
	}
	context.Request.Body = http.MaxBytesReader(context.Writer, context.Request.Body, 4096)
	var request struct {
		Serial         int64 `json:"serial"`
		ExpectedSerial int64 `json:"expected_serial"`
	}
	if err := context.ShouldBindJSON(&request); err != nil {
		respondError(context, http.StatusBadRequest, "无效的序号请求")
		return
	}
	if err := server.Service.SetAccountDisplaySerial(id, request.Serial, request.ExpectedSerial); err != nil {
		respondError(context, http.StatusBadRequest, err.Error())
		return
	}
	respondOK(context, gin.H{"message": "已调整账号序号及后续编号"})
}
