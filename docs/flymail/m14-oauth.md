# M14 — OAuth2 接入

**状态**：代码完成，真机验收待外部凭据
**优先级**：P2

## 问题

`core/imap/xoauth2.go` 早就有 XOAUTH2 的 SASL 实现，但 backend 没有任何授权流程，
拿不到 access token，这段能力一直悬空。微软停用基本认证后 Outlook.com / M365 账户
接不进来；Gmail 也只能靠应用专用密码，且 Google 正在逐步收紧。

调研中发现两个 roadmap 未写出的事实，它们决定了本次的实际范围：

1. **SMTP 侧完全没有 XOAUTH2**。`core/smtp` 只有 `PlainAuth` 的两个变体，
   `types.SMTPConfig` 连 `AccessToken` 字段都没有。不补这块，OAuth 账户能收不能发。
2. **凭据出口天然收敛**。全仓只有 `Service.IMAPConfig(id)` 与 `SMTPConfig(id)` 两个函数
   产出凭据，sync manager、sync service、send service 全部经由它们。
   「过期前自动刷新」因此只需在这两处注入，无需改动任何调用方。

## 范围

1. `core/smtp` 补 XOAUTH2 认证；`types.SMTPConfig` 加 `AccessToken`
2. `internal/oauth`：授权码 + PKCE、刷新、设备码、loopback 回调（纯协议层）
3. `modules/email/account`：令牌加密存储、过期前自动刷新、授权流程状态机、HTTP API
4. 前端：授权面板、账户向导入口、重新授权引导、诊断面板状态徽标

---

## 认证方式的收敛点

`credentials.go` 新增一个 `secret()`，把两种认证方式的差异压在一处：

```go
func (s *Service) secret(a *Account) (value string, isToken bool, err error) {
	if a.IsOAuth() {
		tok, err := s.AccessToken(a.ID)  // 必要时先刷新
		return tok, true, err
	}
	pw, err := s.enc.Decrypt(a.PasswordEnc)
	return pw, false, err
}
```

`IMAPConfig` / `SMTPConfig` 据此决定填 `AccessToken` 还是 `Password`；
core 侧看到 `AccessToken` 非空就切到 XOAUTH2。同步引擎与发送流程一行都不用改。

---

## 数据模型

`accounts` 表新增三列（`AuthType` 早已存在，此前恒为 `password`）：

```go
OAuthProvider  string     `gorm:"column:oauth_provider"`
OAuthTokenEnc  string     `gorm:"column:oauth_token_enc" json:"-"`
OAuthExpiresAt *time.Time `gorm:"column:oauth_expires_at"`
```

- **整包加密**：access + refresh + scope + email 序列化成一个 JSON 再整体加密存进
  `oauth_token_enc`。令牌结构日后增减字段不必改表。
- **过期时间明文冗余**：`oauth_expires_at` 让「哪些账户快过期了」这类诊断查询
  不必解密全表；它不含任何敏感信息。

### ⚠ GORM 命名策略会拆散 OAuth

必须显式写 `gorm:"column:..."`。GORM 的缩略词表只认 `ID`/`API`/`URL` 这类，
不认 `OAuth`，默认会把 `OAuthProvider` 转成 **`o_auth_provider`**。
而按列名局部更新时手写的键是 `oauth_provider` —— `AutoMigrate` 成功、编译通过，
只有运行到 UPDATE 才报 `no such column`。

---

## 授权流程

### 回调形态：默认 loopback，远程部署走固定地址

**loopback（默认）**：在 `127.0.0.1` 的随机端口上临时监听，即 RFC 8252 给原生应用的标准答案。
配合 PKCE 后无需内嵌任何机密，也不必去服务商后台登记地址。端口取 0 由内核分配——
Google 与 Microsoft 对 `http://127.0.0.1` 的重定向都不校验端口，正是为此场景设计。

**前提是后端与浏览器同机**。Wails 桌面端和本机自用满足；但 FlyMail 也跑在 Docker 里，
那时 `127.0.0.1:<port>` 指向容器自己，用户浏览器根本连不上。因此保留一条分支：

配置 `oauth.redirect_base_url` 后改用后端自身的公开端点
`<base>/api/v1/accounts/oauth/callback`，代价是该地址必须在服务商后台登记为重定向 URI。
该端点**挂在鉴权中间件之外**——它由服务商重定向浏览器直接访问，请求里没有 JWT；
来源校验由一次性的高熵 `state` 承担，用后即弃（堵授权码重放）。

