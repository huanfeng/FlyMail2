package models

import (
	"encoding/json"
	"time"

	"gorm.io/gorm"
)

type SystemSetting struct {
	gorm.Model
	Key   string `gorm:"uniqueIndex" json:"key"`
	Value string `json:"value"`
}

type Proxy struct {
	gorm.Model
	Name     string `json:"name"`
	Type     string `json:"type"` // socks5, http
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Username string `json:"username"`
	Password string `json:"password"` // Encrypted
}

type Account struct {
	gorm.Model
	Email             string     `gorm:"uniqueIndex" json:"email"` // address for display
	DisplayName       string     `json:"display_name"`
	Login             string     `json:"login"`     // username for IMAP login
	AuthType          string     `json:"auth_type"` // password, oauth2
	Password          string     `json:"-"`         // Encrypted
	OAuthToken        string     `json:"-"`         // Encrypted JSON
	PasswordExpiresAt *time.Time `json:"password_expires_at"`
	Enabled           bool       `gorm:"default:true" json:"enabled"`

	ProxyID *uint  `json:"proxy_id"`
	Proxy   *Proxy `json:"proxy,omitempty"`

	Provider   string `json:"provider"`
	IMAPServer string `json:"imap_server"`
	IMAPPort   int    `json:"imap_port"`
	UseSSL     bool   `json:"use_ssl"`
	SSLMode    string `json:"ssl_mode"` // ssl, starttls, none

	UseIDLE           bool   `json:"use_idle"`
	PollIntervalDay   int    `json:"poll_interval_day"`
	PollIntervalNight int    `json:"poll_interval_night"`
	Timezone          string `json:"timezone"`

	Status     string    `json:"status"` // Active, AuthFailed, NetworkError
	LastSyncAt time.Time `json:"last_sync_at"`
	LastUID    uint      `json:"last_uid"` // fallback for INBOX when mailbox record missing
}

type ForwardLog struct {
	gorm.Model
	AccountID   uint      `json:"account_id"`
	MessageID   string    `json:"message_id"`
	ChannelID   *uint     `json:"channel_id"`
	ChannelName string    `json:"channel_name"`
	From        string    `json:"from"`
	Subject     string    `json:"subject"`
	ReceivedAt  time.Time `json:"received_at"`
	ForwardedAt time.Time `json:"forwarded_at"`
	Priority    int       `json:"priority"` // Event priority at the time of logging
	Status      string    `json:"status"`   // "success", "failed", "received"
	Action      string    `json:"action"`   // "receive", "push"
	Channel     string    `json:"channel"`
	Request     string    `json:"request"`  // Serialized request payload for push
	Response    string    `json:"response"` // Serialized response payload for push
	Error       string    `json:"error"`
}

type Email struct {
	ID          string `gorm:"primaryKey;size:21" json:"id"`
	CreatedAt   time.Time
	UpdatedAt   time.Time
	DeletedAt   gorm.DeletedAt `gorm:"index"`
	AccountID   uint           `json:"account_id"`
	MessageID   string         `gorm:"index" json:"message_id"`
	MailboxID   uint           `json:"mailbox_id"`
	UID         uint           `gorm:"index:idx_account_uid" json:"uid"` // IMAP UID
	SeqNum      uint           `json:"seq_num"`                          // SeqNum in current mailbox
	Mailbox     string         `gorm:"index" json:"mailbox"`             // decoded mailbox name
	MailboxPath string         `json:"mailbox_path"`                     // raw path used for select
	MailType    string         `json:"mail_type"`                        // categorized type: primary, spam, promotion, etc.
	From        string         `json:"from"`
	To          string         `json:"to"`
	Subject     string         `json:"subject"`
	TextBody    string         `json:"text_body"` // Preview
	HTMLBody    string         `json:"html_body"` // Full content
	ReceivedAt  time.Time      `json:"received_at"`
	IsRead      bool           `json:"is_read"`
}

