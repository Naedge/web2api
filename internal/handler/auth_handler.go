package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"web2api/internal/dto"
	"web2api/internal/service"
)

type AuthHandler struct {
	authService *service.AuthService
}

func NewAuthHandler(authService *service.AuthService) *AuthHandler {
	return &AuthHandler{authService: authService}
}

func (h *AuthHandler) Status(c *gin.Context) {
	initialized, err := h.authService.IsInitialized(c.Request.Context())
	if err != nil {
		writeError(c, err)
		return
	}

	response := gin.H{
		"initialized":   initialized,
		"authenticated": false,
	}

	cookieValue, err := c.Cookie(service.SessionCookieName)
	if err == nil {
		session, parseErr := h.authService.ParseSession(c.Request.Context(), cookieValue)
		if parseErr == nil {
			response["authenticated"] = true
			response["username"] = session.Username
		}
	}

	c.JSON(http.StatusOK, response)
}

func (h *AuthHandler) Setup(c *gin.Context) {
	var req dto.SetupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, &service.StatusError{Code: 400, Message: err.Error()})
		return
	}

	session, err := h.authService.Setup(c.Request.Context(), req.Username, req.Password)
	if err != nil {
		writeError(c, err)
		return
	}

	setSessionCookie(c, session)
	c.JSON(http.StatusOK, gin.H{
		"initialized":   true,
		"authenticated": true,
		"username":      session.Username,
	})
}

func (h *AuthHandler) Login(c *gin.Context) {
	var req dto.LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, &service.StatusError{Code: 400, Message: err.Error()})
		return
	}

	session, err := h.authService.Login(c.Request.Context(), req.Username, req.Password)
	if err != nil {
		writeError(c, err)
		return
	}

	setSessionCookie(c, session)
	c.JSON(http.StatusOK, gin.H{
		"initialized":   true,
		"authenticated": true,
		"username":      session.Username,
	})
}

func (h *AuthHandler) Logout(c *gin.Context) {
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     service.SessionCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   c.Request.TLS != nil,
	})

	c.JSON(http.StatusOK, gin.H{"message": "ok"})
}

func (h *AuthHandler) GetAPIKey(c *gin.Context) {
	info, err := h.authService.GetAPIKey(c.Request.Context())
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, info)
}

func (h *AuthHandler) RegenerateAPIKey(c *gin.Context) {
	info, err := h.authService.RegenerateAPIKey(c.Request.Context())
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, info)
}

func setSessionCookie(c *gin.Context, session *service.IssuedSession) {
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     service.SessionCookieName,
		Value:    session.Value,
		Path:     "/",
		Expires:  session.ExpiresAt,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   c.Request.TLS != nil,
	})
}
