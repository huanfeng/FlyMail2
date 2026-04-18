package app

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/emersion/go-message"
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"golang.org/x/text/encoding/htmlindex"

	appconfig "mail2im/internal/config"
	"mail2im/internal/core"
	"mail2im/internal/dispatcher"
	"mail2im/internal/dispatcher/channels"

	"flymail-core/logger"

	"mail2im/internal/api"
)

// InitFromConfig 从配置文件初始化应用配置，返回 ServerConfig
func InitFromConfig() (ServerConfig, error) {
	cfg, err := appconfig.Load()
	if err != nil {
		return ServerConfig{}, fmt.Errorf("failed to load config: %w", err)
	}
	return ServerConfig{
		Port:     cfg.Port,
		DataRoot: cfg.DataRoot,
	}, nil
}

// ServerConfig holds configuration for the server.
type ServerConfig struct {
	Port     string
	DataRoot string
}

// DefaultServerConfig returns a config with default values from the loaded config.
func DefaultServerConfig() ServerConfig {
	cfg := appconfig.AppConfig
	if cfg != nil {
		return ServerConfig{Port: cfg.Port, DataRoot: cfg.DataRoot}
	}
	return ServerConfig{Port: "8080", DataRoot: "./mail2im_data"}
}

// Server manages the Mail2IM HTTP server and background services.
type Server struct {
	config     ServerConfig
	httpServer *http.Server
	mu         sync.Mutex
	running    bool
	stopCh     chan struct{}
}

// NewServer creates a new Server instance.
func NewServer(cfg ServerConfig) *Server {
	return &Server{
		config: cfg,
		stopCh: make(chan struct{}),
	}
}

// Start initializes all services and starts the HTTP server in a goroutine.
// Returns nil immediately on success; the server runs in the background.
func (s *Server) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.running {
		return fmt.Errorf("server already running")
	}

	// Register CharsetReader
	message.CharsetReader = func(charset string, input io.Reader) (io.Reader, error) {
		enc, err := htmlindex.Get(charset)
		if err != nil {
			return nil, err
		}
		return enc.NewDecoder().Reader(input), nil
	}

	// Ensure directories
	dataRoot := s.config.DataRoot
	if err := os.MkdirAll(dataRoot, 0755); err != nil {
		return fmt.Errorf("failed to create data root %s: %w", dataRoot, err)
	}
	configDir := filepath.Join(dataRoot, "config")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return fmt.Errorf("failed to create config dir %s: %w", configDir, err)
	}

	// Initialize services
	cfg := appconfig.AppConfig
	core.InitCrypto(cfg.AppSecret)
	core.InitAuth(cfg.JWTSecret)
	core.InitDB(filepath.Join(dataRoot, "mail2im.db"))
	appconfig.LoadProviderConfig(filepath.Join(configDir, "providers.json"))
	core.InitEventBus()
	core.InitDebugService()
	core.InitAttachmentManager(filepath.Join(dataRoot, "attachments"))
	core.InitJanitor(30)

	dispatcher.InitDispatcher()
	dispatcher.Instance.Register(channels.NewConsoleChannel(core.PriorityLow))

	go core.StartWatcher()

	// Setup Gin router
	router := s.setupRouter()

	// Start HTTP server
	s.httpServer = &http.Server{
		Addr:    ":" + s.config.Port,
		Handler: router,
	}

	s.running = true
	s.stopCh = make(chan struct{})

	go func() {
		logger.Info("Server starting", zap.String("port", s.config.Port))
		if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("Server error", zap.Error(err))
		}
	}()

	return nil
}

// Stop gracefully shuts down the server and background services.
func (s *Server) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.running {
		return nil
	}

	logger.Info("Stopping server...")

	// Stop watcher
	if core.Watcher != nil {
		core.Watcher.StopAll()
	}

	// Stop janitor
	if core.Cleaner != nil {
		core.Cleaner.Stop()
	}

	// Shutdown HTTP server with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if s.httpServer != nil {
		if err := s.httpServer.Shutdown(ctx); err != nil {
			logger.Error("Server shutdown error", zap.Error(err))
		}
	}

	s.running = false
	close(s.stopCh)

	logger.Info("Server stopped")
	return nil
}

// Running returns whether the server is currently running.
func (s *Server) Running() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.running
}

// Wait blocks until the server is stopped.
func (s *Server) Wait() {
	<-s.stopCh
}

// Port returns the configured port.
func (s *Server) Port() string {
	return s.config.Port
}

