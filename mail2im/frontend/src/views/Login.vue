<script setup lang="ts">
import { ref } from 'vue';
import { useI18n } from 'vue-i18n';
import { useRoute, useRouter } from 'vue-router';
import { useAuthStore } from '../stores/auth';
import { useToast } from 'primevue/usetoast';
import Toast from 'primevue/toast';
import InputText from 'primevue/inputtext';
import Password from 'primevue/password';
import Button from 'primevue/button';
import Dialog from 'primevue/dialog';
import ToggleSwitch from 'primevue/toggleswitch';
import { getRememberedCredentials, setRememberedCredentials, clearRememberedCredentials } from '../utils/storage';

const { t } = useI18n();
const auth = useAuthStore();
const route = useRoute();
const router = useRouter();
const toast = useToast();

const identifier = ref('');
const password = ref('');
const loading = ref(false);
const showResetHelp = ref(false);
const remember = ref(false);

// Load remembered credentials (自动填充并解密)
const remembered = getRememberedCredentials();
if (remembered) {
  identifier.value = remembered.identifier;
  password.value = remembered.password;
  remember.value = true;
}

const submit = async () => {
  loading.value = true;
  try {
    await auth.login(identifier.value.trim(), password.value);
    if (remember.value) {
      // 使用加密存储记住的密码
      setRememberedCredentials(identifier.value, password.value);
    } else {
      // 清除记住的密码
      clearRememberedCredentials();
    }
    const redirect = (route.query.redirect as string) || '/';
    router.replace(redirect);
  } catch (err: any) {
    const detail = t('auth.error_login');
    toast.add({
      severity: 'error',
      summary: t('common.error'),
      detail,
      life: 3200
    });
  } finally {
    loading.value = false;
  }
};
</script>

<template>
  <div class="auth-shell">
    <Toast position="top-center" />
    <div class="auth-card">
      <div class="auth-logo">📮</div>
      <h1 class="auth-title">{{ t('auth.title') }}</h1>
      <p class="auth-subtitle">{{ t('auth.subtitle_login') }}</p>

      <form class="auth-form" @submit.prevent="submit">
        <label class="field">
          <span>{{ t('auth.username') }}</span>
          <InputText v-model="identifier" :placeholder="t('auth.username')" autocomplete="username" required class="full" />
        </label>

        <label class="field">
          <span>{{ t('auth.password') }}</span>
          <Password
            v-model="password"
            :feedback="false"
            toggle-mask
            placeholder="••••••••"
            autocomplete="current-password"
            required
            class="full"
          />
        </label>

        <div class="row-between">
          <div class="remember">
            <ToggleSwitch v-model="remember" />
            <span>{{ t('auth.remember_me') }}</span>
          </div>
          <Button link :label="t('auth.forgot_password')" @click="showResetHelp = true" />
        </div>

        <Button
          type="submit"
          :label="t('auth.login')"
          icon="pi pi-sign-in"
          :loading="loading"
          class="submit-btn"
        />
      </form>
    </div>

    <Dialog v-model:visible="showResetHelp" modal :header="t('auth.reset_help_title')" :style="{ width: '420px' }">
      <p class="reset-body">{{ t('auth.reset_help_body') }}</p>
      <pre class="reset-command"><code>{{ t('auth.reset_help_cmd') }}</code></pre>
      <template #footer>
        <Button :label="t('common.confirm')" icon="pi pi-check" @click="showResetHelp = false" />
      </template>
    </Dialog>
  </div>
</template>

<style scoped>
.auth-shell {
  min-height: 100vh;
  display: grid;
  place-items: center;
  background: radial-gradient(circle at 20% 20%, #f8fbff, #e9f0ff 35%, #e5e7eb 70%);
  padding: 1.5rem;
  box-sizing: border-box;
}

.auth-card {
  width: min(460px, 100%);
  background: white;
  border-radius: 18px;
  padding: 2rem;
  box-shadow: 0 20px 60px rgba(15, 23, 42, 0.12);
  border: 1px solid #e5e7eb;
  display: flex;
  flex-direction: column;
  gap: 0.75rem;
  align-items: center;
  text-align: center;
}

.auth-logo {
  width: 72px;
  height: 72px;
  border-radius: 20px;
  background: linear-gradient(135deg, #6ee7b7, #34d399);
  color: white;
  display: grid;
  place-items: center;
  font-size: 2rem;
  box-shadow: 0 12px 30px rgba(52, 211, 153, 0.35);
}

.auth-title {
  margin: 0;
  font-size: 1.5rem;
  font-weight: 800;
  color: #0f172a;
}

.auth-subtitle {
  margin: 0;
  color: #6b7280;
  font-size: 1rem;
}

.auth-form {
  width: 100%;
  display: flex;
  flex-direction: column;
  gap: 1rem;
  margin-top: 0.5rem;
  width: 100%;
}

.field {
  display: flex;
  flex-direction: column;
  gap: 0.35rem;
  align-items: flex-start;
  text-align: left;
}

.field span {
  font-size: 0.95rem;
  color: #1f2937;
  font-weight: 600;
}

.auth-form :deep(.p-inputtext),
.auth-form :deep(.p-password),
.auth-form :deep(.p-password-input),
.auth-form :deep(.p-inputwrapper) {
  width: 100%;
}

.row-between {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.remember {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  color: #475569;
  font-size: 0.95rem;
}

.submit-btn :deep(.p-button) {
  width: 100%;
}

.reset-body {
  margin: 0 0 0.75rem;
  color: #4b5563;
  line-height: 1.5;
}

.reset-command {
  background: #0f172a;
  color: #e5e7eb;
  padding: 0.75rem 1rem;
  border-radius: 10px;
  overflow: auto;
}

@media (max-width: 480px) {
  .auth-card {
    padding: 1.5rem;
  }
}
</style>
