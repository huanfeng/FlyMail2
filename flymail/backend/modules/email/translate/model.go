// Package translate 提供邮件的 AI 翻译：把主题与正文送去 OpenAI 兼容接口，
// 译文回填进原 HTML 结构，并按「邮件 + 目标语言」缓存。
//
// ── 为什么一定要缓存 ───────────────────────────────────────────────────────
//
// 翻译是按 token 计费的，而邮件是会被反复打开的：从列表点进去、切走再切回来、
// 第二天想起来再看一眼——每一次都重新翻一遍，账单会按用户的阅读习惯增长，
// 而不是按邮件数量增长。缓存让同一封信的同一种目标语言只花一次钱。
//
// 缓存不设过期：一封已经收到的邮件，其内容不会再变，译文也就不会过时。
// 想要重译（换了更好的模型）走 force，那是用户的显式动作。
package translate

import "time"

// Translation 是一封邮件在某个目标语言下的译文。
type Translation struct {
	ID uint `gorm:"primaryKey" json:"-"`
	// idx_tr_msg_lang 是唯一索引：同一封信同一种目标语言只存一份。
	// 唯一性不只是省空间——它让"重译"天然变成覆盖，不必先删后插。
	MessageID  uint   `gorm:"uniqueIndex:idx_tr_msg_lang,priority:1;not null" json:"message_id"`
	TargetLang string `gorm:"uniqueIndex:idx_tr_msg_lang,priority:2;not null" json:"target_lang"`
	// SourceLang 是翻译时识别出的源语言，识别不出时为空串。
	// 存下来是为了让界面能说清"从哪门语言翻过来的"，也便于日后排查错翻。
	SourceLang string `json:"source_lang"`

	Subject string `json:"subject"`
	// TextBody / HTMLBody 与 message_bodies 一一对应：原文是哪种形态，
	// 译文就落在哪个字段上。HTMLBody 存的是**未净化**的回填结果，
	// 出站前按当次请求的远程图策略净化（见 handler），否则一旦把
	// "远程图已换成占位符"的那一版缓存下来，用户此后再也点不出原图。
	TextBody string `json:"text_body"`
	HTMLBody string `json:"html_body"`

	// Partial 表示正文过长、只翻译了前面一部分（见 maxTotalRunes）。
	// 必须如实告诉界面：否则用户会以为后半篇"翻译过了只是没变"。
	Partial bool `json:"partial"`

	// Model 是产出这份译文的模型名，换模型重译时用来解释"这份是旧模型翻的"。
	Model string `json:"model"`
	// Provider 是产出这份译文的 AI 配置名。配了多条、自动切换过时，
	// 用户得知道这份是哪条线路翻的，才能判断要不要换一条重译。
	Provider string `json:"provider"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (Translation) TableName() string { return "message_translations" }
