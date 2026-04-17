package server

import (
	"time"

	"github.com/gin-gonic/gin"

	"flymail/modules/auth"
	"flymail/modules/email/account"
	// "flymail/modules/email/folder"
	"flymail/modules/email/message"
	"flymail/modules/realtime"
	"flymail/modules/system/monitor"
	"flymail/modules/system/setting"
	"flymail/modules/system/task"
	"flymail-core/logger"
	"flymail/shared/config"
	"flymail/shared/middleware"
)

func setupRoutes(router *gin.Engine, services *Services, sseHub realtime.Hub, config *config.Config) {
	// API Documentation - Serve Swagger UI with custom configuration
	router.GET("/api/swagger/*path", serveSwaggerFiles)

	// API v1 routes
	v1 := router.Group("/api/v1")
	{
		// Serve OpenAPI spec file with dynamic server URL
		v1.GET("/openapi.yaml", serveOpenAPISpec(config))

		// Health check
		v1.GET("/health", func(c *gin.Context) {
			c.JSON(200, gin.H{
				"status":    "ok",
				"timestamp": time.Now().Format(time.DateTime),
			})
		})

		// Monitoring endpoints (public)
		monitorGroup := v1.Group("/monitor")
		{
			monitorHandler := monitor.NewHandler(services.Monitor)
			monitorGroup.GET("/status", monitorHandler.GetStatus)
			monitorGroup.GET("/health", monitorHandler.GetHealth)
			monitorGroup.GET("/system", monitorHandler.GetSystemStatus)
			monitorGroup.GET("/realtime", monitorHandler.GetRealtimeStatus)
		}

		// Public routes
		authGroup := v1.Group("/auth")
		{
			authHandler := auth.NewHandler(services.Auth)
			authGroup.POST("/login", authHandler.Login)
			authGroup.POST("/refresh", authHandler.Refresh)
		}

		// Protected routes
		protected := v1.Group("/")
		protected.Use(middleware.AuthMiddleware())
		{
			// User routes
			authHandler := auth.NewHandler(services.Auth)
			protected.GET("/auth/me", authHandler.Me)

			// Admin only auth routes
			adminAuth := protected.Group("/auth")
			adminAuth.Use(middleware.AdminMiddleware())
			{
				adminAuth.PUT("/admin/credentials", authHandler.UpdateAdminCredentials)
			}

			// Account routes
			accounts := protected.Group("/accounts")
			{
				// 暂时只使用基本的 account handler
				accountHandler := account.NewHandler(services.Account, nil)
				accounts.GET("", accountHandler.List)     // Support /accounts
				accounts.GET("/", accountHandler.List)    // Support /accounts/
				accounts.POST("", accountHandler.Create)  // Support /accounts
				accounts.POST("/", accountHandler.Create) // Support /accounts/
				accounts.GET("/:id", accountHandler.Get)
				accounts.PUT("/:id", accountHandler.Update)
				accounts.DELETE("/:id", accountHandler.Delete)
				accounts.GET("/:id/stats", accountHandler.GetStats)
				accounts.GET("/:id/sync-status", accountHandler.GetSyncStatus)
				accounts.PUT("/order", accountHandler.UpdateAccountsOrder)
				accounts.POST("/temp_test", accountHandler.TestAccount)

				// Folder operations - 暂时禁用，因为需要 IMAP client factory
				// folderHandler := folder.NewHandler(services.Folder, nil, nil)
				// accounts.GET("/:id/folders", folderHandler.List)
				// accounts.POST("/:id/folders/sync", folderHandler.Sync)
				// accounts.PUT("/:id/folders/order", folderHandler.UpdateFoldersOrder)
			}

			// Emails routes
			emails := protected.Group("/emails")
			{
				messageHandler := message.NewHandler(services.Message)
				emails.GET("", messageHandler.List)  // Support /emails
				emails.GET("/", messageHandler.List) // Support /emails/
				emails.GET("/:id", messageHandler.Get)
				emails.GET("/:id/flags", messageHandler.GetFlags)
				emails.PATCH("/:id/flags", messageHandler.UpdateFlags)
				emails.DELETE("/:id", messageHandler.Delete)

				// Batch operations
				emails.POST("/batch/flags", messageHandler.BatchUpdateFlags)
				emails.DELETE("/batch", messageHandler.BatchDelete)
			}

			// Task routes
			tasks := protected.Group("/tasks")
			{
				taskHandler := task.NewHTTPHandler(services.Task)
				tasks.GET("", taskHandler.ListTasks)    // Support /tasks
				tasks.GET("/", taskHandler.ListTasks)   // Support /tasks/
				tasks.POST("", taskHandler.CreateTask)  // Support /tasks
				tasks.POST("/", taskHandler.CreateTask) // Support /tasks/
				tasks.GET("/:id", taskHandler.GetTask)
				tasks.PUT("/:id", taskHandler.UpdateTask)
				tasks.DELETE("/:id", taskHandler.DeleteTask)
				tasks.POST("/:id/execute", taskHandler.ExecuteTask)
				tasks.GET("/:id/logs", taskHandler.GetTaskLogs)
			}

			// Setting routes (admin only)
			settings := protected.Group("/settings")
			settings.Use(middleware.AdminMiddleware())
			{
				settingHandler := setting.NewHandler(services.Setting)
				settings.GET("", settingHandler.GetAllSettings)  // Support /settings
				settings.GET("/", settingHandler.GetAllSettings) // Support /settings/
				settings.GET("/:key", settingHandler.GetSetting)
				settings.PUT("/:key", settingHandler.UpdateSetting)
				settings.DELETE("/:key", settingHandler.DeleteSetting)
				settings.PUT("/", settingHandler.UpdateMultipleSettings)

				// App settings endpoints
				settings.GET("/app", settingHandler.GetAppSettings)
				settings.PUT("/app", settingHandler.UpdateAppSettings)

				// Email monitor settings
				settings.GET("/email-monitor", settingHandler.GetEmailMonitorSettings)
				settings.PUT("/email-monitor", settingHandler.UpdateEmailMonitorSettings)
			}

			// Real-time communication routes (using SSE-specific auth middleware)
			sseHandler := realtime.NewHandler(sseHub, logger.Logger)
			v1.GET("/events", middleware.SSEAuthMiddleware(), sseHandler.HandleSSE)

			// SSE management routes (admin only)
			sseManagement := protected.Group("/sse")
			sseManagement.Use(middleware.AdminMiddleware())
			{
				sseManagement.GET("/stats", sseHandler.GetStats)
				sseManagement.GET("/connections", sseHandler.GetConnections)
				sseManagement.POST("/broadcast", sseHandler.Broadcast)
			}
		}
	}
}
