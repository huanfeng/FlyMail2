import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import { useRouter } from 'vue-router'
import type { User } from '../api/types'
import { TOKEN_KEYS } from '../api/config'

export const useAuthStore = defineStore('auth', () => {
  const router = useRouter()
  const user = ref<User | null>(null)
  const isAuthenticated = computed(() => !!user.value)
  const isAdmin = computed(() => user.value?.is_admin || false)

  function setUser(userData: User) {
    user.value = userData
  }

  function logout() {
    user.value = null
    localStorage.removeItem(TOKEN_KEYS.ACCESS_TOKEN)
    localStorage.removeItem(TOKEN_KEYS.REFRESH_TOKEN)
    router.push('/login')
  }

  function clearAuth() {
    user.value = null
    localStorage.removeItem(TOKEN_KEYS.ACCESS_TOKEN)
    localStorage.removeItem(TOKEN_KEYS.REFRESH_TOKEN)
  }

  return {
    user,
    isAuthenticated,
    isAdmin,
    setUser,
    logout,
    clearAuth,
  }
})