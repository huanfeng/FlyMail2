package dispatcher

import (
	"encoding/json"
	"fmt"
	"mail2im/internal/core"
	"mail2im/internal/dispatcher/channels"
	"mail2im/internal/models"
	"strings"
	"sync"
	"time"

	"flymail-core/logger"

	"go.uber.org/zap"
)

type quietConfig struct {
	Enabled bool
	Start   string
	End     string
}

type channelQuiet struct {
	Mode    string
	Enabled bool
	Start   string
	End     string
}

type channelEntry struct {
	id              *uint
	name            string
	channelType     string // "telegram", "discord", "console"
	sender          core.NotificationChannel
	quiet           channelQuiet
	subscribedTypes []string
	templateContent string // resolved template content for this channel
}

type Dispatcher struct {
	channels    []channelEntry
	strategy    *StrategyEngine
	globalQuiet quietConfig
	loc         *time.Location
	mu          sync.RWMutex
}

var Instance *Dispatcher

func InitDispatcher() {
	config := loadStrategyConfig()

	Instance = &Dispatcher{
		channels: make([]channelEntry, 0),
		strategy: NewStrategyEngine(config),
		loc:      core.GetSystemLocation(),
		globalQuiet: quietConfig{
			Enabled: config.QuietEnabled,
			Start:   config.QuietHoursStart,
			End:     config.QuietHoursEnd,
		},
	}

	// Register event handlers
	core.Bus.Subscribe(core.EventEmailReceived, Instance.handleEvent)
	core.Bus.Subscribe(core.EventAuthFailed, Instance.handleEvent)
	core.Bus.Subscribe(core.EventSystemError, Instance.handleEvent)

	Instance.ReloadChannels()
	logger.Info("Notification Dispatcher initialized")
}

func (d *Dispatcher) Register(c core.NotificationChannel) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.channels = append(d.channels, channelEntry{
		id:     nil,
		name:   c.Name(),
		sender: c,
		quiet:  channelQuiet{Mode: "global"},
	})
	logger.Info("Registered notification channel", zap.String("channel", c.Name()))
}

func (d *Dispatcher) ReloadChannels() {
	d.mu.Lock()
	defer d.mu.Unlock()

	cfg := loadStrategyConfig()
	d.loc = core.GetSystemLocation()
	d.globalQuiet = quietConfig{Enabled: cfg.QuietEnabled, Start: cfg.QuietHoursStart, End: cfg.QuietHoursEnd}

	d.channels = make([]channelEntry, 0)

	// Add Console Channel (Always enabled)
	d.channels = append(d.channels, channelEntry{
		id:          nil,
		name:        "Console",
		channelType: "console",
		sender:      channels.NewConsoleChannel(core.PriorityLow),
		quiet:       channelQuiet{Mode: "global"},
	})

	// Load channels from DB
	var dbChannels []models.Channel
	if err := core.DB.Find(&dbChannels, "status = ?", "enabled").Error; err != nil {
		logger.Error("Failed to load channels from DB", zap.Error(err))
		return
	}

	for _, dbCh := range dbChannels {
		var subs []string
		if dbCh.SubscribedTypes != "" {
			_ = json.Unmarshal([]byte(dbCh.SubscribedTypes), &subs)
		}

		// Resolve template content
		tmplContent := resolveTemplate(dbCh)

		quietCfg := channelQuiet{
			Mode:    normalizeQuietMode(dbCh.QuietMode),
			Enabled: dbCh.QuietEnable,
			Start:   dbCh.QuietStart,
			End:     dbCh.QuietEnd,
		}

		switch dbCh.Type {
		case "telegram":
			var config struct {
				Token  string `json:"token"`
				ChatID string `json:"chat_id"`
			}
			if err := json.Unmarshal([]byte(dbCh.Config), &config); err == nil {
				d.channels = append(d.channels, channelEntry{
					id:              &dbCh.ID,
					name:            dbCh.Name,
					channelType:     "telegram",
					sender:          channels.NewTelegramChannelWithConfig(config.Token, config.ChatID, core.EventPriority(dbCh.MinPriority), tmplContent),
					quiet:           quietCfg,
					subscribedTypes: subs,
					templateContent: tmplContent,
				})
				logger.Info("Loaded Telegram channel", zap.String("channel", dbCh.Name))
			} else {
				logger.Error("Failed to parse config for channel", zap.String("channel", dbCh.Name), zap.Error(err))
			}

		case "discord":
			var config struct {
				WebhookURL string `json:"webhook_url"`
			}
			if err := json.Unmarshal([]byte(dbCh.Config), &config); err == nil {
				d.channels = append(d.channels, channelEntry{
					id:              &dbCh.ID,
					name:            dbCh.Name,
					channelType:     "discord",
					sender:          channels.NewDiscordChannel(config.WebhookURL, core.EventPriority(dbCh.MinPriority), tmplContent),
					quiet:           quietCfg,
					subscribedTypes: subs,
					templateContent: tmplContent,
				})
				logger.Info("Loaded Discord channel", zap.String("channel", dbCh.Name))
			} else {
				logger.Error("Failed to parse config for channel", zap.String("channel", dbCh.Name), zap.Error(err))
			}

		case "feishu":
			var config struct {
				WebhookURL string `json:"webhook_url"`
				SignSecret string `json:"sign_secret"`
			}
			if err := json.Unmarshal([]byte(dbCh.Config), &config); err == nil {
				d.channels = append(d.channels, channelEntry{
					id:              &dbCh.ID,
					name:            dbCh.Name,
					channelType:     "feishu",
					sender:          channels.NewFeishuChannel(config.WebhookURL, config.SignSecret, core.EventPriority(dbCh.MinPriority), tmplContent),
					quiet:           quietCfg,
					subscribedTypes: subs,
					templateContent: tmplContent,
				})
				logger.Info("Loaded Feishu channel", zap.String("channel", dbCh.Name))
			} else {
				logger.Error("Failed to parse config for channel", zap.String("channel", dbCh.Name), zap.Error(err))
			}
		}
	}
}

