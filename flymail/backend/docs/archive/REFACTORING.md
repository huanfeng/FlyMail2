# 模块化重构进展

## 重构目标

将项目从分层架构迁移到模块化架构，实现：
- 高内聚低耦合
- 功能修改集中在单一模块
- 清晰的模块边界和依赖关系

## 已完成的工作（更新时间：2025-07-17）

### 1. 基础架构搭建

创建了新的目录结构：
- `modules/` - 业务模块
- `shared/` - 共享代码（config, database, middleware等）
- `pkg/` - 通用工具包（保持不变）

### 2. Auth模块迁移

将认证相关代码整合到 `modules/auth/`：

```
modules/auth/
├── handler.go      # HTTP处理器（原 internal/api/handler/auth.go）
├── service.go      # 业务逻辑（原 internal/service/auth.go）
├── repository.go   # 数据访问（基于 internal/store/db/user.go）
├── model.go        # 用户模型（原 internal/store/model/models.go 的 User 部分）
└── middleware.go   # 认证中间件（原 internal/api/middleware/auth.go）
```

主要改进：
- 移除了对 MonitorCollector 的依赖
- 统一了模块内的数据流
- 保持了原有的接口兼容性

### 3. System/Setting模块迁移

将设置管理整合到 `modules/system/setting/`：

```
modules/system/setting/
├── handler.go      # HTTP处理器
├── service.go      # 业务逻辑
├── repository.go   # 数据访问
└── model.go        # 设置模型和相关类型
```

特点：
- 支持通用设置和专用设置（如邮件监控设置）
- 提供了导入/导出功能
- 保留了原有的验证逻辑

### 4. 共享代码迁移

将通用代码移到 `shared/`：
- `shared/config/` - 配置管理
- `shared/database/` - 数据库连接
- `shared/types/` - 共享类型定义

### 5. Email/Account模块迁移

将邮箱账户管理整合到 `modules/email/account/`：

```
modules/email/account/
├── handler.go      # HTTP处理器
├── service.go      # 业务逻辑（含账户初始化、连接测试等）
├── repository.go   # 数据访问
├── model.go        # 账户模型和相关类型
└── dto.go          # 请求/响应DTO
```

特点：
- 包含完整的账户CRUD功能
- 支持连接测试和能力检测
- 保留了与folder同步和email监控的接口
- 通过接口解耦，避免循环依赖

### 6. Email/Folder模块迁移

将文件夹管理整合到 `modules/email/folder/`：

```
modules/email/folder/
├── handler.go      # HTTP处理器
├── service.go      # 业务逻辑（含排序算法）
├── repository.go   # 数据访问
├── model.go        # 文件夹模型
├── dto.go          # 请求/响应DTO
└── types.go        # 文件夹类型定义和判断逻辑
```

特点：
- 智能文件夹类型识别（支持中英文）
- 自动排序管理（系统文件夹优先）
- 支持IMAP文件夹同步
- 实时邮件计数统计

### 7. Email/Message模块迁移

将邮件消息管理整合到 `modules/email/message/`：

```
modules/email/message/
├── handler.go      # HTTP处理器（邮件列表、详情、操作）
├── service.go      # 业务逻辑（搜索、过滤、批量操作）
├── repository.go   # 数据访问（含附件管理）
├── model.go        # 邮件和附件模型
└── dto/            # 请求响应DTO
    ├── request.go  # 批量操作请求
    └── response.go # 列表和详情响应
```

特点：
- 支持多种过滤条件（账户、文件夹、虚拟文件夹、已读/未读、星标等）
- 全文搜索功能（主题、发件人、收件人、正文）
- 批量操作（批量更新状态、批量删除）
- 附件管理
- 服务端删除接口预留

### 8. Email/Sync模块迁移

将邮件同步服务整合到 `modules/email/sync/`：

```
modules/email/sync/
├── config.go          # 同步配置
├── types.go           # 类型定义
├── account_monitor.go # 单个账户监控器
├── service.go         # 同步服务
└── handler.go         # HTTP处理器
```

特点：
- 支持IMAP IDLE实时推送
- 智能轮询策略（日天/夜间不同频率）
- 错误重试机制
- 手动同步接口
- 状态监控和查询

