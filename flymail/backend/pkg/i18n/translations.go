package i18n

// loadDefaultTranslations 加载默认翻译
func (m *Manager) loadDefaultTranslations() {
	// 加载英文翻译
	m.RegisterTranslations(LanguageEnUS, enUSTranslations)

	// 加载中文翻译
	m.RegisterTranslations(LanguageZhCN, zhCNTranslations)
}
