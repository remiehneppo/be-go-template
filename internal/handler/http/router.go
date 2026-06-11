package http

import (
	"time"

	"github.com/gin-gonic/gin"
	"github.com/remihneppo/be-go-template/internal/config"
	"github.com/remihneppo/be-go-template/internal/middleware"
	"github.com/remihneppo/be-go-template/internal/platform/logger"
)

func NewRouter(cfg config.Config, log logger.Logger) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()

	router.Use(middleware.Recovery(log))
	router.Use(middleware.RequestID(log))
	router.Use(middleware.CORS(cfg.HTTP.CORSAllowOrigins))
	router.Use(middleware.BodyLimit(cfg.HTTP.BodyLimitBytes))
	router.Use(middleware.Timeout(cfg.HTTP.RouteTimeout))
	router.Use(middleware.Logging(log))
	router.Use(middleware.ErrorHandler(log))

	router.GET("/healthz", func(c *gin.Context) {
		OK(c, gin.H{"status": "ok", "time": time.Now().UTC()})
	})

	return router
}