type Channel struct {
	gorm.Model
	Name            string `json:"name"`
	Type            string `json:"type"`   // telegram, discord, etc.
	Config          string `json:"config"` // JSON string
	Status          string `json:"status"` // enabled, disabled
	MinPriority     int    `json:"min_priority"`
	SubscribedTypes string `json:"subscribed_types"`      // JSON array of strings: ["primary", "bill"]
	TemplateID      *uint  `json:"template_id"`           // Optional reference to NotificationTemplate
	Template        string `json:"template"`              // Legacy inline template (fallback)
	QuietMode       string `json:"quiet_mode"`            // global, override, off
	QuietEnable     bool   `json:"quiet_enable"`          // used when QuietMode=override
	QuietStart      string `json:"quiet_start,omitempty"` // HH:MM, used when QuietMode=override
	QuietEnd        string `json:"quiet_end,omitempty"`   // HH:MM, used when QuietMode=override
}

// Mailbox keeps synced folder info and last UID for incremental pull
type Mailbox struct {
	gorm.Model
	AccountID   uint   `gorm:"index:idx_account_mailbox" json:"account_id"`
	Name        string `json:"name"`                                  // decoded/pretty name
	Path        string `gorm:"index:idx_account_mailbox" json:"path"` // raw IMAP name (UTF-7)
	Delimiter   string `json:"delimiter"`
	Attributes  string `json:"attributes"`   // comma separated flags
	LastUID     uint   `json:"last_uid"`     // last pulled UID in this mailbox
	WatchStatus string `json:"watch_status"` // verified, missing
	WatchMode   string `json:"watch_mode"`   // idle, poll, none (default)
	Type        string `json:"type"`         // primary, bill, notification, etc.
}

type MailType struct {
	gorm.Model
	Key        string `gorm:"uniqueIndex" json:"key"` // e.g., "bill"
	Name       string `json:"name"`                   // e.g., "Bill / Receipt"
	Priority   int    `json:"priority"`               // 0=Low, 10=Normal, 20=High
	IsSystem   bool   `json:"is_system"`              // If true, cannot be deleted
	ChannelIDs string `json:"channel_ids"`            // JSON array: [1, 2] — notify to which channels
	Action     string `json:"action"`                 // "notify" / "silent" / "ignore"
}

type FolderRule struct {
	gorm.Model
	Name       string `json:"name"`
	Pattern    string `json:"pattern"`     // Regex to match folder name/path
	TargetType string `json:"target_type"` // Key from MailType
	Order      int    `json:"order"`       // Execution order (lower first)
}

type NotificationTemplate struct {
	gorm.Model
	Name        string `json:"name"`
	Content     string `json:"content"`      // Go template string
	ChannelType string `json:"channel_type"` // "telegram", "discord", "all"
	IsDefault   bool   `json:"is_default"`
	Description string `json:"description"`
}

type OneTimeToken struct {
	gorm.Model
	Token     string    `gorm:"uniqueIndex;size:32" json:"token"`
	EmailID   string    `gorm:"size:21" json:"email_id"`
	ExpiresAt time.Time `json:"expires_at"`
	Used      bool      `json:"used"`
}

type User struct {
	gorm.Model
	Username     string     `gorm:"uniqueIndex" json:"username"`
	Email        string     `gorm:"uniqueIndex" json:"email"`
	PasswordHash string     `json:"-"`
	LastLoginAt  *time.Time `json:"last_login_at"`
}

type RefreshToken struct {
	gorm.Model
	UserID    uint      `gorm:"index" json:"user_id"`
	TokenHash string    `gorm:"uniqueIndex" json:"-"`
	ExpiresAt time.Time `json:"expires_at"`
	Revoked   bool      `json:"revoked"`
	User      User      `gorm:"constraint:OnDelete:CASCADE"`
}

func AutoMigrate(db *gorm.DB) error {
	err := db.AutoMigrate(
		&SystemSetting{},
		&Proxy{},
		&Account{},
		&ForwardLog{},
		&Mailbox{},
		&Email{},
		&Channel{},
		&OneTimeToken{},
		&User{},
		&RefreshToken{},
		&MailType{},
		&FolderRule{},
		&NotificationTemplate{},
	)
	if err != nil {
		return err
	}
	// Seed initial data
	return seedInitialData(db)
}

