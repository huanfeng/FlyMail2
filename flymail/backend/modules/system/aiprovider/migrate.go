package aiprovider

import "strings"

// 旧版（单配置）存在 settings 表里的三个键。只在迁移时读取。
const (
	legacyKeyBaseURL = "ai_base_url"
	legacyKeyAPIKey  = "ai_api_key"
	legacyKeyModel   = "ai_model"
)

// LegacyStore 是迁移所需的 settings 表读写能力（*setting.Repository 满足之）。
//
// 这里读的是**原始值**：密钥在 settings 里本来就是同一把密钥加密的密文，
// 原样搬进新表即可，不必解开再加密——也就不会因为解密失败而把密钥弄丢。
type LegacyStore interface {
	Get(key string) (string, bool, error)
	Delete(key string) error
}

// MigrateLegacy 把旧版单配置搬成第一条配置，然后删掉旧键。启动时调用，可重复执行。
//
//   - 新表已有数据：只清旧键，不再搬（用户已经在新界面里配过了，旧值不该复活）。
//   - 旧键没配全（地址或模型为空）：旧版里这等于「没开翻译」，只清旧键。
//
// 先插入后删旧键：中途失败时最坏是下次启动再清一遍旧键，不会丢配置。
func MigrateLegacy(repo *Repository, store LegacyStore) (migrated bool, err error) {
	n, err := repo.Count()
	if err != nil {
		return false, err
	}
	baseURL, _, err := store.Get(legacyKeyBaseURL)
	if err != nil {
		return false, err
	}
	model, _, err := store.Get(legacyKeyModel)
	if err != nil {
		return false, err
	}
	key, _, err := store.Get(legacyKeyAPIKey)
	if err != nil {
		return false, err
	}
	baseURL, model = strings.TrimSpace(baseURL), strings.TrimSpace(model)

	if n == 0 && baseURL != "" && model != "" {
		p := Provider{
			Name:    hostOf(baseURL),
			BaseURL: baseURL, // 旧版落库前已经 ai.Endpoint 归一过
			APIKey:  key,
			Model:   model,
			Enabled: true,
		}
		if err := repo.Create(&p); err != nil {
			return false, err
		}
		migrated = true
	}
	for _, k := range []string{legacyKeyBaseURL, legacyKeyAPIKey, legacyKeyModel} {
		if err := store.Delete(k); err != nil {
			return migrated, err
		}
	}
	return migrated, nil
}
