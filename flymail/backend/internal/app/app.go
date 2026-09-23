package app

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"flymail-core/logger"

	"flymail/internal/config"
	"flymail/internal/crypto"
	"flymail/internal/database"
	"flymail/internal/logging"
	"flymail/internal/server"
	"flymail/internal/sse"
	"flymail/modules/auth"
	"flymail/modules/email/account"
	"flymail/modules/email/draft"
	"flymail/modules/email/folder"
	"flymail/modules/email/message"
	"flymail/modules/email/rule"
	"flymail/modules/email/send"
	syncmod "flymail/modules/email/sync"
	"flymail/modules/system/monitoring"
	"flymail/modules/system/notify"
	"flymail/modules/system/privacy"
	"flymail/modules/system/setting"

	"go.uber.org/zap"
	"gorm.io/gorm"
)

// appVersion 监控/关于展示用的版本号（后续可由构建注入）。
const appVersion = "dev-preview"

type App struct {
	cfg      *config.Config
	srv      *http.Server
	addr     string
	manager  *syncmod.Manager
	cancel   context.CancelFunc
	logClose func() error
	db       *gorm.DB
	authSvc  *auth.Service

	// emitHook 是通知事件的额外观察者（桌面形态注入：新邮件弹系统 toast）。
	// 与 notify 落库/外发解耦，为空时零开销。messageID 仅单封新邮件事件非 0。
	emitHookMu sync.RWMutex
	emitHook   func(eventType string, accountID uint, messageID uint, title, body string)
}

// notifyStreamEvent 是推给浏览器的通知事件。
//
// 与同步事件（type=new_mail）分开：那个的语义是「有变化，去重新拉」，
// 对基线导入、archive / junk 一律会发；这个是「值得打扰用户的一件事」，
// 已经过了 emit 那一侧的三道闸门（文件夹类型、非基线未读、跨文件夹去重）。
// 前端据此弹浏览器通知，拿 new_mail 弹的话首次导入几千封历史邮件就会刷屏。
type notifyStreamEvent struct {
	Type      string `json:"type"`  // 固定 "notify"
	Event     string `json:"event"` // mail_new / sync_failed / account_status / mail_rule
	AccountID uint   `json:"account_id"`
	// MessageID 仅单封新邮件非 0，前端据此点击直达那封信
	MessageID uint   `json:"message_id"`
	Title     string `json:"title"`
	Body      string `json:"body"`
}

// SetEmitHook 注册通知事件观察者。桌面形态在 OnStartup 时注入，nil 表示移除。
func (a *App) SetEmitHook(fn func(eventType string, accountID uint, messageID uint, title, body string)) {
	a.emitHookMu.Lock()
	a.emitHook = fn
	a.emitHookMu.Unlock()
}

// EnsureDefaultAdmin 数据库中无任何管理员时创建默认账户（桌面形态首次运行「开箱即用」）。
// 返回是否新建。
func (a *App) EnsureDefaultAdmin(username, password string) (bool, error) {
	return a.authSvc.EnsureAdmin(username, password)
}

