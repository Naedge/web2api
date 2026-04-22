package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"web2api/internal/service"
)

type AuthMiddleware struct {
	authService *service.AuthService
}

func NewAuthMiddleware(
	authService *service.AuthService,
) *AuthMiddleware {
	return &AuthMiddleware{
		authService: authService,
	}
}

func (m *AuthMiddleware) RequireAPIKey() gin.HandlerFunc {
	return func(c *gin.Context) {
		token := extractBearerToken(c.GetHeader("Authorization"))
		valid, err := m.authService.ValidateAPIKey(c.Request.Context(), token)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
				"detail": gin.H{"error": err.Error()},
			})
			return
		}
		if !valid {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"detail": gin.H{"error": "authorization is invalid"},
			})
			return
		}
		c.Next()
	}
}

func (m *AuthMiddleware) RequireAPIOrSession() gin.HandlerFunc {
	return func(c *gin.Context) {
		token := extractBearerToken(c.GetHeader("Authorization"))
		if token != "" {
			valid, err := m.authService.ValidateAPIKey(c.Request.Context(), token)
			if err != nil {
				c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
					"detail": gin.H{"error": err.Error()},
				})
				return
			}
			if valid {
				c.Next()
				return
			}
		}

		cookieValue, err := c.Cookie(service.SessionCookieName)
		if err == nil {
			session, parseErr := m.authService.ParseSession(c.Request.Context(), cookieValue)
			if parseErr == nil {
				c.Set("session", session)
				c.Next()
				return
			}
		}

		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
			"detail": gin.H{"error": "Unauthorized"},
		})
	}
}

func (m *AuthMiddleware) RequireSession() gin.HandlerFunc {
	return func(c *gin.Context) {
		cookieValue, err := c.Cookie(service.SessionCookieName)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"detail": gin.H{"error": "Unauthorized"},
			})
			return
		}

		session, err := m.authService.ParseSession(c.Request.Context(), cookieValue)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"detail": gin.H{"error": "Unauthorized"},
			})
			return
		}

		c.Set("session", session)
		c.Next()
	}
}

func extractBearerToken(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if parts := strings.SplitN(value, " ", 2); len(parts) == 2 && strings.EqualFold(parts[0], "Bearer") {
		return strings.TrimSpace(parts[1])
	}
	return value
}
