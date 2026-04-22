package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"web2api/internal/dto"
	"web2api/internal/service"
)

type ProxyHandler struct {
	proxyService *service.ProxyService
}

func NewProxyHandler(proxyService *service.ProxyService) *ProxyHandler {
	return &ProxyHandler{proxyService: proxyService}
}

func (h *ProxyHandler) Get(c *gin.Context) {
	item, err := h.proxyService.Get(c.Request.Context())
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, item)
}

func (h *ProxyHandler) Save(c *gin.Context) {
	var req dto.ProxySettingsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, &service.StatusError{Code: 400, Message: err.Error()})
		return
	}

	item, err := h.proxyService.Save(c.Request.Context(), req)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, item)
}
