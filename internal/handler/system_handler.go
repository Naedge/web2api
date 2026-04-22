package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"web2api/internal/service"
)

type SystemHandler struct {
	chatService *service.ChatService
}

func NewSystemHandler(chatService *service.ChatService) *SystemHandler {
	return &SystemHandler{
		chatService: chatService,
	}
}

func (h *SystemHandler) Index(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"name":    "web2api",
		"version": service.AppVersion(),
	})
}

func (h *SystemHandler) Version(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"version": service.AppVersion(),
	})
}

func (h *SystemHandler) ListModels(c *gin.Context) {
	c.JSON(http.StatusOK, h.chatService.ListModels())
}
