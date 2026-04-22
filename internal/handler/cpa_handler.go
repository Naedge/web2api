package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"web2api/internal/dto"
	"web2api/internal/service"
)

type CPAHandler struct {
	cpaService *service.CPAService
}

func NewCPAHandler(cpaService *service.CPAService) *CPAHandler {
	return &CPAHandler{cpaService: cpaService}
}

func (h *CPAHandler) ListPools(c *gin.Context) {
	items, err := h.cpaService.ListPools(c.Request.Context())
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"pools": items})
}

func (h *CPAHandler) CreatePool(c *gin.Context) {
	var req dto.CPAPoolCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, &service.StatusError{Code: 400, Message: err.Error()})
		return
	}

	item, err := h.cpaService.CreatePool(c.Request.Context(), req)
	if err != nil {
		writeError(c, err)
		return
	}
	pools, err := h.cpaService.ListPools(c.Request.Context())
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"pool": item, "pools": pools})
}

func (h *CPAHandler) UpdatePool(c *gin.Context) {
	var req dto.CPAPoolUpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, &service.StatusError{Code: 400, Message: err.Error()})
		return
	}

	item, err := h.cpaService.UpdatePool(c.Request.Context(), c.Param("poolID"), req)
	if err != nil {
		writeError(c, err)
		return
	}
	pools, err := h.cpaService.ListPools(c.Request.Context())
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"pool": item, "pools": pools})
}

func (h *CPAHandler) DeletePool(c *gin.Context) {
	if err := h.cpaService.DeletePool(c.Request.Context(), c.Param("poolID")); err != nil {
		writeError(c, err)
		return
	}
	pools, err := h.cpaService.ListPools(c.Request.Context())
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"pools": pools})
}

func (h *CPAHandler) ListFiles(c *gin.Context) {
	files, err := h.cpaService.ListRemoteFiles(c.Request.Context(), c.Param("poolID"))
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"pool_id": c.Param("poolID"), "files": files})
}

func (h *CPAHandler) StartImport(c *gin.Context) {
	var req dto.CPAImportRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, &service.StatusError{Code: 400, Message: err.Error()})
		return
	}

	job, err := h.cpaService.StartImport(c.Request.Context(), c.Param("poolID"), req.Names)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"import_job": job})
}

func (h *CPAHandler) GetImport(c *gin.Context) {
	job, err := h.cpaService.GetImportJob(c.Request.Context(), c.Param("poolID"))
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"import_job": job})
}
