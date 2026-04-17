<script lang="ts" setup>
import { ref, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { Eye, EyeOff, Save, Loader2 } from 'lucide-vue-next'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Separator } from '@/components/ui/separator'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { useAuthStore } from '@/stores/auth'
import { authService } from '@/api'
import type { UpdateCredentialsRequest } from '@/api/types'
import { getErrorMessage } from '@/utils/error'

const { t } = useI18n()
const authStore = useAuthStore()

// 管理员信息表单状态
const adminForm = ref<UpdateCredentialsRequest>({
  username: '',
  email: '',
  password: '',
  old_password: ''
})
const showPassword = ref(false)
const showOldPassword = ref(false)
const isUpdatingAdmin = ref(false)
const adminUpdateError = ref<string | null>(null)
const adminUpdateSuccess = ref(false)

// 初始化管理员表单
onMounted(() => {
  if (authStore.user) {
    adminForm.value.username = authStore.user.username || ''
    adminForm.value.email = authStore.user.email || ''
  }
})

// 更新管理员信息
async function updateAdminCredentials() {
  // 验证表单
  const hasChanges = adminForm.value.username !== authStore.user?.username ||
                    adminForm.value.email !== authStore.user?.email ||
                    adminForm.value.password

  if (!hasChanges) {
    adminUpdateError.value = t('settings.security.noChanges')
    return
  }

  if (adminForm.value.password && !adminForm.value.old_password) {
    adminUpdateError.value = t('settings.security.passwordChangeRequiresCurrent')
    return
  }

  isUpdatingAdmin.value = true
  adminUpdateError.value = null
  adminUpdateSuccess.value = false

  try {
    // 构建请求数据，只包含有变化的字段
    const updateData: UpdateCredentialsRequest = {}

    if (adminForm.value.username && adminForm.value.username !== authStore.user?.username) {
      updateData.username = adminForm.value.username
    }

    if (adminForm.value.email && adminForm.value.email !== authStore.user?.email) {
      updateData.email = adminForm.value.email
    }

    if (adminForm.value.password) {
      updateData.password = adminForm.value.password
      updateData.old_password = adminForm.value.old_password
    }

    await authService.updateCredentials(updateData)

    adminUpdateSuccess.value = true
    // 清空密码字段
    adminForm.value.password = ''
    adminForm.value.old_password = ''

    // 3秒后清除成功消息
    setTimeout(() => {
      adminUpdateSuccess.value = false
    }, 3000)
  } catch (err) {
    adminUpdateError.value = getErrorMessage(err) || t('settings.security.updateFailed')
  } finally {
    isUpdatingAdmin.value = false
  }
}
</script>

<template>
  <div v-if="authStore.user?.is_admin" class="space-y-4">
    <h4 class="text-sm font-medium">{{ t('settings.security.adminInfo') }}</h4>

    <!-- 成功/错误提示 -->
    <Alert v-if="adminUpdateSuccess" class="bg-green-50 border-green-200">
      <AlertDescription class="text-green-800">
        {{ t('settings.security.adminInfoSuccess') }}
      </AlertDescription>
    </Alert>

    <Alert v-if="adminUpdateError" variant="destructive">
      <AlertDescription>{{ adminUpdateError }}</AlertDescription>
    </Alert>

    <div class="space-y-4">
      <div class="space-y-2">
        <Label for="admin_username">{{ t('settings.security.username') }}</Label>
        <Input
          id="admin_username"
          v-model="adminForm.username"
          :placeholder="t('settings.security.username')"
        />
      </div>

      <div class="space-y-2">
        <Label for="admin_email">{{ t('settings.security.email') }}</Label>
        <Input
          id="admin_email"
          v-model="adminForm.email"
          type="email"
          :placeholder="t('settings.security.email')"
        />
      </div>

      <Separator />

      <div class="space-y-2">
        <Label for="new_password">{{ t('settings.security.newPassword') }} <span
            class="text-muted-foreground text-sm">({{ t('settings.security.newPasswordDesc')
            }})</span></Label>
        <div class="relative">
          <Input
            id="new_password"
            v-model="adminForm.password"
            :type="showPassword ? 'text' : 'password'"
            :placeholder="t('settings.security.newPassword')"
            class="pr-10"
          />
          <Button
            type="button"
            variant="ghost"
            size="icon"
            class="absolute right-0 top-0 h-full w-10"
            @click="showPassword = !showPassword"
          >
            <Eye v-if="showPassword" class="h-4 w-4" />
            <EyeOff v-else class="h-4 w-4" />
          </Button>
        </div>
      </div>

      <div v-if="adminForm.password" class="space-y-2">
        <Label for="old_password">{{ t('settings.security.currentPassword') }} <span
            class="text-destructive">*</span></Label>
        <div class="relative">
          <Input
            id="old_password"
            v-model="adminForm.old_password"
            :type="showOldPassword ? 'text' : 'password'"
            :placeholder="t('settings.security.currentPassword')"
            class="pr-10"
          />
          <Button
            type="button"
            variant="ghost"
            size="icon"
            class="absolute right-0 top-0 h-full w-10"
            @click="showOldPassword = !showOldPassword"
          >
            <Eye v-if="showOldPassword" class="h-4 w-4" />
            <EyeOff v-else class="h-4 w-4" />
          </Button>
        </div>
      </div>

      <Button
        @click="updateAdminCredentials"
        :disabled="isUpdatingAdmin"
        class="w-full sm:w-auto"
      >
        <Loader2 v-if="isUpdatingAdmin" class="mr-2 h-4 w-4 animate-spin" />
        <Save v-else class="mr-2 h-4 w-4" />
        {{ isUpdatingAdmin ? t('common.saving') : t('common.save') }}
      </Button>
    </div>
  </div>
</template>