func resolveTemplate(dbCh models.Channel) string {
	if dbCh.TemplateID != nil && *dbCh.TemplateID > 0 {
		var t models.NotificationTemplate
		if err := core.DB.First(&t, *dbCh.TemplateID).Error; err == nil {
			return t.Content
		}
	}
	return dbCh.Template
}

func (d *Dispatcher) handleEvent(event core.Event) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	accountID, subject, from, messageID, receivedAt := extractEmailMeta(event)
	mailType := extractMailType(event)

	// Check global strategy (block patterns, etc.)
	if !d.strategy.ShouldSend(event) {
		logger.Info("Event blocked by strategy", zap.String("type", string(event.Type)))
		return
	}

	// MailType-based routing for email events
	var targetChannelIDs map[uint]bool
	if event.Type == core.EventEmailReceived && mailType != "" {
		var mt models.MailType
		if err := core.DB.Where("key = ?", mailType).First(&mt).Error; err == nil {
			action := mt.Action
			if action == "" {
				action = "notify" // backward compat
			}
			switch action {
			case "ignore", "silent":
				logger.Info("Event suppressed by MailType", zap.String("action", action), zap.String("mailType", mailType))
				return
			}
			// Parse ChannelIDs if set
			if mt.ChannelIDs != "" && mt.ChannelIDs != "[]" {
				var ids []uint
				if err := json.Unmarshal([]byte(mt.ChannelIDs), &ids); err == nil && len(ids) > 0 {
					targetChannelIDs = make(map[uint]bool, len(ids))
					for _, id := range ids {
						targetChannelIDs[id] = true
					}
				}
			}
		}
	}

	// Build template data once for all channels
	tmplData := BuildTemplateData(event)

	for _, ch := range d.channels {
		// 1. If MailType specifies target channels, only send to those
		if targetChannelIDs != nil && ch.id != nil {
			if !targetChannelIDs[*ch.id] {
				continue
			}
		}

		// 2. Check Priority
		if event.Priority < ch.sender.MinPriority() {
			continue
		}

		// 3. Check Mail Type subscription (legacy, for backward compat)
		if targetChannelIDs == nil && len(ch.subscribedTypes) > 0 && mailType != "" {
			allowed := false
			for _, t := range ch.subscribedTypes {
				if strings.EqualFold(t, mailType) || strings.EqualFold(t, "all") {
					allowed = true
					break
				}
			}
			if !allowed {
				continue
			}
		}

		// 4. Check Quiet Mode
		if d.isQuietForChannel(ch.quiet) {
			logger.Info("Channel suppressed by quiet hours", zap.String("channel", ch.name))
			continue
		}

		go func(entry channelEntry, e core.Event, data TemplateData) {
			delays := []time.Duration{5 * time.Second, 15 * time.Second}
			maxAttempts := 1 + len(delays) // 3 次

			var reqDetail, respDetail string
			var err error

			rendered := renderForChannel(entry, data)

			for attempt := 0; attempt < maxAttempts; attempt++ {
				if attempt > 0 {
					time.Sleep(delays[attempt-1])
				}

				reqDetail, respDetail = "", ""
				if rendered != "" {
					if dtc, ok := entry.sender.(core.DetailedTemplateChannel); ok {
						reqDetail, respDetail, err = dtc.SendRenderedWithDetails(rendered, e)
					} else if tc, ok := entry.sender.(core.TemplateAwareChannel); ok {
						err = tc.SendRendered(rendered, e)
					} else if ds, ok := entry.sender.(core.DetailedSender); ok {
						reqDetail, respDetail, err = ds.SendWithDetails(e)
					} else {
						err = entry.sender.Send(e)
					}
				} else {
					if ds, ok := entry.sender.(core.DetailedSender); ok {
						reqDetail, respDetail, err = ds.SendWithDetails(e)
					} else {
						err = entry.sender.Send(e)
					}
				}

				if err == nil {
					break
				}
				if attempt < maxAttempts-1 {
					logger.Warn("Send failed, retrying",
						zap.String("channel", entry.name),
						zap.Int("attempt", attempt+1),
						zap.Error(err),
					)
				}
			}

			channelName := entry.name
			if channelName == "" {
				channelName = entry.sender.Name()
			}

			if err != nil {
				logger.Error("Failed to send to channel after retries", zap.String("channel", channelName), zap.Error(err))
				if accountID > 0 {
					core.RecordForwardLog(accountID, "push", "failed", entry.id, channelName, messageID, subject, from, receivedAt, int(e.Priority), reqDetail, respDetail, err.Error())
				}
				return
			}
			if accountID > 0 {
				core.RecordForwardLog(accountID, "push", "success", entry.id, channelName, messageID, subject, from, receivedAt, int(e.Priority), reqDetail, respDetail, "")
			}
		}(ch, event, tmplData)
	}
}

