<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { Mail } from 'lucide-vue-next'
import { useI18n } from 'vue-i18n'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Card, CardContent, CardDescription, CardFooter, CardHeader, CardTitle } from '@/components/ui/card'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Checkbox } from '@/components/ui/checkbox'
import { authService } from '@/api'
import { useAuthStore } from '@/stores/auth'
import { getErrorMessage } from '@/utils/error'

const router = useRouter()
const authStore = useAuthStore()
const { t } = useI18n()

const username = ref('')
const password = ref('')
const rememberMe = ref(false)
const isLoading = ref(false)
const error = ref('')

// Load saved credentials
function loadSavedCredentials() {
  const savedCredentials = localStorage.getItem('flymail_saved_credentials')
  if (savedCredentials) {
    try {
      const { username: savedUsername, password: savedPassword, remember } = JSON.parse(savedCredentials)
      if (remember) {
        username.value = savedUsername || ''
        password.value = savedPassword || ''
        rememberMe.value = true
      }
    } catch (err) {
      console.warn('Failed to load saved credentials:', err)
      localStorage.removeItem('flymail_saved_credentials')
    }
  }
}

// Save credentials if remember me is checked
function saveCredentials() {
  if (rememberMe.value) {
    localStorage.setItem('flymail_saved_credentials', JSON.stringify({
      username: username.value,
      password: password.value,
      remember: true
    }))
  } else {
    localStorage.removeItem('flymail_saved_credentials')
  }
}

// Check if already authenticated
onMounted(async () => {
  if (authStore.isAuthenticated) {
    router.push('/')
    return
  }

  // Load saved credentials
  loadSavedCredentials()

  // Check if we have a token and try to get user info
  const token = authService.getAccessToken()
  if (token) {
    try {
      await authService.getCurrentUser()
      router.push('/')
    } catch (err) {
      // Token is invalid, stay on login page
      authStore.clearAuth()
    }
  }
})

async function handleLogin() {
  error.value = ''

  if (!username.value || !password.value) {
    error.value = t('login.enterCredentials')
    return
  }

  isLoading.value = true

  try {
    await authService.login({
      username: username.value,
      password: password.value
    })

    // Save credentials if login successful
    saveCredentials()

    // Redirect to home or return URL
    const returnUrl = router.currentRoute.value.query.returnUrl as string
    router.push(returnUrl || '/')
  } catch (err) {
    error.value = getErrorMessage(err) || t('login.errorMessage')
  } finally {
    isLoading.value = false
  }
}

</script>

<template>
  <div class="min-h-screen flex flex-col bg-background">
    <!-- 主要内容区域 -->
    <div class="flex-1 flex items-center justify-center px-4 py-12 sm:px-6 lg:px-8">
      <div class="max-w-md w-full space-y-8">
        <!-- Logo and Title -->
        <div class="text-center">
          <div class="flex justify-center">
            <div class="rounded-full bg-primary p-3">
              <Mail class="h-8 w-8 text-primary-foreground" />
            </div>
          </div>
          <h2 class="mt-6 text-3xl font-bold tracking-tight text-foreground">
            {{ t('app.name') }}
          </h2>
          <p class="mt-2 text-sm text-muted-foreground">
            {{ t('app.title') }}
          </p>
        </div>

        <!-- Login Form -->
        <Card>
          <CardHeader>
            <CardTitle>{{ t('login.title') }}</CardTitle>
            <CardDescription>
              {{ t('login.subtitle') }}
            </CardDescription>
          </CardHeader>
          <CardContent class="space-y-4">
            <!-- Error Alert -->
            <Alert v-if="error" variant="destructive">
              <AlertDescription>{{ error }}</AlertDescription>
            </Alert>

            <!-- Login Form -->
            <form @submit.prevent="handleLogin" autocomplete="on">
              <!-- Username Field -->
              <div class="space-y-2 mb-4">
                <Label for="username">{{ t('login.username') }}</Label>
                <Input
                  id="username"
                  v-model="username"
                  type="text"
                  :placeholder="t('login.usernamePlaceholder')"
                  :disabled="isLoading"
                  autocomplete="username"
                  autofocus
                />
              </div>

              <!-- Password Field -->
              <div class="space-y-2 mb-4">
                <Label for="password">{{ t('login.password') }}</Label>
                <Input
                  id="password"
                  v-model="password"
                  type="password"
                  :placeholder="t('login.passwordPlaceholder')"
                  :disabled="isLoading"
                  autocomplete="current-password"
                />
              </div>

              <!-- Remember Me -->
              <div class="flex items-center space-x-2">
                <Checkbox
                  id="remember"
                  v-model:checked="rememberMe"
                  :disabled="isLoading"
                />
                <Label
                  for="remember"
                  class="text-sm font-normal cursor-pointer"
                >
                  {{ t('login.rememberMe') }}
                </Label>
              </div>
            </form>
          </CardContent>
          <CardFooter>
            <Button
              type="submit"
              class="w-full"
              @click="handleLogin"
              :disabled="isLoading"
            >
              <span v-if="!isLoading">{{ t('login.loginButton') }}</span>
              <span v-else class="flex items-center">
                <svg class="animate-spin -ml-1 mr-2 h-4 w-4" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24">
                  <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
                  <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
                </svg>
                {{ t('login.loggingIn') }}
              </span>
            </Button>
          </CardFooter>
        </Card>
      </div>
    </div>

    <!-- 版权声明栏 -->
    <footer class="bg-background border-t border-border py-4 px-4 sm:px-6 lg:px-8">
      <div class="max-w-7xl mx-auto">
        <div class="text-center text-xs text-muted-foreground">
          <p>
            © {{ new Date().getFullYear() }} {{ t('app.name') }}.
            <span class="mx-2">|</span>
            {{ t('app.copyright').split('©')[1] || t('app.copyright') }}
            <span class="mx-2">|</span>
            <a href="#" class="hover:text-foreground transition-colors">{{ t('app.privacyPolicy') }}</a>
            <span class="mx-2">|</span>
            <a href="#" class="hover:text-foreground transition-colors">{{ t('app.termsOfService') }}</a>
          </p>
        </div>
      </div>
    </footer>
  </div>
</template>