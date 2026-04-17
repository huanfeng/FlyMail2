import { defineStore } from 'pinia'
import { ref } from 'vue'

export interface Settings {
  theme: 'light' | 'dark' | 'system'
  language: string
  fontSize: number
  notifications: {
    desktop: boolean
    sound: boolean
    email: boolean
  }
}

export const useSettingsStore = defineStore('settings', () => {
  const settings = ref<Settings>({
    theme: 'system',
    language: 'auto',
    fontSize: 14,
    notifications: {
      desktop: true,
      sound: true,
      email: false,
    },
  })
  
  const isSettingsOpen = ref(false)
  
  function updateSettings(newSettings: Partial<Settings>) {
    settings.value = { ...settings.value, ...newSettings }
  }
  
  function openSettings() {
    isSettingsOpen.value = true
  }
  
  function closeSettings() {
    isSettingsOpen.value = false
  }
  
  function toggleSettings() {
    isSettingsOpen.value = !isSettingsOpen.value
  }
  
  return {
    settings,
    isSettingsOpen,
    updateSettings,
    openSettings,
    closeSettings,
    toggleSettings,
  }
})