func seedInitialData(db *gorm.DB) error {
	// Seed MailTypes
	types := []MailType{
		{Key: "primary", Name: "Primary", Priority: 10, IsSystem: true, Action: "notify", ChannelIDs: "[]"},
		{Key: "bill", Name: "Bill", Priority: 10, IsSystem: true, Action: "notify", ChannelIDs: "[]"},
		{Key: "notification", Name: "Notification", Priority: 10, IsSystem: true, Action: "notify", ChannelIDs: "[]"},
		{Key: "promotion", Name: "Promotion", Priority: 0, IsSystem: true, Action: "silent", ChannelIDs: "[]"},
		{Key: "social", Name: "Social", Priority: 0, IsSystem: true, Action: "silent", ChannelIDs: "[]"},
		{Key: "spam", Name: "Spam", Priority: 0, IsSystem: true, Action: "ignore", ChannelIDs: "[]"},
		{Key: "trash", Name: "Trash", Priority: 0, IsSystem: true, Action: "ignore", ChannelIDs: "[]"},
		{Key: "draft", Name: "Draft", Priority: 0, IsSystem: true, Action: "ignore", ChannelIDs: "[]"},
		{Key: "sent", Name: "Sent", Priority: 0, IsSystem: true, Action: "ignore", ChannelIDs: "[]"},
		{Key: "important", Name: "Important", Priority: 20, IsSystem: true, Action: "notify", ChannelIDs: "[]"},
		{Key: "unknown", Name: "Unknown", Priority: 10, IsSystem: true, Action: "notify", ChannelIDs: "[]"},
	}
	for _, t := range types {
		var existing MailType
		if db.Where("key = ?", t.Key).First(&existing).Error != nil {
			// Not found — create
			db.Create(&t)
		} else {
			// Existing — only update Name/Priority/IsSystem, preserve user's Action/ChannelIDs
			db.Model(&existing).Updates(map[string]any{
				"name":      t.Name,
				"priority":  t.Priority,
				"is_system": t.IsSystem,
			})
			// Backfill Action if empty (migration from old schema)
			if existing.Action == "" {
				db.Model(&existing).Update("action", t.Action)
			}
			if existing.ChannelIDs == "" {
				db.Model(&existing).Update("channel_ids", "[]")
			}
		}
	}

	// Seed FolderRules (Regex for Folder Names)
	rules := []FolderRule{
		{Name: "Spam", Pattern: "(?i)(\\\\junk|\\bspam\\b|\\bjunk\\b|垃圾)", TargetType: "spam", Order: 10},
		{Name: "Trash", Pattern: "(?i)(\\\\trash|\\btrash\\b|\\bdeleted\\b|回收站)", TargetType: "trash", Order: 10},
		{Name: "Drafts", Pattern: "(?i)(\\\\drafts|\\bdraft)", TargetType: "draft", Order: 10},
		{Name: "Sent", Pattern: "(?i)(\\\\sent|\\bsent)", TargetType: "sent", Order: 10},
		{Name: "Promotion", Pattern: "(?i)(promo|广告|营销)", TargetType: "promotion", Order: 20},
		{Name: "Social", Pattern: "(?i)(social)", TargetType: "social", Order: 20},
		{Name: "Important", Pattern: "(?i)(important|\\\\flagged|starred)", TargetType: "important", Order: 20},
		{Name: "Inbox", Pattern: "(?i)(inbox|收件箱)", TargetType: "primary", Order: 99},
	}
	for _, r := range rules {
		db.FirstOrCreate(&FolderRule{}, FolderRule{Name: r.Name})
		// Ensure defaults are set if created
		db.Model(&FolderRule{}).Where("name = ?", r.Name).Updates(r)
	}

	// Seed default notification templates
	defaultTemplates := []NotificationTemplate{
		{
			Name:        "Telegram Default",
			ChannelType: "telegram",
			IsDefault:   true,
			Description: "Default template for Telegram (HTML format)",
			Content: `📧 <b>{{.Subject}}</b>
From: {{.From}}
To: {{.AccountEmail}}
Folder: {{.Mailbox}} ({{.MailType}})
Time: {{.ReceivedAt}}
{{if .IsVerificationCode}}
🔑 Code: <code>{{.VerificationCode}}</code>{{end}}
{{if .BodyPreview}}
{{.BodyPreview}}{{end}}{{if .ViewLink}}

<a href="{{.ViewLink}}">View Email</a>{{end}}`,
		},
		{
			Name:        "Discord Default",
			ChannelType: "discord",
			IsDefault:   true,
			Description: "Default template for Discord (Markdown format)",
			Content: `**{{.Subject}}**
**From:** {{.From}}
**To:** {{.AccountEmail}}
**Folder:** {{.Mailbox}} ({{.MailType}})
**Time:** {{.ReceivedAt}}
{{if .IsVerificationCode}}
🔑 **Code:** ` + "`{{.VerificationCode}}`" + `{{end}}
{{if .BodyPreview}}
> {{.BodyPreview}}{{end}}{{if .ViewLink}}
[View Email]({{.ViewLink}}){{end}}`,
		},
		{
			Name:        "Feishu Default",
			ChannelType: "feishu",
			IsDefault:   true,
			Description: "Default template for Feishu (Lark Markdown format)",
			Content: `**{{.Subject}}**
**From:** {{.From}}
**To:** {{.AccountEmail}}
**Folder:** {{.Mailbox}} ({{.MailType}})
**Time:** {{.ReceivedAt}}
{{if .IsVerificationCode}}
🔑 **Code:** ` + "`{{.VerificationCode}}`" + `{{end}}
{{if .BodyPreview}}
> {{.BodyPreview}}{{end}}{{if .ViewLink}}
[View Email]({{.ViewLink}}){{end}}`,
		},
		{
			Name:        "General Default",
			ChannelType: "all",
			IsDefault:   true,
			Description: "General purpose template (plain text)",
			Content: `New Email: {{.Subject}}
From: {{.From}}
To: {{.AccountEmail}}
Folder: {{.Mailbox}} ({{.MailType}})
Time: {{.ReceivedAt}}
{{if .IsVerificationCode}}
🔑 Code: {{.VerificationCode}}{{end}}
{{if .BodyPreview}}
{{.BodyPreview}}{{end}}`,
		},
	}
	for _, t := range defaultTemplates {
		db.FirstOrCreate(&NotificationTemplate{}, NotificationTemplate{Name: t.Name})
		db.Model(&NotificationTemplate{}).Where("name = ?", t.Name).Updates(t)
	}

	// One-time migration: convert Channel.SubscribedTypes → MailType.ChannelIDs
	migrateSubscribedTypes(db)

	return nil
}

