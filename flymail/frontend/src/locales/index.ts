import { createI18n } from 'vue-i18n'
import { nextTick } from 'vue'
import en from './en.json'
import zh from './zh.json'
import { detectUserLanguage, getBrowserLanguageInfo, simulateAcceptLanguageHeader } from '@/utils/languageDetection'

export type MessageSchema = typeof en

export type Locale = 'en' | 'zh'

const LOCALE_KEY = 'flymailplus-locale'

function getDefaultLocale(): Locale {
  const detection = detectUserLanguage(LOCALE_KEY, ['en', 'zh'])
  
  // 在开发环境输出语言检测信息
  if (import.meta.env.DEV) {
    console.group('🌍 Language Detection')
    console.log('Detected locale:', detection.detectedLocale)
    console.log('Detection source:', detection.source)
    console.log('Browser languages:', detection.detectedLanguages)
    if (detection.matchedLanguage) {
      console.log('Matched language:', detection.matchedLanguage)
    }
    console.log('Browser info:', getBrowserLanguageInfo())
    console.log('Simulated Accept-Language:', simulateAcceptLanguageHeader())
    console.groupEnd()
  }
  
  return detection.detectedLocale
}

export const i18n = createI18n<[MessageSchema], Locale>({
  legacy: false,
  locale: getDefaultLocale(),
  fallbackLocale: 'en',
  messages: {
    en,
    zh
  },
  datetimeFormats: {
    en: {
      short: {
        year: 'numeric',
        month: 'short',
        day: 'numeric'
      },
      long: {
        year: 'numeric',
        month: 'long',
        day: 'numeric',
        weekday: 'long',
        hour: 'numeric',
        minute: 'numeric'
      }
    },
    zh: {
      short: {
        year: 'numeric',
        month: 'short',
        day: 'numeric'
      },
      long: {
        year: 'numeric',
        month: 'long',
        day: 'numeric',
        weekday: 'long',
        hour: 'numeric',
        minute: 'numeric',
        hour12: false
      }
    }
  },
  numberFormats: {
    en: {
      currency: {
        style: 'currency',
        currency: 'USD'
      }
    },
    zh: {
      currency: {
        style: 'currency',
        currency: 'CNY'
      }
    }
  }
})

export async function setLocale(locale: Locale) {
  // Type assertion needed for vue-i18n locale property
  (i18n.global.locale as any).value = locale
  localStorage.setItem(LOCALE_KEY, locale)
  
  document.querySelector('html')?.setAttribute('lang', locale)
  
  await nextTick()
}

export function useI18n() {
  return i18n.global
}