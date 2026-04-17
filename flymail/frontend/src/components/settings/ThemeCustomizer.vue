<script lang="ts" setup>
import { ref, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { Label } from '@/components/ui/label'

const { t } = useI18n()

const themes = [
  { name: 'zinc', color: 'hsl(240 6% 10%)' },
  { name: 'slate', color: 'hsl(222 47% 11%)' },
  { name: 'stone', color: 'hsl(25 5% 45%)' },
  { name: 'gray', color: 'hsl(220 9% 46%)' },
  { name: 'neutral', color: 'hsl(0 0% 45%)' },
  { name: 'red', color: 'hsl(0 72% 51%)' },
  { name: 'rose', color: 'hsl(350 89% 60%)' },
  { name: 'orange', color: 'hsl(25 95% 53%)' },
  { name: 'green', color: 'hsl(142 71% 45%)' },
  { name: 'blue', color: 'hsl(217 91% 60%)' },
  { name: 'yellow', color: 'hsl(45 93% 47%)' },
  { name: 'violet', color: 'hsl(263 70% 50%)' },
]

const radiusOptions = [
  { value: '0', label: '0' },
  { value: '0.25', label: '0.25' },
  { value: '0.5', label: '0.5' },
  { value: '0.75', label: '0.75' },
  { value: '1', label: '1' },
]

// 从localStorage读取保存的主题设置
const savedTheme = localStorage.getItem('selectedTheme') || 'zinc'
const savedRadius = localStorage.getItem('selectedRadius') || '0.5'
const savedMode = localStorage.getItem('colorMode') as 'light' | 'dark' || 'light'

const selectedTheme = ref(savedTheme)
const selectedRadius = ref(savedRadius)
const colorMode = ref<'light' | 'dark'>(savedMode)

// 主题定制功能
function applyTheme(themeName: string) {
  selectedTheme.value = themeName

  // 移除所有主题类
  const themeClasses = themes.map(t => `theme-${t.name}`)
  document.documentElement.classList.remove(...themeClasses)

  // 添加新主题类
  const newThemeClass = `theme-${themeName}`
  document.documentElement.classList.add(newThemeClass)

  // 保存到localStorage
  localStorage.setItem('selectedTheme', themeName)
}

function applyRadius(radius: string) {
  selectedRadius.value = radius
  // 应用圆角到CSS变量
  document.documentElement.style.setProperty('--radius', `${radius}rem`)
  // 保存到localStorage
  localStorage.setItem('selectedRadius', radius)
}

function applyColorMode(mode: 'light' | 'dark') {
  colorMode.value = mode
  if (mode === 'dark') {
    document.documentElement.classList.add('dark')
  } else {
    document.documentElement.classList.remove('dark')
  }
  // 保存到localStorage
  localStorage.setItem('colorMode', mode)
}

// 初始化主题设置
onMounted(() => {
  // 应用主题
  applyTheme(selectedTheme.value)
  // 应用圆角
  applyRadius(selectedRadius.value)
  // 应用颜色模式
  if (colorMode.value === 'dark') {
    document.documentElement.classList.add('dark')
  }
})
</script>

<template>
  <div class="space-y-6">
    <!-- 主题颜色选择 -->
    <div class="space-y-3">
      <Label class="text-sm font-medium">{{ t('settings.theme.color') }}</Label>
      <div class="grid grid-cols-4 gap-3">
        <button
          v-for="theme in themes"
          :key="theme.name"
          @click="applyTheme(theme.name)"
          :class="[
            'flex items-center gap-2 rounded-md border p-3 text-sm transition-all hover:bg-accent',
            selectedTheme === theme.name && 'border-primary bg-accent'
          ]"
        >
                    <span
            class="h-4 w-4 rounded-full shrink-0"
            :style="{ backgroundColor: theme.color }"
          />
          {{ t('settings.theme.colors.' + theme.name) }}
        </button>
      </div>
    </div>

    <!-- 圆角半径选择 -->
    <div class="space-y-3">
      <Label class="text-sm font-medium">{{ t('settings.theme.radius') }}</Label>
      <div class="flex gap-2">
        <button
          v-for="option in radiusOptions"
          :key="option.value"
          @click="applyRadius(option.value)"
          :class="[
            'flex-1 rounded-md border px-3 py-2 text-sm transition-all hover:bg-accent',
            selectedRadius === option.value && 'border-primary bg-accent'
          ]"
        >
          {{ option.label }}
        </button>
      </div>
    </div>

    <!-- 颜色模式切换 -->
    <div class="space-y-3">
      <Label class="text-sm font-medium">{{ t('settings.theme.colorMode') }}</Label>
      <div class="flex gap-2">
        <button
          @click="applyColorMode('light')"
          :class="[
            'flex-1 flex items-center justify-center gap-2 rounded-md border px-4 py-2 text-sm transition-all hover:bg-accent',
            colorMode === 'light' && 'border-primary bg-accent'
          ]"
        >
          <svg class="h-4 w-4" xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
            <circle cx="12" cy="12" r="5"></circle>
            <line x1="12" y1="1" x2="12" y2="3"></line>
            <line x1="12" y1="21" x2="12" y2="23"></line>
            <line x1="4.22" y1="4.22" x2="5.64" y2="5.64"></line>
            <line x1="18.36" y1="18.36" x2="19.78" y2="19.78"></line>
            <line x1="1" y1="12" x2="3" y2="12"></line>
            <line x1="21" y1="12" x2="23" y2="12"></line>
            <line x1="4.22" y1="19.78" x2="5.64" y2="18.36"></line>
            <line x1="18.36" y1="5.64" x2="19.78" y2="4.22"></line>
          </svg>
          {{ t('settings.theme.light') }}
        </button>
        <button
          @click="applyColorMode('dark')"
          :class="[
            'flex-1 flex items-center justify-center gap-2 rounded-md border px-4 py-2 text-sm transition-all hover:bg-accent',
            colorMode === 'dark' && 'border-primary bg-accent'
          ]"
        >
          <svg class="h-4 w-4" xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
            <path d="M21 12.79A9 9 0 1 1 11.21 3 7 7 0 0 0 21 12.79z"></path>
          </svg>
          {{ t('settings.theme.dark') }}
        </button>
      </div>
    </div>
  </div>
</template>