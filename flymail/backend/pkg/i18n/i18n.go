package i18n

import (
	"fmt"
	"strings"
	"sync"
)

// Language 语言代码类型
type Language string

const (
	// 支持的语言代码
	LanguageAuto Language = "auto"  // 自动检测
	LanguageEnUS Language = "en-US" // 美式英语
	LanguageZhCN Language = "zh-CN" // 简体中文
)

// 全局实例
var (
	globalManager *Manager
	once          sync.Once
)

// Manager 多语言管理器
type Manager struct {
	defaultLanguage Language
	translations    map[Language]map[string]string
	mu              sync.RWMutex
}

// NewManager 创建新的多语言管理器
func NewManager(defaultLang Language) *Manager {
	return &Manager{
		defaultLanguage: defaultLang,
		translations:    make(map[Language]map[string]string),
	}
}

// GetGlobalManager 获取全局多语言管理器实例
func GetGlobalManager() *Manager {
	once.Do(func() {
		globalManager = NewManager(LanguageEnUS)
		globalManager.loadDefaultTranslations()
	})
	return globalManager
}

// SetDefaultLanguage 设置默认语言
func (m *Manager) SetDefaultLanguage(lang Language) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.defaultLanguage = lang
}

// GetDefaultLanguage 获取默认语言
func (m *Manager) GetDefaultLanguage() Language {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.defaultLanguage
}

// RegisterTranslations 注册翻译
func (m *Manager) RegisterTranslations(lang Language, translations map[string]string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.translations[lang] == nil {
		m.translations[lang] = make(map[string]string)
	}

	for key, value := range translations {
		m.translations[lang][key] = value
	}
}

// Translate 翻译文本
func (m *Manager) Translate(key string, lang Language) string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// 处理自动语言检测
	if lang == LanguageAuto {
		lang = m.defaultLanguage
	}

	// 查找指定语言的翻译
	if langMap, exists := m.translations[lang]; exists {
		if translation, found := langMap[key]; found {
			return translation
		}
	}

	// 如果找不到，尝试默认语言
	if lang != m.defaultLanguage {
		if langMap, exists := m.translations[m.defaultLanguage]; exists {
			if translation, found := langMap[key]; found {
				return translation
			}
		}
	}

	// 都找不到，返回key本身
	return key
}

// TranslateWithParams 带参数的翻译
func (m *Manager) TranslateWithParams(key string, lang Language, params map[string]interface{}) string {
	template := m.Translate(key, lang)

	// 简单的参数替换
	for paramKey, paramValue := range params {
		placeholder := fmt.Sprintf("{{%s}}", paramKey)
		template = strings.ReplaceAll(template, placeholder, fmt.Sprintf("%v", paramValue))
	}

	return template
}

// GetSupportedLanguages 获取支持的语言列表
func (m *Manager) GetSupportedLanguages() []Language {
	m.mu.RLock()
	defer m.mu.RUnlock()

	languages := make([]Language, 0, len(m.translations))
	for lang := range m.translations {
		languages = append(languages, lang)
	}
	return languages
}

// HasTranslation 检查是否有翻译
func (m *Manager) HasTranslation(key string, lang Language) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if lang == LanguageAuto {
		lang = m.defaultLanguage
	}

	if langMap, exists := m.translations[lang]; exists {
		_, found := langMap[key]
		return found
	}

	return false
}

// 全局便捷函数

// T 翻译函数（使用默认语言）
func T(key string) string {
	return GetGlobalManager().Translate(key, GetGlobalManager().GetDefaultLanguage())
}

// TL 指定语言的翻译函数
func TL(key string, lang Language) string {
	return GetGlobalManager().Translate(key, lang)
}

// TP 带参数的翻译函数
func TP(key string, params map[string]interface{}) string {
	return GetGlobalManager().TranslateWithParams(key, GetGlobalManager().GetDefaultLanguage(), params)
}

// TPL 指定语言带参数的翻译函数
func TPL(key string, lang Language, params map[string]interface{}) string {
	return GetGlobalManager().TranslateWithParams(key, lang, params)
}

// SetGlobalDefaultLanguage 设置全局默认语言
func SetGlobalDefaultLanguage(lang Language) {
	GetGlobalManager().SetDefaultLanguage(lang)
}

// RegisterGlobalTranslations 注册全局翻译
func RegisterGlobalTranslations(lang Language, translations map[string]string) {
	GetGlobalManager().RegisterTranslations(lang, translations)
}

// ValidateLanguage 验证语言代码
func ValidateLanguage(lang Language) bool {
	switch lang {
	case LanguageAuto, LanguageEnUS, LanguageZhCN:
		return true
	default:
		return false
	}
}