// migrateSubscribedTypes converts legacy Channel.SubscribedTypes into MailType.ChannelIDs.
// This runs idempotently — it only acts when ChannelIDs are all empty and SubscribedTypes exist.
func migrateSubscribedTypes(db *gorm.DB) {
	// Check if any MailType already has non-empty ChannelIDs (migration already done)
	var count int64
	db.Model(&MailType{}).Where("channel_ids != '' AND channel_ids != '[]'").Count(&count)
	if count > 0 {
		return // Already migrated
	}

	// Load all channels that have subscribed types
	var channels []Channel
	db.Where("subscribed_types != '' AND subscribed_types != '[]'").Find(&channels)
	if len(channels) == 0 {
		return // Nothing to migrate
	}

	// Build mapping: mail_type_key → []channel_id
	typeToChannels := make(map[string][]uint)
	for _, ch := range channels {
		var subs []string
		if err := json.Unmarshal([]byte(ch.SubscribedTypes), &subs); err != nil {
			continue
		}
		for _, s := range subs {
			typeToChannels[s] = append(typeToChannels[s], ch.ID)
		}
	}

	// Apply to MailTypes
	for key, ids := range typeToChannels {
		idsJSON, _ := json.Marshal(ids)
		db.Model(&MailType{}).Where("key = ?", key).Update("channel_ids", string(idsJSON))
	}
}