// New 构建 App：初始化日志、开库、迁移、装配 handler。
func New(cfg *config.Config) (*App, error) {
	// 最先初始化统一日志：core zap 全局单例，标准库 log 经 RedirectStdLog 收编。
	logClose, err := logging.Setup(logging.Options{
		Dir:        cfg.LogDir(),
		MaxSizeMB:  cfg.Log.MaxSizeMB,
		MaxBackups: cfg.Log.MaxBackups,
		MaxAgeDays: cfg.Log.MaxAgeDays,
		Compress:   cfg.Log.Compress,
		Console:    cfg.Log.Console,
		Level:      cfg.Log.Level,
		Format:     cfg.Log.Format,
	})
	if err != nil {
		return nil, err
	}

	db, err := database.Open(cfg.DBPath())
	if err != nil {
		return nil, err
	}
	if err := database.Migrate(db); err != nil {
		return nil, err
	}
	// 回写队列表随 sync 包迁移（database 包不反向依赖 sync，避免测试期 import cycle）。
	if err := syncmod.MigrateWriteback(db); err != nil {
		return nil, err
	}
	authSvc := auth.NewService(auth.NewRepository(db), auth.Options{
		JWTSecret:      cfg.Auth.JWTSecret,
		AccessTTLMin:   cfg.Auth.AccessTokenTTL,
		RefreshTTLHour: cfg.Auth.RefreshTokenTTL,
	})
	enc, err := crypto.New(cfg.Crypto.EncryptionKey)
	if err != nil {
		return nil, err
	}
	accountSvc := account.NewService(account.NewRepository(db), enc)
	folderSvc := folder.NewService(folder.NewRepository(db))
	messageSvc := message.NewService(message.NewRepository(db), message.NewBodyRepository(db))
	syncSvc := syncmod.NewService(accountSvc, folderSvc, messageSvc)
	settingSvc := setting.NewService(setting.NewRepository(db))
	// 密文设置（目前只有 OAuth 的 client_secret）与账户密码共用同一把密钥。
	settingSvc.SetEncryptor(enc)
	syncSvc.SetSyncDepthProvider(func() int { return settingSvc.GetInt(setting.KeySyncDepth, 1000) })

	// OAuth 客户端凭据：每次用时现取，管理员在设置页改完即刻生效，不必重启。
	//
	// 两个来源的优先级是「数据库 > 配置文件/环境变量」：库里的值是管理员刚在界面上
	// 做的事，理应压过部署时写下的默认值。反过来的话，一旦 compose 里留了个
	// FLYMAIL_OAUTH_GOOGLE_CLIENT_ID，界面上怎么改都不生效，而界面还显示已保存。
	//
	// 回调基地址复用「对外访问地址」（app_base_url）：两者要的是同一个东西——
	// FlyMail 对外是什么地址。让用户在两个地方填两遍同一个值，只会制造它们不一致的机会。
	// oauth.redirect_base_url 仍然优先，供需要把回调指到别处的部署使用。
	accountSvc.SetOAuthSettingsProvider(func() account.OAuthSettings {
		redirect := cfg.OAuth.RedirectBaseURL
		if redirect == "" {
			redirect = settingSvc.GetString(setting.KeyAppBaseURL, "")
		}
		return account.OAuthSettings{
			GoogleClientID: settingSvc.GetString(
				setting.KeyOAuthGoogleClientID, cfg.OAuth.Google.ClientID),
			GoogleClientSecret: firstNonEmpty(
				settingSvc.GetSecret(setting.KeyOAuthGoogleClientSecret), cfg.OAuth.Google.ClientSecret),
			MicrosoftClientID:     cfg.OAuth.Microsoft.ClientID,
			MicrosoftClientSecret: cfg.OAuth.Microsoft.ClientSecret,
			MicrosoftTenant:       cfg.OAuth.Microsoft.Tenant,
			RedirectBaseURL:       redirect,
		}
	})
	sendSvc := send.NewService(accountSvc, folderSvc)
	draftSvc := draft.NewService(draft.NewRepository(db))

	a := &App{}

	// SSE Hub 提前建：emit 要借它把通知同样推给浏览器（见下）。
	hub := sse.NewHub()

	// 通知中心：站内记录 + 外发推送。emit 回调注入到各事件源（解耦）。
	// 外层再包两层观察者：桌面形态借 emitHook 弹系统原生通知，
	// 浏览器形态借 SSE 收到同一条事件后弹 Notification。
	notifySvc := notify.NewService(notify.NewRepository(db))
	// 通知里的「打开邮件」直达链接。
	//
	// 装在这里而不是 notify 包内部：拼一条链接要三样东西——对外访问地址（设置）、
	// 邮件所属的账户与文件夹（邮件服务）、以及前端的 URL 形状。notify 不该为了
	// 一行链接去依赖设置与邮件两个模块，而 app 本来就持有全部装配件。
	//
	// 对外访问地址留空时返回空串，通知就不带链接——服务端没有可靠办法猜出这个值：
	// 它看到的 Host 可能是反代内网名或容器名，监听地址可能是 0.0.0.0。
	// 没配 app_base_url 时退回 OAuth 那个同义配置，省得同一个地址填两遍。
	var badBase atomic.Value // 上次告警过的非法地址，避免每封邮件刷一行日志
	notifySvc.SetLinkBuilder(func(accountID, messageID uint) string {
		raw := settingSvc.GetString(setting.KeyAppBaseURL, cfg.OAuth.RedirectBaseURL)
		base, err := setting.NormalizeBaseURL(raw)
		if err != nil {
			// ⚠ 不能静默。设置页那条路径有校验，但回退值 cfg.OAuth.RedirectBaseURL
			// 直接来自配置文件、从不经过校验——有人在 yaml 里漏写 scheme，
			// 结果就是所有通知都不带链接而日志里一个字都没有，只能从 app 装配
			// 一路读到 NormalizeBaseURL 才查得出来。
			if prev, _ := badBase.Load().(string); prev != raw {
				badBase.Store(raw)
				logger.Warn("app: 对外访问地址不合法，通知将不带链接",
					zap.String("value", raw), zap.Error(err))
			}
			return ""
		}
		if base == "" {
			return ""
		}
		var folderID uint
		if messageID != 0 {
			if msg, err := messageSvc.GetByID(messageID); err == nil && msg != nil {
				folderID = msg.FolderID
			}
		}
		return notify.MailLink(base, accountID, folderID, messageID)
	})
	// 通知里的邮件内容。装在这里的理由和链接一样：取内容要依赖 message 模块，
	// 而 notify 不该为此依赖它。
	//
	// wantFull 由 Service 按各渠道配的档位算出——只有确实有渠道要全文时才去查
	// 正文表，绝大多数渠道停在摘要档，不必为它们多查一次。
	notifySvc.SetMailProvider(func(messageID uint, wantFull bool) *notify.MailData {
		msg, err := messageSvc.GetByID(messageID)
		if err != nil || msg == nil {
			return nil
		}
		from := msg.FromName
		if from == "" {
			from = msg.FromAddr
		}
		d := &notify.MailData{
			From:    from,
			Subject: msg.Subject,
			Date:    msg.Date,
			Snippet: msg.Snippet,
		}
		if wantFull {
			// BodyText 优先纯文本，没有就把 HTML 剥成文本——通知是纯文本/卡片形态，
			// 直接塞 HTML 源码过去只会是一堆标签。
			if text, known := messageSvc.BodyText(messageID); known {
				d.Body = text
			}
		}
		return d
	})

	baseEmit := notifySvc.EmitFunc()
	emit := func(eventType string, accountID uint, messageID uint, title, body string) {
		baseEmit(eventType, accountID, messageID, title, body)
		a.emitHookMu.RLock()
		hook := a.emitHook
		a.emitHookMu.RUnlock()
		if hook != nil {
			hook(eventType, accountID, messageID, title, body)
		}
		// 推给浏览器。这里而不是跟着 new_mail 走，是因为**闸门都在这一侧**：
		// new_mail 对基线导入、archive / junk 一样会发，拿它弹桌面通知，
		// 用户首次添加账户导入几千封历史邮件时就会被通知淹没。
		// 能走到这里的已经过了「文件夹类型 + 非基线未读 + 跨文件夹去重」三道闸门。
		payload, err := json.Marshal(notifyStreamEvent{
			Type:      "notify",
			Event:     eventType,
			AccountID: accountID,
			MessageID: messageID,
			Title:     title,
			Body:      body,
		})
		if err == nil {
			hub.Publish(payload)
		}
	}
	syncSvc.SetEmitter(emit)
	accountSvc.SetEmitter(emit)
	// 用户改动邮件状态（已读/星标/删除/移动）后广播 mail_state，
	// 让同时开着的其它界面重取计数与列表（详见 sync/mailstate.go）。
	syncSvc.SetPublisher(hub)

	// 后台同步管理器（IDLE + 轮询，新邮件经 Hub 推送）。
	manager := syncmod.NewManager(accountSvc, folderSvc, messageSvc, hub)
	manager.SetEmitter(emit)
	manager.SetPollIntervalProvider(func() int { return settingSvc.GetInt(setting.KeySyncPollInterval, 180) })
	manager.SetMaxConcurrentProvider(func() int { return settingSvc.GetInt(setting.KeySyncMaxConcurrent, 8) })
	manager.SetMaxIdleProvider(func() int { return settingSvc.GetInt(setting.KeySyncMaxIdleConns, 100) })
	manager.SetBodySyncProviders(
		func() string { return settingSvc.GetString(setting.KeyBodySyncMode, setting.DefaultBodySyncMode) },
		func() int { return settingSvc.GetInt(setting.KeyBodySyncRecentDays, 30) },
	)
	// 手动触发/详情/附件/回写经 Manager 投递到账户 runner，并与 Manager 共享同步进度存储。
	manager.EnableWriteback(db)
	syncSvc.SetManager(manager)

	// 规则引擎 + 黑名单：动作经 syncSvc 的批量操作（本地先改 + 回写队列），命中通知经 emit；
	// Manager 在每轮收件箱增量同步后调用它。
	ruleSvc := rule.NewService(rule.NewRepository(db), folderSvc, messageSvc)
	ruleSvc.SetActor(syncSvc)
	ruleSvc.SetEmitter(emit)
	ruleSvc.SetSelfAddresses(func() []string {
		list, err := accountSvc.List()
		if err != nil {
			return nil
		}
		out := make([]string, 0, len(list))
		for _, a := range list {
			out = append(out, a.Email)
		}
		return out
	})
	manager.SetRuleRunner(ruleSvc)

	// 阅读隐私：远程图片信任名单；登录限流（按 IP，落库）
	privacySvc := privacy.NewService(db)
	syncSvc.SetTrustedSenderCheck(privacySvc.IsTrusted)
	syncSvc.SetAttachmentTokenIssuer(authSvc.IssueAttachmentToken)
	loginLimiter := auth.NewLimiter(db)

	// 系统监控（只读聚合）
	monitoringSvc := monitoring.NewService(accountSvc, folderSvc, syncSvc, manager, time.Now(), appVersion, cfg.DBPath())
	// SSE 连接票据：受保护端点签发，EventSource 用 ?ticket= 连接，握手时核销。
	ticketStore := sse.NewTicketStore(sse.TicketTTL)
	eventsHandler := sse.NewHandler(hub, ticketStore.Consume)

	handler := server.New(server.Deps{
		Auth:             authSvc,
		Account:          accountSvc,
		Folder:           folderSvc,
		Message:          messageSvc,
		Sync:             syncSvc,
		Setting:          settingSvc,
		Send:             sendSvc,
		Draft:            draftSvc,
		Notify:           notifySvc,
		Monitoring:       monitoringSvc,
		Rule:             ruleSvc,
		Privacy:          privacySvc,
		LoginLimiter:     loginLimiter,
		TrustedProxies:   cfg.Server.TrustedProxies,
		Events:           eventsHandler,
		EventsTicket:     sse.NewTicketHandler(ticketStore),
		VerifyAttachment: authSvc.VerifyAttachmentAccess,
	})
	a.cfg = cfg
	a.srv = &http.Server{Handler: handler}
	a.manager = manager
	a.logClose = logClose
	a.db = db
	a.authSvc = authSvc
	return a, nil
}

