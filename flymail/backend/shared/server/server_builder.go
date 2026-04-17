package server

import (
	"fmt"

	"flymail/modules/auth"
	"flymail/modules/email/account"
	"flymail/modules/email/folder"
	"flymail/modules/email/message"
	"flymail/modules/email/sync"
	sse "flymail/modules/realtime"
	"flymail/modules/system/monitor"
	"flymail/modules/system/setting"
	"flymail/modules/system/task"
	"flymail/pkg/logger"
	"flymail/shared/config"
	"flymail/shared/database"
	"flymail/shared/middleware"
	"github.com/gin-gonic/gin"
)

// ServerBuilder 用于逐步构建 Server
type ServerBuilder struct {
	config   *config.Config
	database *database.ServerDB
	server   *Server
	errors   []error
}

// NewBuilder 创建一个新的 ServerBuilder
func NewBuilder(config *config.Config) *ServerBuilder {
	return &ServerBuilder{
		config: config,
		server: &Server{
			config: config,
		},
	}
}

// WithDatabase 设置数据库
func (b *ServerBuilder) WithDatabase() *ServerBuilder {
	db := database.GetDB()
	if db == nil {
		b.addError(fmt.Errorf("database not initialized"))
		return b
	}
	b.database = db
	return b
}

// WithRouter 初始化路由
func (b *ServerBuilder) WithRouter() *ServerBuilder {
	gin.SetMode(gin.ReleaseMode)
	if b.config.Logger.Development {
		gin.SetMode(gin.DebugMode)
	}

	router := gin.New()

	// 禁用Gin的自动重定向尾随斜杠, 防止CORS问题
	router.RedirectTrailingSlash = false

	// 全局中间件
	router.Use(gin.Recovery())
	router.Use(middleware.LoggerMiddleware())
	router.Use(middleware.MonitorMiddleware())
	router.Use(middleware.SetupCORS(b.config))

	// 设置受信任的代理 - 使用默认值
	if err := router.SetTrustedProxies([]string{"127.0.0.1", "::1"}); err != nil {
		b.addError(fmt.Errorf("set trusted proxies: %w", err))
		return b
	}

	b.server.router = router
	return b
}

// WithSSEHub 初始化 SSE Hub
func (b *ServerBuilder) WithSSEHub() *ServerBuilder {
	hub := sse.NewHub(logger.Logger)
	b.server.sseHub = hub

	// 启动 SSE Hub
	if err := hub.Start(); err != nil {
		b.addError(fmt.Errorf("start SSE hub: %w", err))
		return b
	}

	return b
}

// WithServices 初始化所有服务
func (b *ServerBuilder) WithServices() *ServerBuilder {
	if b.database == nil {
		b.addError(fmt.Errorf("database must be initialized first"))
		return b
	}

	if b.server.sseHub == nil {
		b.addError(fmt.Errorf("SSE hub must be initialized first"))
		return b
	}

	// Initialize auth module
	authRepo := auth.NewRepository(b.database.MainDB)
	authService := auth.NewService(authRepo, b.config)

	// Initialize email modules
	emailAccountRepo := account.NewRepository(b.database.MainDB)
	emailAccountService := account.NewService(emailAccountRepo)

	emailMessageRepo := message.NewRepository(b.database.MainDB)
	emailMessageService := message.NewService(emailMessageRepo)

	// Initialize folder module
	folderRepo := folder.NewRepository(b.database.MainDB)
	// TODO: emailAccountService 需要实现 AccountVerifier 接口
	// 暂时传 nil，等后续实现
	folderService := folder.NewService(folderRepo, nil)

	// Create sync service with dependencies
	syncConfig := sync.DefaultConfig()
	emailSyncService := sync.NewService(emailAccountRepo, emailMessageRepo, emailMessageService, syncConfig)

	// Initialize system modules
	settingRepo := setting.NewRepository(b.database.MainDB)
	settingService := setting.NewService(settingRepo)

	// Initialize monitor module
	monitorCollector := monitor.NewCollector()
	monitorService := monitor.NewService(monitorCollector, b.database.MainDB)
	// TODO: 设置可选依赖项，需要类型断言或其他方式

	// Initialize task module
	taskRepo := task.NewConfigRepository(b.database.MainDB)
	taskLogRepo := task.NewLogRepository(b.database.MainDB)
	taskManager := task.NewManager(taskRepo, taskLogRepo, 5) // 5 workers

	// Store services in server
	b.server.services = &Services{
		Auth:    authService,
		Account: emailAccountService,
		Message: emailMessageService,
		Folder:  folderService,
		Sync:    emailSyncService,
		Setting: settingService,
		Monitor: monitorService,
		Task:    taskManager,
	}

	return b
}

// WithRoutes 设置路由
func (b *ServerBuilder) WithRoutes() *ServerBuilder {
	if b.server.router == nil {
		b.addError(fmt.Errorf("router must be initialized first"))
		return b
	}

	if b.server.services == nil {
		b.addError(fmt.Errorf("services must be initialized first"))
		return b
	}

	setupRoutes(b.server.router, b.server.services, b.server.sseHub, b.config)
	return b
}

// Build 构建最终的 Server 实例
func (b *ServerBuilder) Build() (*Server, error) {
	if len(b.errors) > 0 {
		return nil, fmt.Errorf("failed to build server: %v", b.errors)
	}

	if b.server.router == nil {
		return nil, fmt.Errorf("router not initialized")
	}

	if b.server.services == nil {
		return nil, fmt.Errorf("services not initialized")
	}

	return b.server, nil
}

// addError 添加错误到错误列表
func (b *ServerBuilder) addError(err error) {
	b.errors = append(b.errors, err)
}

// Services holds all the services
type Services struct {
	Auth    auth.Service
	Account account.Service
	Message message.Service
	Folder  folder.Service
	Sync    sync.Service
	Setting setting.Service
	Monitor monitor.Service
	Task    task.Manager
}
