// Package aiprovider 管理 AI 接口配置（OpenAI 兼容）：可以配多条，按顺序组成
// 「使用列表」，翻译时依次尝试，前一个用不了就换下一个。
//
// ── 为什么是一张表而不是 settings 里的一段 JSON ──────────────────────────────
//
// 每条配置的密钥要单独加密、单独「清除」，而 settings 的密文机制是按键整值加密的：
// 塞进一段 JSON 里，改一条的模型名就得把所有密钥解开再加密一遍，
// 界面上「这条配没配密钥」也得靠解析 JSON 才答得出来。
// 顺序则直接沿用账户那套 sort_order + 整表重排（见 Repository.Reorder）。
package aiprovider

import (
	"time"

	"flymail/internal/ai"
)

// Provider 是一条 AI 接口配置。
type Provider struct {
	ID   uint   `gorm:"primaryKey" json:"id"`
	Name string `gorm:"not null" json:"name"`
	// BaseURL 落库前已经 ai.Endpoint 归一，是可以直接发请求的完整地址。
	BaseURL string `gorm:"not null" json:"base_url"`
	// ⚠ APIKey 是密文，永远不出网：对外只给 key_set。
	APIKey string `json:"-"`
	Model  string `gorm:"not null" json:"model"`
	// Enabled 为假的配置留着但不参与切换——临时停用一个欠费的服务商，
	// 不必把地址和密钥删了再重填。
	Enabled bool `gorm:"not null" json:"enabled"`
	// SortOrder 越小越先用。新建的排到末尾（见 Repository.Create）。
	SortOrder int `gorm:"not null;default:0" json:"-"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (Provider) TableName() string { return "ai_providers" }

// View 是配置的对外表示：密钥只报「配没配」，外加运行时健康状态。
type View struct {
	Provider
	KeySet bool     `json:"key_set"`
	Status ai.State `json:"status"`
	// Cooling 由服务端按当前时刻算好：前端时钟可能与服务端不一致。
	Cooling bool `json:"cooling"`
}
