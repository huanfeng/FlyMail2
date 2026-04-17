<script lang="ts" setup>
import { useI18n } from 'vue-i18n'
import { setLocale, type Locale } from '@/locales'
import { useSettingsStore } from '@/stores/settings'
import { Button } from '@/components/ui/button'
import { Label } from '@/components/ui/label'

const { t, locale } = useI18n()
const settingsStore = useSettingsStore()

function getCurrentLanguageValue() {
  const storedLanguage = settingsStore.settings.language
  if (storedLanguage === 'auto') {
    return 'auto'
  }
  return storedLanguage
}

function getCurrentLanguageDisplay() {
  const storedLanguage = settingsStore.settings.language
  if (storedLanguage === 'auto') {
    // 显示自动检测的结果
    const detectedLang = locale.value === 'zh' ? 'zh-CN' : 'en-US'
    return {
      label: t('languages.auto'),
      detail: `(${t(`languages.${detectedLang}`)})`
    }
  }
  return {
    label: t(`languages.${storedLanguage}`),
    detail: ''
  }
}

async function updateLanguage(value: string) {
  if (value === 'auto') {
    // 自动检测，使用浏览器语言
    const browserLang = navigator.language.startsWith('zh') ? 'zh' : 'en'
    await setLocale(browserLang as Locale)
    settingsStore.updateSettings({ language: 'auto' })
  } else {
    // 明确选择的语言
    const langCode = value === 'zh-CN' ? 'zh' : 'en'
    await setLocale(langCode as Locale)
    settingsStore.updateSettings({ language: value })
  }
}
</script>

<template>
  <div class="space-y-3">
    <Label>{{ t('settings.language.language') }}</Label>
    <div class="space-y-2">
      <!-- 当前语言状态显示 -->
      <div class="text-sm text-muted-foreground">
        {{ t('settings.language.currentLanguage') }}: {{ getCurrentLanguageDisplay().label }}{{
          getCurrentLanguageDisplay().detail }}
      </div>
      <!-- 语言选择按钮 -->
      <div class="space-y-2">
        <Button variant="outline" class="w-full justify-start h-auto p-3"
          :class="getCurrentLanguageValue() === 'auto' && 'border-primary bg-accent'"
          @click="updateLanguage('auto')">
          <div class="flex items-center justify-between w-full">
            <div class="flex flex-col items-start">
              <span class="font-medium">{{ t('languages.auto') }}</span>
              <span class="text-xs text-muted-foreground">
                {{ t('settings.language.autoDetectDescription') }}
              </span>
            </div>
            <span class="text-xs text-muted-foreground ml-2">
              ({{ locale === 'zh' ? t('languages.zh-CN') : t('languages.en-US') }})
            </span>
          </div>
        </Button>
        <Button variant="outline" class="w-full justify-start h-auto p-3"
          :class="getCurrentLanguageValue() === 'zh-CN' && 'border-primary bg-accent'"
          @click="updateLanguage('zh-CN')">
          <div class="flex flex-col items-start w-full">
            <span class="font-medium">{{ t('languages.zh-CN') }}</span>
            <span class="text-xs text-muted-foreground">
              {{ t('settings.language.chineseDescription') }}
            </span>
          </div>
        </Button>
        <Button variant="outline" class="w-full justify-start h-auto p-3"
          :class="getCurrentLanguageValue() === 'en-US' && 'border-primary bg-accent'"
          @click="updateLanguage('en-US')">
          <div class="flex flex-col items-start w-full">
            <span class="font-medium">{{ t('languages.en-US') }}</span>
            <span class="text-xs text-muted-foreground">
              {{ t('settings.language.englishDescription') }}
            </span>
          </div>
        </Button>
      </div>
    </div>
  </div>
</template>