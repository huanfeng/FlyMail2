# 真实邮箱测试指南

## 概述

真实邮箱测试允许你使用真实的邮箱账号（如 Gmail、Outlook、163、QQ 等）来测试 IMAP 连接功能。这些测试会验证：

- IMAP 连接是否成功
- IDLE 模式支持
- 代理连接
- 错误处理（无效凭据、无效服务器等）

## 配置步骤

### 1. 创建测试配置文件

复制示例配置文件：

```bash
cp .env.test.example .env.test
```

### 2. 编辑配置文件

编辑 `.env.test` 文件，填入真实的邮箱信息：

```bash
# 测试邮箱账号 1 (Gmail)
TEST_EMAIL_1=your-email@gmail.com
TEST_PASSWORD_1=your-app-password
TEST_IMAP_SERVER_1=imap.gmail.com
TEST_IMAP_PORT_1=993
TEST_SSL_MODE_1=ssl

# 测试邮箱账号 2 (Outlook)
TEST_EMAIL_2=your-email@outlook.com
TEST_PASSWORD_2=your-password
TEST_IMAP_SERVER_2=outlook.office365.com
TEST_IMAP_PORT_2=993
TEST_SSL_MODE_2=ssl
```

### 3. Gmail 特殊配置

Gmail 需要使用"应用专用密码"而不是普通密码：

1. 启用两步验证：https://myaccount.google.com/security
2. 生成应用专用密码：https://myaccount.google.com/apppasswords
3. 使用生成的密码作为 `TEST_PASSWORD_1`

### 4. 163/QQ 邮箱配置

163 和 QQ 邮箱需要开启 IMAP 服务并获取授权码：

**163 邮箱：**
1. 登录 163 邮箱
2. 设置 → POP3/SMTP/IMAP
3. 开启 IMAP 服务
4. 获取授权码

**QQ 邮箱：**
1. 登录 QQ 邮箱
2. 设置 → 账户
3. 开启 IMAP/SMTP 服务
4. 获取授权码

## 运行测试

### 运行所有真实邮箱测试

```bash
# 加载环境变量并运行测试
source .env.test  # Linux/Mac
# 或
export $(cat .env.test | xargs)  # Linux/Mac

# Windows PowerShell
Get-Content .env.test | ForEach-Object {
    if ($_ -match '^([^=]+)=(.*)$') {
        [Environment]::SetEnvironmentVariable($matches[1], $matches[2], "Process")
    }
}

# 运行测试
go test ./internal/e2e/ -v -run "TestRealAccount" -count=1
```

### 运行特定测试

```bash
# 测试 IMAP 连接
go test ./internal/e2e/ -v -run "TestRealAccount_IMAPConnection" -count=1

# 测试代理连接
go test ./internal/e2e/ -v -run "TestRealAccount_IMAPConnection_WithProxy" -count=1

# 测试无效凭据
go test ./internal/e2e/ -v -run "TestRealAccount_IMAPConnection_InvalidCredentials" -count=1

# 测试无效服务器
go test ./internal/e2e/ -v -run "TestRealAccount_IMAPConnection_InvalidServer" -count=1
```

### 使用 Makefile

```bash
# 运行所有 E2E 测试（包括真实邮箱测试）
make test-e2e-real
```

## 测试输出示例

```
=== RUN   TestRealAccount_IMAPConnection/user@gmail.com
    real_account_test.go:35: 连接成功: 1.234s
    real_account_test.go:36:   支持 IDLE: true
    real_account_test.go:37:   安全模式: ssl
    real_account_test.go:38:   能力: [IMAP4rev1 IDLE SASL-IR LOGIN-REFERRALS ENABLE ID]
--- PASS: TestRealAccount_IMAPConnection/user@gmail.com (1.23s)
```

## 故障排除

### 连接超时

如果连接超时，可能是网络问题或防火墙阻止：

```bash
# 测试网络连接
telnet imap.gmail.com 993

# 检查防火墙设置
```

### 认证失败

- Gmail: 确保使用应用专用密码，不是普通密码
- 163/QQ: 确保使用授权码，不是普通密码
- Outlook: 确保账户安全设置允许 IMAP 访问

### 代理问题

如果使用代理连接：

```bash
# 测试代理连接
curl -x socks5://proxy:1080 https://imap.gmail.com
```

## 安全注意事项

1. **不要提交 `.env.test` 文件**：该文件已添加到 `.gitignore`
2. **使用应用专用密码**：不要使用主密码
3. **定期更换密码**：测试完成后可以删除或更换密码
4. **限制权限**：测试账号应该只有必要的权限

## 最佳实践

1. **使用专门的测试账号**：不要使用个人邮箱
2. **清理测试数据**：测试完成后清理测试邮件
3. **并行测试**：多个账号可以并行测试
4. **记录问题**：记录连接问题以便排查

## 相关文件

- `.env.test.example` - 配置文件示例
- `internal/testutil/testutil.go` - 测试工具函数
- `internal/e2e/real_account_test.go` - 真实邮箱测试
