package http

import (
	"log/slog"
	"net/http"

	"github.com/WenshuaiDev/InfraPro/services/api/internal/modules/health"
	"github.com/WenshuaiDev/InfraPro/services/api/internal/transport/http/middleware"
	"github.com/gin-gonic/gin"
)

func NewRouter(logger *slog.Logger) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	_ = router.SetTrustedProxies(nil)
	router.HandleMethodNotAllowed = true
	router.Use(middleware.RequestLog(logger), gin.CustomRecovery(func(c *gin.Context, recovered any) {
		logger.Error("request panic", "error", recovered)
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "internal_error"})
	}))
	router.GET("/healthz", health.Get)
	router.GET("/api/v1/health", health.Get)
	router.NoRoute(func(c *gin.Context) {
		c.JSON(http.StatusNotFound, gin.H{"error": "not_found"})
	})
	router.NoMethod(func(c *gin.Context) {
		c.JSON(http.StatusMethodNotAllowed, gin.H{"error": "method_not_allowed"})
	})
	return router
}
