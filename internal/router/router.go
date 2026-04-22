package router

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"web2api/internal/handler"
	"web2api/internal/middleware"
)

type Dependencies struct {
	Auth    *middleware.AuthMiddleware
	Web     *handler.WebHandler
	AuthUI  *handler.AuthHandler
	System  *handler.SystemHandler
	Account *handler.AccountHandler
	CPA     *handler.CPAHandler
	Image   *handler.ImageHandler
}

func New(dep Dependencies) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(corsMiddleware())

	r.GET("/", dep.Web.Serve)
	r.GET("/version", dep.System.Version)
	r.GET("/auth/status", dep.AuthUI.Status)
	r.POST("/auth/setup", dep.AuthUI.Setup)
	r.POST("/auth/login", dep.AuthUI.Login)

	apiRoutes := r.Group("/api")
	apiRoutes.Use(dep.Auth.RequireSession())
	apiRoutes.POST("/auth/logout", dep.AuthUI.Logout)
	apiRoutes.GET("/accounts", dep.Account.List)
	apiRoutes.POST("/accounts", dep.Account.Create)
	apiRoutes.POST("/accounts/update", dep.Account.Update)
	apiRoutes.DELETE("/accounts", dep.Account.Delete)
	apiRoutes.POST("/accounts/refresh", dep.Account.Refresh)
	apiRoutes.GET("/cpa/pools", dep.CPA.ListPools)
	apiRoutes.POST("/cpa/pools", dep.CPA.CreatePool)
	apiRoutes.POST("/cpa/pools/:poolID", dep.CPA.UpdatePool)
	apiRoutes.DELETE("/cpa/pools/:poolID", dep.CPA.DeletePool)
	apiRoutes.GET("/cpa/pools/:poolID/files", dep.CPA.ListFiles)
	apiRoutes.POST("/cpa/pools/:poolID/import", dep.CPA.StartImport)
	apiRoutes.GET("/cpa/pools/:poolID/import", dep.CPA.GetImport)
	apiRoutes.POST("/studio/images/generations", dep.Image.CreateImage)
	apiRoutes.POST("/studio/images/edits", dep.Image.EditImage)

	v1Routes := r.Group("/v1")
	v1Routes.Use(dep.Auth.RequireAPIOrSession())
	v1Routes.GET("/models", dep.System.ListModels)
	v1Routes.POST("/images/generations", dep.Image.CreateImage)
	v1Routes.POST("/images/edits", dep.Image.EditImage)
	v1Routes.POST("/chat/completions", dep.Image.CreateChatCompletion)
	v1Routes.POST("/responses", dep.Image.CreateResponse)

	r.NoRoute(func(c *gin.Context) {
		if c.Request.Method == http.MethodGet &&
			!strings.HasPrefix(c.Request.URL.Path, "/api/") &&
			!strings.HasPrefix(c.Request.URL.Path, "/v1/") &&
			!strings.HasPrefix(c.Request.URL.Path, "/auth/") &&
			c.Request.URL.Path != "/version" {
			dep.Web.Serve(c)
			return
		}
		c.JSON(http.StatusNotFound, gin.H{
			"detail": gin.H{"error": "Not Found"},
		})
	})

	return r
}

func corsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}
