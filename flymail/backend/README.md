# FlyMail Backend

FlyMail是一款自托管的单用户邮箱客户端后台，支持多邮箱账户管理。

## 功能特性

- ✅ **多邮箱账户管理**: 支持添加、配置和管理多个邮箱账户
- ✅ **协议支持**: 支持SMTP和IMAP协议（含IDLE扩展）
- ✅ **用户认证**: JWT + 刷新Token认证机制
- ✅ **数据库**: 使用SQLite作为数据存储，无CGO依赖
- ✅ **实时通信**: 支持SSE实时推送
- ✅ **异步任务系统**: 完整的任务队列和管理系统
- ✅ **智能邮件监控**: 支持IDLE和轮询模式，自动切换
- ✅ **邮件同步**: 支持全量/增量同步，批量处理
- ✅ **定时任务**: 自动邮件同步和数据库备份
- ✅ **RESTful API**: 完整的REST API支持
- ✅ **OpenAPI文档**: 完整的Swagger UI文档界面
- ✅ **监控系统**: 内置Prometheus格式指标和实时监控
- ✅ **统一响应格式**: 标准化的API响应和错误处理
- 🚧 **Google OAuth**: 计划支持Google OAuth认证

## 技术栈

- **语言**: Go 1.23+
- **Web框架**: Gin
- **数据库**: SQLite (github.com/glebarez/sqlite)
- **ORM**: GORM
- **配置管理**: Viper
- **命令行**: Cobra
- **认证**: JWT (golang-jwt/jwt)
- **定时任务**: Cron
- **实时通信**: SSE (Server-Sent Events)
- **监控**: 内置Prometheus格式指标
- **日志**: Zap (高性能结构化日志)
- **任务队列**: 内置异步任务管理系统

## 安装和使用

### 1. 构建项目

```bash
go build -o bin/flymail cmd/flymail/main.go
```

### 2. 初始化数据库

```bash
./bin/flymail db init
```

### 3. 启动服务器

```bash
./bin/flymail server
```

默认服务器将在 `http://127.0.0.1:8080` 启动。

### 5. 查看API文档

浏览器访问 `http://127.0.0.1:8080/api/swagger` 查看完整的Swagger UI文档界面。

### 6. 重置管理员密码

```bash
./bin/flymail db reset-admin-password
```

## 配置文件

配置文件位于 `./data/config.yaml`：

```yaml
server:
  host: 127.0.0.1
  port: 8080

database:
  path: flymail.db

auth:
  jwt_secret: ""  # 自动生成
  jwt_expiration: 3600
  jwt_refresh_expiration_hours: 168 # 7天
  admin_default_password: admin123

data_dir: "./data"
```

## API 文档

### 在线文档

访问 `http://localhost:8080/docs` 查看完整的交互式 Swagger UI 文档。

在文档界面中，您可以：
- 查看所有API端点的详细说明
- 测试API接口
- 查看请求/响应示例
- 下载OpenAPI规范文件

### API 认证

所有API都需要在请求头中包含JWT token：

```
Authorization: Bearer <your-jwt-token>
```

在Swagger UI中，点击右上角的"Authorize"按钮输入token即可进行认证。

## 日志配置

FlyMail使用高性能的zap日志库，支持不同的日志级别和输出格式。

### 日志级别

在`config.yaml`中配置日志级别：

```yaml
logger:
  level: info  # 可选: debug, info, warn, error
  development: false  # true为开发模式（控制台格式）
  output_paths:
    - stdout
```

### 调试模式

当遇到连接问题（如163邮箱登录失败）时，可以启用调试模式查看详细信息：

1. 使用调试配置文件：
```bash
./bin/flymail server --config ./data/config.debug.yaml
```

2. 或者修改配置文件：
```yaml
logger:
  level: debug
  development: true
```

调试模式会显示：
- IMAP协议交互详情
- HTTP请求和响应内容
- 详细的错误信息
- 数据库查询日志

### 日志输出示例

