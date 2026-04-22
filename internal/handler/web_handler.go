package handler

import (
	"mime"
	"net/http"
	"path/filepath"

	"github.com/gin-gonic/gin"

	"web2api/internal/frontend"
)

type WebHandler struct{}

func NewWebHandler() *WebHandler {
	return &WebHandler{}
}

func (h *WebHandler) Serve(c *gin.Context) {
	body, assetPath, err := frontend.ReadAsset(c.Request.URL.Path)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"detail": gin.H{"error": "Not Found"},
		})
		return
	}

	contentType := mime.TypeByExtension(filepath.Ext(assetPath))
	if contentType == "" {
		contentType = "text/html; charset=utf-8"
	}

	c.Data(http.StatusOK, contentType, body)
}
