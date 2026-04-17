# 163邮箱特殊支持说明

## 概述

FlyMail 已经添加了对网易163系列邮箱（163.com、126.com、yeah.net、yeah.com）的特殊支持。这些邮箱服务器需要在IMAP连接后发送特殊的ID命令才能正常获取邮件。

## 实现细节

### 1. 自动检测163邮箱

系统会自动检测IMAP服务器地址是否包含以下域名：
- 163.com
- 126.com
- yeah.net
- yeah.com

### 2. ID命令发送

当检测到163系列邮箱时，系统会在登录成功后自动发送IMAP ID扩展命令：

```
ID ("name" "FlyMail" "version" "1.0.0" "vendor" "FlyMail")
```

### 3. 错误处理

- 如果服务器不支持ID扩展，会记录调试日志但不会失败
- 如果ID命令发送失败，会记录警告日志但不会中断连接
- 确保即使ID命令失败，邮箱连接仍然可以继续使用

## 配置示例

### 163邮箱配置
```json
{
  "name": "我的163邮箱",
  "email": "username@163.com",
  "type": "personal",
  "imap_server": "imap.163.com",
  "imap_port": 993,
  "imap_ssl": true,
  "smtp_server": "smtp.163.com",
  "smtp_port": 465,
  "smtp_ssl": true,
  "username": "username@163.com",
  "password": "授权码(不是登录密码)"
}
```

### 126邮箱配置
```json
{
  "name": "我的126邮箱",
  "email": "username@126.com",
  "type": "personal",
  "imap_server": "imap.126.com",
  "imap_port": 993,
  "imap_ssl": true,
  "smtp_server": "smtp.126.com",
  "smtp_port": 465,
  "smtp_ssl": true,
  "username": "username@126.com",
  "password": "授权码(不是登录密码)"
}
```

### Yeah邮箱配置
```json
{
  "name": "我的Yeah邮箱",
  "email": "username@yeah.net",
  "type": "personal",
  "imap_server": "imap.yeah.net",
  "imap_port": 993,
  "imap_ssl": true,
  "smtp_server": "smtp.yeah.net",
  "smtp_port": 465,
  "smtp_ssl": true,
  "username": "username@yeah.net",
  "password": "授权码(不是登录密码)"
}
```

## 注意事项

1. **使用授权码**：163系列邮箱必须使用授权码而不是登录密码。请在邮箱设置中开启IMAP/SMTP服务并生成授权码。

2. **端口设置**：
   - IMAP推荐使用993端口（SSL/TLS）
   - SMTP推荐使用465端口（SSL/TLS）

3. **服务器地址**：
   - 163邮箱：imap.163.com / smtp.163.com
   - 126邮箱：imap.126.com / smtp.126.com
   - Yeah邮箱：imap.yeah.net / smtp.yeah.net

## 故障排除

### 连接失败
1. 确认已在邮箱设置中开启IMAP/SMTP服务
2. 确认使用的是授权码而不是登录密码
3. 检查防火墙是否允许相应端口的连接

### 无法收取邮件
1. 查看日志中是否有"Successfully sent IMAP ID command"的信息
2. 如果没有，可能是服务器地址未被正确识别为163系列
3. 确认IMAP服务器地址设置正确

### 日志查看
启用调试模式可以查看详细的IMAP协议交互：
```bash
export LOG_LEVEL=debug
./flymail
```

## 技术实现

相关代码位于 `internal/email/imap.go`：

1. `is163Server()` - 检测是否为163系列邮箱服务器
2. `sendIMAPID()` - 发送IMAP ID扩展命令
3. `connect()` - 在连接成功后自动调用ID命令

这个实现确保了与163系列邮箱的兼容性，同时不影响其他邮箱服务器的正常使用。