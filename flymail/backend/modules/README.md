# 模块化架构说明

本项目采用模块化架构，将代码按功能而非层级进行组织。

## 目录结构

```
modules/
├── auth/                # 认证授权模块
├── email/              # 邮件核心模块
│   ├── account/        # 邮箱账户管理
│   ├── folder/         # 文件夹管理
│   ├── message/        # 邮件消息管理
│   ├── sync/           # 邮件同步（原email_monitor）
│   └── protocol/       # 邮件协议实现
├── system/             # 系统管理模块
│   ├── monitor/        # 系统监控
│   ├── task/           # 任务管理
│   └── setting/        # 系统设置
├── notify/             # 通知模块
└── realtime/           # 实时通信模块（SSE）
```

## 模块结构

每个模块遵循统一的结构：

```
module/
├── handler.go      # HTTP处理器
├── service.go      # 业务逻辑
├── repository.go   # 数据访问
├── model.go        # 数据模型
└── dto.go          # 数据传输对象（可选）
```

## 依赖原则

1. 模块间通过接口通信
2. 避免循环依赖
3. 共享代码放在 `shared/` 目录
4. 通用工具放在 `pkg/` 目录

## 使用示例

### 创建新模块

```go
// modules/mymodule/service.go
package mymodule

type Service interface {
    DoSomething(ctx context.Context) error
}

type service struct {
    repo Repository
}

func NewService(repo Repository) Service {
    return &service{repo: repo}
}
```

### 注册模块

在服务器初始化时注册模块：

```go
// 创建repository
myRepo := mymodule.NewRepository(db)

// 创建service
myService := mymodule.NewService(myRepo)

// 创建handler
myHandler := mymodule.NewHandler(myService)

// 注册路由
router.GET("/api/v1/mymodule", myHandler.Get)
```

## 已完成的模块

- ✅ auth - 认证授权模块
- ✅ system/setting - 系统设置模块
- ✅ system/monitor - 系统监控模块
- ✅ system/task - 任务管理模块
- ✅ email/account - 邮箱账户模块
- ✅ email/folder - 文件夹模块
- ✅ email/message - 邮件消息模块
- ✅ email/sync - 邮件同步模块
- ✅ email/protocol - 邮件协议模块
- ✅ notify - 通知模块
- ✅ realtime - 实时通信模块