部署方漏配时会出现一种静默失败：用户授权完跳回自己机器的 127.0.0.1，页面打不开，
流程永远停在 pending 且无任何报错。`StartOAuthResponse.loopback` 因此如实回传回调形态，
前端在「loopback 但 FlyMail 是远程访问」时直接给出提示。

### 设备码

Microsoft 个人账户在部分租户策略下走不通 loopback，设备码是官方替代路径。
Google 已于 2022 年停用设备码对 Gmail scope 的支持，故 `googleProvider().DeviceURL` 留空，
发起阶段即拒绝。

### 状态机

流程**只存在于内存**：至多存活 10 分钟，且持有 `code_verifier` 这类一次性机密，
落库既无必要也扩大泄露面。进程重启后未完成的授权自然作废。

```
POST   /accounts/oauth/start          → { flow_id, auth_url | user_code, loopback }
GET    /accounts/oauth/flows/:id      → { status: pending|success|failed, email, error }
POST   /accounts/oauth/complete       → 建号，或为既有账户续上新令牌
DELETE /accounts/oauth/flows/:id      → 取消，释放本地端口
GET    /accounts/oauth/providers      → 提供方清单与配置状态
GET    /accounts/oauth/callback       → 固定回调模式的落点（免鉴权）
```

`complete` 成功后流程立即作废，防止同一次授权被重复消费建出多个账户。

---

## 令牌保鲜

### 刷新提前量 5 分钟

不贴着过期时间：一次完整同步可能持续数分钟，若开始时刚好有效、中途过期，
IMAP 会在半程被断开，写回队列还得重来。

### 必须按账户串行

同步引擎、发送流程与手动触发可能同时发现令牌过期并各自去刷新。
**Microsoft 每次刷新都轮换 refresh_token 并作废上一枚**，两个并发刷新里慢的那个
会拿着已作废的凭据写回数据库，把账户推进「需重新授权」——一个纯由竞态制造的故障。

`accountLock(id)` 按账户加锁，持锁后重新读库并复检有效性：先到的那个可能已经刷过了，
此时直接复用。

### refresh_token 的两种语义

| | Google | Microsoft |
|---|---|---|
| 刷新响应里的 refresh_token | 通常**不返回** | 每次**轮换**一枚新的 |
| 处理 | 沿用旧值 | 用新值覆盖 |

`Client.Refresh` 接收旧值并在响应未回带时回填。不做这层，Google 账户刷新一次
就永久失去刷新能力。

同理，刷新响应不含 `id_token`，邮箱字段要从旧令牌沿用，否则诊断信息会莫名其妙变空。

### 失败分类

- `invalid_grant` / `expired_token` → 授权已被撤销，重试无意义。账户置
  `needs_reauth` 并发 `account_status` 通知。**不禁用账户**——禁用后用户在界面上
  找不到它，反而无法自助修复。
- 5xx / 网络抖动 → 临时故障，不动账户状态，交给同步引擎既有的重试策略。
  一次服务端抖动不该要求用户重走授权。

刷新成功会自动解除 `needs_reauth`。

---

## 授权范围

**Google**：`https://mail.google.com/` 是唯一能同时授权 IMAP 与 SMTP 的 scope，
`gmail.readonly` 等细粒度 scope 只对 Gmail API 生效，走不通 IMAP。
授权 URL 必须带 `access_type=offline` + `prompt=consent` —— 同一账户第二次授权时
若不强制同意页，Google 只回 access_token，刷新链当场断掉。

**Microsoft**：`IMAP.AccessAsUser.All` + `SMTP.Send`，且 `offline_access` 必须显式申请，
否则拿不到 refresh_token，用户每小时重新授权一次。

---

## 邮箱地址的获取

从 `id_token` 载荷解析，省掉一次 userinfo 请求。**不校验签名**：
id_token 是我们自己通过 TLS 直连令牌端点换回来的，不经第三方传递，
属 OIDC Core 3.1.3.7 明确豁免的场景。它只用于预填邮箱，不承担鉴权职责——
真正的权限边界在 access_token 上。

取值顺序 `email` → `preferred_username` → `upn`：**Microsoft 个人账户的 id_token
常常没有 email**，只有 preferred_username。

