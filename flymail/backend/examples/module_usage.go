package examples

import (
	"context"
	"log"

	"gorm.io/gorm"

	"flymail/modules/auth"
	"flymail/modules/system/setting"
	"flymail/shared/config"
	"flymail/shared/database"
)

// ModuleUsageExample 展示如何使用模块化架构
func ModuleUsageExample() {
	// 1. 初始化配置
	cfg := config.GetConfig()

	// 2. 初始化数据库
	err := database.Init(cfg)
	if err != nil {
		log.Fatal(err)
	}

	// 3. 创建模块实例
	// Auth模块
	db := database.GetDB()
	authRepo := auth.NewRepository(db.MainDB)
	authService := auth.NewService(authRepo, cfg)
	authHandler := auth.NewHandler(authService)

	// Setting模块
	settingRepo := setting.NewRepository(db.MainDB)
	settingService := setting.NewService(settingRepo)
	settingHandler := setting.NewHandler(settingService)

	// 4. 使用示例
	ctx := context.Background()

	// 登录示例
	authResp, err := authService.Login(ctx, "admin", "password")
	if err != nil {
		log.Printf("Login failed: %v", err)
	} else {
		log.Printf("Login successful, token: %s", authResp.AccessToken)
	}

	// 设置示例
	err = settingService.SetSetting(ctx, "app_name", "FlyMail")
	if err != nil {
		log.Printf("Set setting failed: %v", err)
	}

	value, err := settingService.GetSetting(ctx, "app_name")
	if err != nil {
		log.Printf("Get setting failed: %v", err)
	} else {
		log.Printf("App name: %s", value)
	}

	// 5. 注册路由（在实际应用中）
	// router := gin.New()
	// authGroup := router.Group("/api/v1/auth")
	// {
	//     authGroup.POST("/login", authHandler.Login)
	//     authGroup.POST("/refresh", authHandler.Refresh)
	//     authGroup.GET("/me", auth.AuthMiddleware(), authHandler.Me)
	// }
	//
	// settingGroup := router.Group("/api/v1/settings", auth.AuthMiddleware())
	// {
	//     settingGroup.GET("/:key", settingHandler.GetSetting)
	//     settingGroup.PUT("/:key", settingHandler.UpdateSetting)
	//     settingGroup.GET("/", settingHandler.GetAllSettings)
	// }

	_ = authHandler
	_ = settingHandler
}

// 模块依赖注入示例
type ServiceRegistry struct {
	AuthService    auth.Service
	SettingService setting.Service
	// 其他服务...
}

// NewServiceRegistry 创建服务注册表
func NewServiceRegistry(db *gorm.DB, cfg *config.Config) *ServiceRegistry {
	// 创建repositories
	authRepo := auth.NewRepository(db)
	settingRepo := setting.NewRepository(db)

	// 创建services
	authService := auth.NewService(authRepo, cfg)
	settingService := setting.NewService(settingRepo)

	return &ServiceRegistry{
		AuthService:    authService,
		SettingService: settingService,
	}
}