// Handler 返回装配好的 HTTP 处理器（gin 引擎）。
// 桌面形态（Wails）将其作为 AssetServer.Handler 复用，无需 TCP 监听。
func (a *App) Handler() http.Handler { return a.srv.Handler }

// StartBackground 启动后台同步管理器（IDLE + 轮询，Shutdown 时取消）。
// 桌面形态无需 HTTP 监听，单独调用此方法即可；server 形态由 Start 内部调用。
func (a *App) StartBackground() {
	if a.manager != nil {
		ctx, cancel := context.WithCancel(context.Background())
		a.cancel = cancel
		a.manager.Start(ctx)
	}
}

// Start 在指定地址监听（addr 为空则用配置 host:port）。返回实际监听地址。
func (a *App) Start(addr string) (string, error) {
	if addr == "" {
		addr = fmt.Sprintf("%s:%d", a.cfg.Server.Host, a.cfg.Server.Port)
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return "", err
	}
	a.addr = ln.Addr().String()
	go func() { _ = a.srv.Serve(ln) }()

	a.StartBackground()
	return a.addr, nil
}

func (a *App) Shutdown() error {
	// 先停后台同步（取消 ctx 并等待 worker 退出），再关 HTTP。
	if a.cancel != nil {
		a.cancel()
	}
	if a.manager != nil {
		a.manager.Stop()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err := a.srv.Shutdown(ctx)
	// 释放 SQLite 连接：否则文件句柄保持到进程退出（桌面形态/测试临时目录清理都需要）。
	if a.db != nil {
		if sqlDB, dbErr := a.db.DB(); dbErr == nil {
			_ = sqlDB.Close()
		}
	}
	if a.logClose != nil {
		_ = a.logClose() // 关闭日志文件句柄
	}
	return err
}

// firstNonEmpty 返回第一个非空字符串，用于「数据库值优先、回落配置文件」。
func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