重新授权时会比对邮箱与账户是否一致。用户在服务商页面上很容易选错账号，
不拦住的话，这个账户会顶着 A 的地址去同步 B 的邮箱。

---

## 安全要点

- **XOAUTH2 要求链路已加密**。`core/smtp` 的 `authFor` 在未加密时直接报错而非降级——
  Bearer 令牌等价于长期凭证，明文外发的后果重于密码。
- **回调页转义**。`error_description` 来自回调 URL（外部可控），
  两处回调页（loopback 与固定端点）都经 `html.EscapeString`。
- **回调响应禁缓存**。URL 里带着授权码，统一 `Cache-Control: no-store` +
  `Referrer-Policy: no-referrer`。
- **loopback 的 state 校验前置**于读取 code：本机任意进程都能往那个端口投递请求。
- **client_secret 可为空**。loopback + PKCE 属公共客户端，Microsoft 公共客户端不需要
  secret，Google「桌面应用」类型虽仍签发但规范上不视其为机密。

---

## 配置

```yaml
oauth:
  google:
    client_id: ""
    client_secret: ""
  microsoft:
    client_id: ""
    client_secret: ""
    tenant: common        # common / organizations / <租户 ID>
  redirect_base_url: ""   # 留空走 loopback；远程部署填公开地址
```

对应环境变量 `FLYMAIL_OAUTH_GOOGLE_CLIENT_ID` 等。

⚠ 每个键都必须 `v.SetDefault(...)` 注册：viper 的 `AutomaticEnv` 只对「已知的 key」
在 Unmarshal 时生效，不注册则环境变量根本读不到（Docker 部署下凭据只能走环境变量）。
这与 `log.dir` / `auth.jwt_secret` 是同一个坑。

未配置 client_id 的提供方，`/accounts/oauth/start` 返回 **501** 而非 400，
与「请求参数错误」区分开，前端据此提示部署方去补配置。
提供方清单也会把未配置的一并返回并标记，让入口置灰而不是凭空消失——
比直接隐藏更容易让部署方意识到还缺一步。

---

## 测试

协议层与账户层全部用 `httptest` 假授权服务器覆盖，**不需要真实凭据**：

- `internal/oauth`：PKCE、授权 URL 的提供方差异、令牌交换、刷新的两种
  refresh_token 语义、错误码映射、设备码三态（pending / slow_down / 成功）、
  id_token 解析回退链、loopback 的 state 校验与 XSS 回归
- `modules/email/account`：令牌加密落库、提前量判定、**并发刷新只发一次请求**、
  invalid_grant → needs_reauth + 通知、5xx 不改状态、刷新成功解除状态、
  两种回调形态的完整闭环、state 一次性、重新授权的邮箱一致性校验
- 路由：`/accounts/oauth/*` 与既有 `/accounts/:id` 的共存（冲突会让服务起不来）
- 前端：`OAuthPanel` 的发起/落库只执行一次、失败展示、设备码渲染、取消释放端口

---

## 待外部凭据的验收项

需注册 Google Cloud 项目与 Azure 应用，属外部准备工作。凭据到位后补验：

- [ ] Gmail 账户经 OAuth 授权后可正常收发
- [ ] Outlook.com 个人账户经设备码流程接入成功
- [ ] token 过期前自动刷新，用户无感（可把 `tokenLeeway` 调大以缩短观察周期）
- [x] 刷新失败时账户进入明确的「需重新授权」状态，诊断面板可见
- [x] token 在数据库中为加密存储

### 申请要点

**Google Cloud**：创建 OAuth 客户端，类型选「桌面应用」（自带 loopback 支持）。
若走固定回调地址则选「Web 应用」并登记
`<base>/api/v1/accounts/oauth/callback`。
需要在 OAuth 同意屏幕申请 `https://mail.google.com/` —— 这是受限 scope，
对外发布需通过 Google 的安全评估；自用可停在「测试」状态并把自己加进测试用户。

**Azure**：注册应用，「支持的账户类型」选「任何组织目录中的账户和个人 Microsoft 账户」，
平台添加「移动和桌面应用程序」并勾选 `https://login.microsoftonline.com/common/oauth2/nativeclient`
与自定义的 loopback 重定向；API 权限添加
`IMAP.AccessAsUser.All`、`SMTP.Send`、`offline_access`。
设备码流程需在「身份验证」里开启「允许公共客户端流」。
