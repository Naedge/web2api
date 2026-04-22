package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"web2api/internal/dto"
	"web2api/internal/service"
)

type AccountHandler struct {
	accountService *service.AccountService
}

func NewAccountHandler(accountService *service.AccountService) *AccountHandler {
	return &AccountHandler{accountService: accountService}
}

func (h *AccountHandler) List(c *gin.Context) {
	items, err := h.accountService.List(c.Request.Context())
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"items":    items,
		"accounts": items,
	})
}

func (h *AccountHandler) Create(c *gin.Context) {
	var req dto.AccountCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, &service.StatusError{Code: 400, Message: err.Error()})
		return
	}

	_, added, skipped, err := h.accountService.AddTokens(c.Request.Context(), req.Tokens)
	if err != nil {
		writeError(c, err)
		return
	}

	refreshResult, err := h.accountService.RefreshAccountsDetailed(c.Request.Context(), req.Tokens)
	if err != nil {
		writeError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"added":     added,
		"skipped":   skipped,
		"refreshed": refreshResult.Refreshed,
		"errors":    refreshResult.Errors,
		"items":     refreshResult.Items,
		"accounts":  refreshResult.Items,
	})
}

func (h *AccountHandler) Update(c *gin.Context) {
	var req dto.AccountUpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, &service.StatusError{Code: 400, Message: err.Error()})
		return
	}

	item, err := h.accountService.Update(c.Request.Context(), req)
	if err != nil {
		writeError(c, err)
		return
	}

	items, err := h.accountService.List(c.Request.Context())
	if err != nil {
		writeError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"item":    item,
		"account": item,
		"items":   items,
	})
}

func (h *AccountHandler) Delete(c *gin.Context) {
	var req dto.AccountDeleteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, &service.StatusError{Code: 400, Message: err.Error()})
		return
	}

	rows, items, err := h.accountService.DeleteTokens(c.Request.Context(), req.Tokens)
	if err != nil {
		writeError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"removed":  rows,
		"deleted":  rows,
		"items":    items,
		"accounts": items,
	})
}

func (h *AccountHandler) Refresh(c *gin.Context) {
	var req dto.AccountRefreshRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, &service.StatusError{Code: 400, Message: err.Error()})
		return
	}

	result, err := h.accountService.RefreshAccountsDetailed(c.Request.Context(), req.AccessTokens)
	if err != nil {
		writeError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"refreshed": result.Refreshed,
		"errors":    result.Errors,
		"items":     result.Items,
		"accounts":  result.Items,
	})
}
