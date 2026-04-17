package middleware

import (
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"flymail/pkg/logger"
	"flymail/shared/config"
)

// SetupCORS 配置 CORS 中间件
func SetupCORS(cfg *config.Config) gin.HandlerFunc {
	corsConfig := cors.DefaultConfig()

	if cfg.App.Env == "development" || cfg.Logger.Development {
		// 开发模式：开放 CORS
		logger.Info("Setting up CORS for development environment")
		// 使用 AllowOriginFunc 来允许所有来源
		corsConfig.AllowOriginFunc = func(origin string) bool {
			return true
		}
		corsConfig.AllowCredentials = true
		corsConfig.AllowMethods = []string{"GET", "POST", "PUT", "DELETE", "OPTIONS", "HEAD", "PATCH"}
		corsConfig.AllowHeaders = []string{"*"}
		corsConfig.ExposeHeaders = []string{"*"}
	} else {
		// 生产模式：严格 CORS 设置
		logger.Info("Setting up CORS for production environment")
		corsConfig.AllowOrigins = cfg.CORS.AllowOrigins
		corsConfig.AllowMethods = cfg.CORS.AllowMethods
		corsConfig.AllowHeaders = cfg.CORS.AllowHeaders
		corsConfig.ExposeHeaders = cfg.CORS.ExposeHeaders
		corsConfig.AllowCredentials = cfg.CORS.AllowCredentials
		corsConfig.MaxAge = time.Duration(cfg.CORS.MaxAge) * time.Second

		// 如果没有配置允许的域名，则使用默认的安全设置
		if len(cfg.CORS.AllowOrigins) == 0 {
			logger.Warn("No CORS origins configured for production")
		}
	}

	logger.Info("CORS configuration applied",
		zap.Bool("development", cfg.Logger.Development),
		zap.String("env", cfg.App.Env),
		zap.Bool("allow_origin_func", corsConfig.AllowOriginFunc != nil),
		zap.Strings("allow_origins", corsConfig.AllowOrigins),
		zap.Strings("allow_methods", corsConfig.AllowMethods),
		zap.Bool("allow_credentials", corsConfig.AllowCredentials))

	return cors.New(corsConfig)
}
