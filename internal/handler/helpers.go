package handler

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"web2api/internal/service"
)

func writeError(c *gin.Context, err error) {
	var statusErr *service.StatusError
	if errors.As(err, &statusErr) {
		c.AbortWithStatusJSON(statusErr.Code, gin.H{
			"detail": gin.H{"error": statusErr.Message},
		})
		return
	}

	c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
		"detail": gin.H{"error": err.Error()},
	})
}
