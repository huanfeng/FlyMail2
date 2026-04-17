package server

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"flymail/modules/realtime"
	"flymail/pkg/logger"
	"flymail/shared/config"
	"flymail/shared/database"
)

// Server represents the HTTP server
type Server struct {
	config   *config.Config
	router   *gin.Engine
	db       *database.ServerDB
	services *Services
	sseHub   realtime.Hub
}

// New creates a new server using the builder pattern
func New(config *config.Config) (*Server, error) {
	builder := NewBuilder(config)

	return builder.
		WithDatabase().
		WithRouter().
		WithSSEHub().
		WithServices().
		WithRoutes().
		Build()
}

// Start starts the HTTP server
func (s *Server) Start() error {
	srv := &http.Server{
		Addr:    fmt.Sprintf(":%d", s.config.Server.Port),
		Handler: s.router,
	}

	// 启动服务器
	go func() {
		logger.Info("Starting server",
			zap.Int("port", s.config.Server.Port),
			zap.String("env", s.config.App.Env),
		)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatal("Failed to start server", zap.Error(err))
		}
	}()

	// 等待中断信号
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	logger.Info("Shutting down server...")

	// 优雅关闭
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		logger.Error("Server forced to shutdown", zap.Error(err))
		return err
	}

	// 停止 SSE Hub
	if s.sseHub != nil {
		if err := s.sseHub.Stop(); err != nil {
			logger.Error("Failed to stop SSE hub", zap.Error(err))
		}
	}

	// 关闭数据库连接
	if s.db != nil {
		if err := s.db.Close(); err != nil {
			logger.Error("Failed to close database", zap.Error(err))
		}
	}

	logger.Info("Server exited")
	return nil
}

// GetRouter returns the gin router
func (s *Server) GetRouter() *gin.Engine {
	return s.router
}

// GetDB returns the database instance
func (s *Server) GetDB() *database.ServerDB {
	return s.db
}
