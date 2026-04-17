/**
 * 语言检测工具
 * 模拟浏览器 Accept-Language 头的行为来检测用户的语言偏好
 */

export type SupportedLocale = 'en' | 'zh'

interface LanguageDetectionResult {
  detectedLocale: SupportedLocale
  source: 'saved' | 'navigator.languages' | 'navigator.language' | 'fallback'
  detectedLanguages: string[]
  matchedLanguage?: string
}

/**
 * 检测用户的语言偏好
 * 按照以下优先级检测：
 * 1. 用户手动设置的语言（localStorage）
 * 2. navigator.languages 数组（类似 Accept-Language）
 * 3. navigator.language 主要语言
 * 4. 默认英文
 */
export function detectUserLanguage(
  storageKey: string = 'flymailplus-locale',
  supportedLocales: SupportedLocale[] = ['en', 'zh']
): LanguageDetectionResult {
  const result: LanguageDetectionResult = {
    detectedLocale: 'en',
    source: 'fallback',
    detectedLanguages: []
  }

  // 1. 检查用户手动设置的语言
  const saved = localStorage.getItem(storageKey)
  if (saved && supportedLocales.includes(saved as SupportedLocale)) {
    result.detectedLocale = saved as SupportedLocale
    result.source = 'saved'
    return result
  }

  // 2. 获取浏览器语言偏好列表
  const languages = navigator.languages || [navigator.language]
  result.detectedLanguages = [...languages]

  // 3. 遍历语言偏好列表，寻找匹配的语言
  for (const lang of languages) {
    const normalizedLang = lang.toLowerCase()
    
    // 检查中文变体 (zh, zh-CN, zh-TW, zh-HK 等)
    if (normalizedLang.startsWith('zh') && supportedLocales.includes('zh')) {
      result.detectedLocale = 'zh'
      result.source = 'navigator.languages'
      result.matchedLanguage = lang
      return result
    }
    
    // 检查英文变体 (en, en-US, en-GB 等)
    if (normalizedLang.startsWith('en') && supportedLocales.includes('en')) {
      result.detectedLocale = 'en'
      result.source = 'navigator.languages'
      result.matchedLanguage = lang
      return result
    }
  }

  // 4. 如果都不匹配，检查主要语言作为后备
  const primaryLang = navigator.language.toLowerCase().split('-')[0]
  if (primaryLang === 'zh' && supportedLocales.includes('zh')) {
    result.detectedLocale = 'zh'
    result.source = 'navigator.language'
    result.matchedLanguage = navigator.language
    return result
  }

  // 5. 默认返回英文
  result.detectedLocale = 'en'
  result.source = 'fallback'
  return result
}

/**
 * 获取浏览器的语言偏好信息（用于调试）
 */
export function getBrowserLanguageInfo() {
  return {
    language: navigator.language,
    languages: navigator.languages || [navigator.language],
    userLanguage: (navigator as any).userLanguage, // IE 兼容
    browserLanguage: (navigator as any).browserLanguage, // IE 兼容
    systemLanguage: (navigator as any).systemLanguage, // IE 兼容
    timezone: Intl.DateTimeFormat().resolvedOptions().timeZone,
    locale: new Intl.NumberFormat().resolvedOptions().locale
  }
}

/**
 * 模拟 Accept-Language 头的值
 * 将 navigator.languages 转换为类似 Accept-Language 的格式
 */
export function simulateAcceptLanguageHeader(): string {
  const languages = navigator.languages || [navigator.language]
  
  return languages
    .map((lang, index) => {
      // Accept-Language 使用 q 值表示优先级，从 1.0 递减
      const qValue = index === 0 ? '' : `;q=${(1.0 - index * 0.1).toFixed(1)}`
      return `${lang}${qValue}`
    })
    .join(', ')
}