生产模式（JSON格式）：
```json
{"level":"info","time":"2024-01-01T10:00:00.000Z","caller":"server/server.go:200","msg":"Starting server","address":"0.0.0.0:8080"}
```

开发模式（控制台格式）：
```
2024-01-01T10:00:00.000Z	INFO	server/server.go:200	Starting server	{"address": "0.0.0.0:8080"}
```

## 监控和指标

FlyMail内置了完整的监控系统，提供Prometheus兼容的指标输出。

### 监控端点

- `/api/v1/monitor/metrics` - Prometheus格式的指标
- `/api/v1/monitor/status` - JSON格式的系统状态
- `/api/v1/monitor/health` - 健康检查端点
- `/api/v1/monitor/history` - 历史指标数据

### 指标类型

#### 系统指标
- CPU使用率
- 内存使用情况
- Goroutine数量
- 数据库连接数
- 运行时间

#### 业务指标
- 用户总数
- 邮箱账户数
- 邮件总数
- 今日发送/接收邮件数
- 活跃会话数
- 失败操作统计

#### 实时状态
- 活跃连接数
- 任务队列状态
- 服务健康状态

### Prometheus集成

在`prometheus.yml`中添加：

```yaml
scrape_configs:
  - job_name: 'flymail'
    static_configs:
      - targets: ['localhost:8080']
    metrics_path: '/api/v1/monitor/metrics'
```

## API 使用示例

### 登录

```bash
curl -X POST http://localhost:8080/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{
    "username": "admin",
    "password": "admin123"
  }'
```

### 添加邮箱账户

```bash
curl -X POST http://localhost:8080/api/v1/accounts \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "我的Gmail",
    "email": "user@gmail.com",
    "type": "imap",
    "imap_server": "imap.gmail.com",
    "imap_port": 995,
    "imap_ssl": true,
    "smtp_server": "smtp.gmail.com",
    "smtp_port": 587,
    "smtp_ssl": true,
    "username": "user@gmail.com",
    "password": "app-password"
  }'
```

### 同步邮件

```bash
curl -X POST http://localhost:8080/api/v1/accounts/1/sync \
  -H "Authorization: Bearer <token>"
```

### 获取邮件列表

```bash
curl http://localhost:8080/api/v1/emails \
  -H "Authorization: Bearer <token>"
```

## SSE (Server-Sent Events) 连接

连接到 SSE 以接收实时通知：

```javascript
const eventSource = new EventSource('/api/v1/events', {
  headers: {
    'Authorization': 'Bearer <token>'
  }
});

eventSource.onmessage = function(event) {
  const data = JSON.parse(event.data);
  console.log('收到通知:', data);
};
```

## 异步任务系统

FlyMail内置了强大的异步任务管理系统，用于处理邮件同步等长时间运行的操作。

### 任务管理API

#### 创建任务
```bash
curl -X POST http://localhost:8080/api/v1/tasks \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{
    "type": "email_sync",
    "priority": "normal",
    "params": {
      "account_id": 1,
      "force": true
    }
  }'
```

#### 查看任务列表
```bash
curl http://localhost:8080/api/v1/tasks \
  -H "Authorization: Bearer <token>"
```

#### 实时任务进度（SSE）
```bash
curl http://localhost:8080/api/v1/tasks/1/stream \
  -H "Authorization: Bearer <token>" \
  -H "Accept: text/event-stream"
```

### 支持的任务类型

- **email_sync**: 邮件同步任务
  - 支持全量同步（force=true）
  - 支持增量同步
  - 支持指定文件夹
  - 批量处理避免长连接问题

- **email_sync_all**: 所有账户邮件同步
  - 并发同步所有邮箱账户
  - 自动创建子任务

### 任务优先级

- **high**: 高优先级，立即执行
- **normal**: 正常优先级（默认）
- **low**: 低优先级，在空闲时执行

## 邮件监控系统

FlyMail实现了智能的邮件监控系统，能够实时检测新邮件并自动触发同步。