// renderForChannel renders the appropriate template for a channel.
// Returns empty string if no template is configured (legacy fallback).
func renderForChannel(entry channelEntry, data TemplateData) string {
	if entry.templateContent == "" {
		return ""
	}

	// Apply channel-specific body content truncation
	limit := BodyLimitForChannel(entry.channelType)
	data.BodyContent = truncate(data.BodyContent, limit)

	switch entry.channelType {
	case "telegram":
		// Telegram uses HTML — escape data fields before rendering
		return RenderTemplateHTML(entry.templateContent, data, DefaultFallbackHTML(data))
	case "feishu":
		// Feishu uses Lark Markdown — render as-is
		return RenderTemplate(entry.templateContent, data, DefaultFallbackMessage(data))
	case "discord":
		// Discord uses Markdown — render as-is
		return RenderTemplate(entry.templateContent, data, DefaultFallbackMessage(data))
	default:
		return RenderTemplate(entry.templateContent, data, DefaultFallbackMessage(data))
	}
}

func extractMailType(event core.Event) string {
	if payload, ok := event.Payload.(map[string]any); ok {
		if t, ok := payload["mail_type"].(string); ok {
			return t
		}
	}
	return ""
}

func extractEmailMeta(event core.Event) (uint, string, string, string, time.Time) {
	payload, ok := event.Payload.(map[string]any)
	if !ok {
		return 0, "", "", "", time.Now()
	}

	var accountID uint
	if v, ok := payload["account_id"]; ok {
		switch val := v.(type) {
		case float64:
			accountID = uint(val)
		case int:
			accountID = uint(val)
		case uint:
			accountID = val
		}
	}

	subject, _ := payload["subject"].(string)
	from, _ := payload["from"].(string)
	messageID, _ := payload["message_id"].(string)
	if messageID == "" {
		if v, ok := payload["uid"]; ok {
			messageID = fmt.Sprintf("uid:%v", v)
		}
	}

	receivedAt := time.Now()
	if v, ok := payload["received_at"].(time.Time); ok {
		receivedAt = v
	}

	return accountID, subject, from, messageID, receivedAt
}

func (d *Dispatcher) isQuietForChannel(q channelQuiet) bool {
	quiet := d.globalQuiet
	switch normalizeQuietMode(q.Mode) {
	case "override":
		quiet.Enabled = q.Enabled
		quiet.Start = q.Start
		quiet.End = q.End
	case "off":
		quiet.Enabled = false
	}

	if !quiet.Enabled {
		return false
	}

	now := core.NowInSystemTZ()
	window := core.TimeWindow{Enabled: quiet.Enabled, Start: quiet.Start, End: quiet.End}
	return core.IsInWindow(now, window, d.loc)
}

func normalizeQuietMode(mode string) string {
	switch strings.ToLower(mode) {
	case "override":
		return "override"
	case "off":
		return "off"
	default:
		return "global"
	}
}

func loadStrategyConfig() StrategyConfig {
	return StrategyConfig{
		QuietEnabled:    core.ParseBoolSetting("quiet_enabled", false),
		QuietHoursStart: core.GetSystemSettingWithDefault("quiet_start", ""),
		QuietHoursEnd:   core.GetSystemSettingWithDefault("quiet_end", ""),
		BlockPatterns:   []string{},
	}
}
