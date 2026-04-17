我现在要开发一款Web部署的邮箱客户端后台, 定位是自托管的单用户, 支持多邮箱账户的应用, 应用名称是 FlyMail.
核心功能:
使用管理员用户登录, 管理多个邮箱账户
用户登录的验证使用 JWT + 刷新 Token
支持标准的 SMTP, IMAP协议, 支持Google 的OAuth 认证
数据库使用SQLITE
支持与前端实时通讯来检测新邮件, 使用 SSE
支持后台的定时任务管理, 可以定时进行一些事务处理, 包括频繁的邮件检查, 不频繁的自动数据库备份之类的

以下是技术要求:
使用最新版本的golang
数据库使用 github.com/glebarez/sqlite 避免依赖 CGO
数据库模型使用 gorm
参数和配置解析使用 viper, 支持默认值, 配置文件, 环境变量, 命令行参数的多层级管理
使用 openapi 描述 API, 支持版本控制, 默认为 api/v1
命令行支持基本的操作, 如:
	flymail server // 使用配置文件的配置启动服务
	flymail db init // 数据库初始化
	flymail db reset-admin-password // 重置管理员密码
所有的配置都存在 ./data/ 下, 并且可以通过命令行参数进行配置