func (s *Server) setupRouter() *gin.Engine {
	r := gin.Default()

	// CORS
	corsConfig := cors.DefaultConfig()
	cfg := appconfig.AppConfig
	if cfg != nil && cfg.Env == "production" {
		if cfg.CORSOrigins != "" {
			corsConfig.AllowOrigins = strings.Split(cfg.CORSOrigins, ",")
		}
	} else {
		corsConfig.AllowAllOrigins = true
	}
	corsConfig.AllowMethods = []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"}
	corsConfig.AllowHeaders = []string{"Origin", "Content-Type", "Authorization"}
	corsConfig.ExposeHeaders = []string{"Content-Length"}
	corsConfig.AllowCredentials = true
	corsConfig.MaxAge = 12 * time.Hour
	r.Use(cors.New(corsConfig))

	// Routes
	apiGroup := r.Group("/api")

	apiGroup.GET("/public/emails/:token", api.GetSharedEmail)

	authGroup := apiGroup.Group("/auth")
	authGroup.POST("/setup", api.SetupUser)
	authGroup.POST("/login", api.Login)
	authGroup.POST("/refresh", api.RefreshToken)

	authProtected := apiGroup.Group("/auth")
	authProtected.Use(api.AuthMiddleware())
	authProtected.GET("/me", api.GetMe)
	authProtected.PUT("/profile", api.UpdateProfile)

	protected := apiGroup.Group("/")
	protected.Use(api.AuthMiddleware())

	protected.GET("/config", api.GetSettings)
	protected.POST("/config", api.UpdateSettings)
	protected.POST("/config/export", api.ExportConfig)
	protected.POST("/config/import", api.ImportConfig)
	protected.GET("/logs", api.GetLogs)
	protected.DELETE("/logs/:id", api.DeleteLog)
	protected.DELETE("/logs", api.DeleteAllLogs)
	protected.GET("/debug/stats", api.GetDebugStats)
	protected.POST("/debug/test-event", api.TriggerTestEvent)

	protected.GET("/proxies", api.GetProxies)
	protected.POST("/proxies", api.CreateProxy)
	protected.PUT("/proxies/:id", api.UpdateProxy)
	protected.DELETE("/proxies/:id", api.DeleteProxy)

	protected.GET("/accounts", api.GetAccounts)
	protected.GET("/accounts/:id", api.GetAccount)
	protected.POST("/accounts", api.CreateAccount)
	protected.POST("/accounts/batch", api.CreateAccounts)
	protected.PUT("/accounts/:id", api.UpdateAccount)
	protected.POST("/accounts/test", api.TestAccountConnection)
	protected.DELETE("/accounts/:id", api.DeleteAccount)
	protected.GET("/providers", api.GetProviders)

	protected.GET("/accounts/:id/mailboxes", api.GetMailboxes)
	protected.POST("/accounts/:id/mailboxes/sync", api.SyncMailboxes)
	protected.PUT("/mailboxes/:mailbox_id", api.UpdateMailbox)

	protected.GET("/emails", api.GetEmails)
	protected.GET("/emails/:id", api.GetEmail)
	protected.GET("/emails/:id/html", api.GetEmailHTML)
	protected.DELETE("/emails/:id", api.DeleteEmail)
	protected.POST("/emails/:id/share", api.GenerateShareLink)

	protected.GET("/channels", api.GetChannels)
	protected.POST("/channels", api.CreateChannel)
	protected.PUT("/channels/:id", api.UpdateChannel)
	protected.DELETE("/channels/:id", api.DeleteChannel)
	protected.POST("/channels/test", api.TestChannel)

	protected.GET("/mailtypes", api.GetMailTypes)
	protected.POST("/mailtypes", api.CreateMailType)
	protected.PUT("/mailtypes/:id", api.UpdateMailType)
	protected.DELETE("/mailtypes/:id", api.DeleteMailType)

	protected.GET("/rules", api.GetFolderRules)
	protected.POST("/rules", api.CreateFolderRule)
	protected.PUT("/rules/:id", api.UpdateFolderRule)
	protected.DELETE("/rules/:id", api.DeleteFolderRule)

	protected.GET("/notification-policy", api.GetNotificationPolicy)
	protected.PUT("/notification-policy/:key", api.UpdateNotificationPolicy)

	protected.GET("/templates", api.GetTemplates)
	protected.POST("/templates", api.CreateTemplate)
	protected.PUT("/templates/:id", api.UpdateTemplate)
	protected.DELETE("/templates/:id", api.DeleteTemplate)
	protected.POST("/templates/preview", api.PreviewTemplate)
	protected.GET("/templates/variables", api.GetTemplateVariables)
	protected.GET("/templates/defaults", api.GetDefaultTemplates)

	return r
}