### 监控模式

系统会根据邮件服务器的能力自动选择最佳的监控模式：

#### 1. IDLE模式（推荐）
- 使用IMAP IDLE扩展保持长连接
- 服务器主动推送新邮件通知
- 实时性高，资源消耗低
- 支持的服务器：Gmail、Outlook、163等

#### 2. 轮询模式
- 定期检查新邮件
- 智能时段调度：
  - 白天（8:00-22:00）：每分钟检查
  - 夜间（22:00-8:00）：每10分钟检查
- 适用于不支持IDLE的服务器

### 监控特性

- **自动重连**: 连接断开时自动重新建立
- **健康检查**: 定期发送NOOP命令保持连接活跃
- **重复检测**: 避免创建重复的同步任务
- **多账户并发**: 每个账户独立监控线程

## 架构设计

FlyMail采用分层架构设计，包含以下层次：

### 1. API层 (Handlers)
- 处理HTTP请求和响应
- 参数验证
- 调用Service层
- SSE事件推送

### 2. Service层
- 业务逻辑实现
- 事务管理
- 指标收集
- 任务调度

### 3. Repository层
- 数据访问抽象
- 数据库操作封装
- 查询构建

### 4. 任务系统
- 异步任务管理器
- 任务队列和调度
- Worker Pool模式
- 任务状态持久化

### 5. 监控系统
- 邮件账户监控（IDLE/轮询）
- 系统指标收集
- 业务指标统计
- Prometheus格式输出

## 目录结构

```
.
├── cmd/
│   └── flymail/
│       └── main.go              # 主程序入口
├── internal/
│   ├── config/                  # 配置管理
│   ├── database/                # 数据库连接和初始化
│   ├── email/                   # 邮件协议实现 (SMTP/IMAP)
│   ├── handlers/                # HTTP 处理器
│   ├── metrics/                 # 监控指标收集
│   ├── middleware/              # 中间件
│   ├── models/                  # 数据模型
│   ├── repositories/            # 数据访问层
│   │   ├── interfaces/          # Repository接口定义
│   │   └── impl/                # Repository实现
│   ├── scheduler/               # 定时任务
│   ├── server/                  # 服务器配置
│   ├── services/                # 业务逻辑层
│   ├── tasks/                   # 任务处理器
│   ├── queue/                   # 任务队列
│   ├── monitor/                 # 邮件监控
│   ├── sse/                     # SSE实时通信
│   │   ├── interfaces/          # Service接口定义
│   │   ├── auth/                # 认证服务
│   │   ├── email/               # 邮件服务
│   │   ├── account/             # 账户服务
│   │   └── metrics/             # 监控服务
├── pkg/
│   ├── jwt/                     # JWT 工具
│   └── utils/                   # 通用工具
├── api/
│   └── v1/
│       └── openapi.yaml         # OpenAPI 规范
├── data/                        # 数据目录
│   ├── config.yaml              # 配置文件
│   └── flymail.db               # SQLite 数据库
└── docs/                        # 文档
```

## 开发计划

### 待实现功能
- [ ] Google OAuth 认证支持
- [ ] 邮件搜索功能
- [ ] 邮件模板功能
- [ ] 数据导入/导出
- [ ] 多语言支持
- [ ] 邮件规则和过滤器

### 已完成功能
- [x] 项目基础架构
- [x] 用户认证系统 (JWT)
- [x] 邮箱账户管理
- [x] SMTP 协议支持
- [x] IMAP 协议支持（含IDLE扩展）
- [x] 邮件收发和同步
- [x] 邮件标签和文件夹管理
- [x] 邮件附件支持
- [x] 实时通讯 (SSE)
- [x] 异步任务管理系统
- [x] 智能邮件监控
- [x] 定时任务调度
- [x] RESTful API
- [x] OpenAPI 文档
- [x] 监控和指标系统
- [x] 统一响应格式
- [x] Web 示例客户端

## 许可证

MIT License