### 9. Email/Protocol模块迁移

将邮件协议实现整合到 `modules/email/protocol/`：

```
modules/email/protocol/
├── imap.go    # IMAP协议实现
├── smtp.go    # SMTP协议实现
└── factory.go # 协议工厂
```

特点：
- IMAP协议完整实现（连接、认证、IDLE、收取、删除）
- SMTP协议实现（发送邮件）
- 支持SSL/TLS和STARTTLS
- 字符集编码支持（包括GBK、GB2312等）
- 连接池和会话管理

### 10. System/Monitor模块迁移

将系统监控服务整合到 `modules/system/monitor/`：

```
modules/system/monitor/
├── types.go      # 类型定义
├── collector.go  # 数据收集器
├── service.go    # 监控服务
├── handler.go    # HTTP处理器
└── middleware.go # 监控中间件
```

特点：
- 系统级别状态监控（CPU、内存、GC、Goroutine）
- 服务健康检查
- 实时指标收集
- 错误统计和分析
- 请求速率监控

### 11. System/Task模块迁移

将任务管理系统整合到 `modules/system/task/`：

```
modules/system/task/
├── types.go            # 类型定义
├── repository.go       # 数据访问层
├── queue.go            # 优先级队列
├── errors.go           # 错误定义
├── manager.go          # 任务管理器
├── manager_internal.go # 管理器内部实现
└── handler.go          # HTTP处理器
```

特点：
- 多种任务类型（循环、定时、一次性）
- 优先级队列调度
- Cron表达式支持
- 任务执行日志
- 事件通知机制
- 并发工作者池

### 12. Notify模块迁移

将通知系统整合到 `modules/notify/`：

```
modules/notify/
├── types.go              # 类型定义（事件类型、严重性等）
├── model.go              # 数据模型（通知渠道、日志等）
├── repository.go         # 数据访问层
├── service.go            # 通知服务（管理器实现）
├── handler.go            # HTTP处理器
├── helper.go             # 辅助函数（全局通知API）
├── sse_integration.go    # SSE集成
└── channels/
    └── interface.go      # 渠道接口和基础实现
```

特点：
- 支持多种通知渠道（飞书、企业微信、Telegram、Email、Webhook、SSE）
- 渠道时间范围控制
- 事件类型过滤
- 失败重试机制
- 通知日志记录
- 与SSE实时推送集成

### 13. Realtime模块迁移

将实时通信（SSE）系统整合到 `modules/realtime/`：

```
modules/realtime/
├── types.go                  # 类型定义
├── client.go                 # SSE客户端实现
├── hub.go                    # 事件分发中心
├── subscription_manager.go   # 订阅管理
└── handler.go                # HTTP处理器
```

特点：
- Server-Sent Events (SSE) 实现
- 基于主题的事件广播
- 客户端订阅管理
- 自动心跳保活
- 支持事件过滤
- 与通知系统深度集成

## 下一步计划

### Email模块群（优先级：高）
1. ✅ `email/account` - 邮箱账户管理
2. ✅ `email/folder` - 文件夹管理
3. ✅ `email/message` - 邮件消息处理
4. ✅ `email/sync` - 邮件同步（原email_monitor）
5. ✅ `email/protocol` - IMAP/SMTP协议实现

### System模块群（优先级：中）
1. ✅ `system/setting` - 系统设置
2. ✅ `system/monitor` - 系统监控
3. ✅ `system/task` - 任务管理

### 其他模块（优先级：低）
1. ✅ `notify` - 通知系统
2. ✅ `realtime` - 实时通信（SSE）

## 迁移指南

### 对于新功能
直接在相应模块下开发，遵循模块结构规范。

### 对于现有代码修改
1. 如果涉及的模块已迁移，在新模块中修改
2. 如果模块未迁移，可以继续在原位置修改，待后续统一迁移

### 导入路径更新
- 原：`flymail/internal/config`
- 新：`flymail/shared/config`

## 注意事项

1. **保持向后兼容**：迁移过程中保持API接口不变
2. **逐步迁移**：每次只迁移一个模块，确保稳定性
3. **充分测试**：每个模块迁移后需要完整测试
4. **文档更新**：及时更新相关文档和注释