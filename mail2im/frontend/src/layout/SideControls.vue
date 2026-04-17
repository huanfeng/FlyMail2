<script setup lang="ts">
import { useLayout } from './composables/layout';
import { useI18n } from 'vue-i18n';
import { ref, computed, onMounted, onBeforeUnmount, reactive } from 'vue';
import { useAuthStore } from '../stores/auth';
import { pinia } from '../stores';
import { useRouter } from 'vue-router';
import { useToast } from 'primevue/usetoast';
import Dialog from 'primevue/dialog';
import InputText from 'primevue/inputtext';
import Password from 'primevue/password';
import Button from 'primevue/button';

const { layoutConfig } = useLayout();
const { t } = useI18n();
const auth = useAuthStore(pinia);
const router = useRouter();
const toast = useToast();

const isDark = computed(() => layoutConfig.darkTheme);
const toggleDarkMode = () => {
  layoutConfig.darkTheme = !layoutConfig.darkTheme;
};

const userMenuOpen = ref(false);
const userMenuRef = ref<HTMLElement | null>(null);
const profileDialog = ref(false);
const savingProfile = ref(false);
const profileForm = reactive({
  username: '',
  email: '',
  currentPassword: '',
  newPassword: '',
  confirmPassword: ''
});

const userName = computed(() => auth.user?.username || 'User');
const userEmail = computed(() => auth.user?.email || '');

const handleClickOutside = (event: MouseEvent) => {
  if (userMenuRef.value && !userMenuRef.value.contains(event.target as Node)) {
    userMenuOpen.value = false;
  }
};

const openProfile = () => {
  auth.loadFromStorage();
  profileForm.username = auth.user?.username || '';
  profileForm.email = auth.user?.email || '';
  profileForm.currentPassword = '';
  profileForm.newPassword = '';
  profileForm.confirmPassword = '';
  profileDialog.value = true;
  userMenuOpen.value = false;
};

const logout = () => {
  auth.logout();
  userMenuOpen.value = false;
  profileDialog.value = false;
  router.push({ name: 'login' });
};

const saveProfile = async () => {
  savingProfile.value = true;
  try {
    if (profileForm.newPassword && profileForm.newPassword !== profileForm.confirmPassword) {
      toast.add({ severity: 'warn', summary: t('common.warning'), detail: t('user.password_mismatch'), life: 2500 });
      savingProfile.value = false;
      return;
    }
    await auth.updateProfile({
      username: profileForm.username.trim(),
      email: profileForm.email.trim(),
      current_password: profileForm.currentPassword,
      new_password: profileForm.newPassword
    });
    toast.add({ severity: 'success', summary: t('common.success'), detail: t('user.update_success'), life: 2500 });
    profileDialog.value = false;
    profileForm.currentPassword = '';
    profileForm.newPassword = '';
  } catch (err) {
    console.error(err);
    toast.add({ severity: 'error', summary: t('common.error'), detail: t('user.update_error'), life: 3200 });
  } finally {
    savingProfile.value = false;
  }
};

onMounted(() => {
  auth.loadFromStorage();
  document.addEventListener('click', handleClickOutside);
});

onBeforeUnmount(() => {
  document.removeEventListener('click', handleClickOutside);
});
</script>

<template>
  <div class="controls-card">
    <div class="brand">
      <i class="pi pi-send text-primary text-xl"></i>
      <div class="brand-text">
        <div class="title">Mail2IM</div>
        <div class="subtitle">{{ userName }}</div>
      </div>
    </div>
    <div class="actions">
      <button class="control-btn" @click="toggleDarkMode" title="Dark Mode">
        <i :class="['pi', isDark ? 'pi-moon' : 'pi-sun']"></i>
      </button>
      <div class="relative" ref="userMenuRef">
        <button class="control-btn" :title="t('user.profile')" @click.stop="userMenuOpen = !userMenuOpen">
          <i class="pi pi-user"></i>
        </button>
        <div v-if="userMenuOpen" class="user-menu">
          <div class="menu-header">{{ t('user.greeting', { name: userName }) }}</div>
          <div class="menu-sub" v-if="userEmail">{{ userEmail }}</div>
          <button class="menu-item" @click="openProfile">
            <i class="pi pi-user-edit mr-2"></i>
            <span>{{ t('user.profile') }}</span>
          </button>
          <button class="menu-item danger" @click="logout">
            <i class="pi pi-sign-out mr-2"></i>
            <span>{{ t('user.logout') }}</span>
          </button>
        </div>
      </div>
    </div>

    <Dialog v-model:visible="profileDialog" modal :header="t('user.profile')" :style="{ width: '520px' }">
      <form class="profile-grid" @submit.prevent="saveProfile">
        <div class="form-row">
          <label class="form-label" for="username-side">{{ t('user.username') }}</label>
          <InputText id="username-side" v-model="profileForm.username" autocomplete="username" class="flex-1" />
        </div>
        <div class="form-row">
          <label class="form-label" for="email-side">{{ t('user.email') }}</label>
          <InputText id="email-side" v-model="profileForm.email" autocomplete="email" class="flex-1" />
        </div>
        <div class="form-row">
          <label class="form-label" for="current-side">{{ t('user.current_password') }}</label>
          <Password id="current-side" v-model="profileForm.currentPassword" toggle-mask :feedback="false" autocomplete="current-password" class="flex-1" />
        </div>
        <div class="form-row">
          <label class="form-label" for="new-side">{{ t('user.new_password') }}</label>
          <Password id="new-side" v-model="profileForm.newPassword" toggle-mask :feedback="false" autocomplete="new-password" class="flex-1" />
        </div>
        <div class="form-row">
          <label class="form-label" for="confirm-side">{{ t('user.new_password_confirm') }}</label>
          <Password id="confirm-side" v-model="profileForm.confirmPassword" toggle-mask :feedback="false" autocomplete="new-password" class="flex-1" />
        </div>
        <div class="form-actions">
          <Button type="button" text :label="t('user.cancel')" @click="profileDialog = false" class="mr-2" />
          <Button type="submit" :label="t('user.save')" icon="pi pi-save" :loading="savingProfile" />
        </div>
      </form>
    </Dialog>
  </div>
</template>

<style scoped>
.controls-card {
  background: var(--p-surface-0);
  border-radius: 14px;
  padding: 1rem 1.25rem;
  border: 1px solid var(--p-surface-200);
  box-shadow: 0 6px 20px rgba(15, 23, 42, 0.04);
  display: flex;
  align-items: center;
  gap: 1rem;
}

.app-dark .controls-card {
  background: var(--p-surface-900);
  border-color: var(--p-surface-700);
}

.brand {
  display: flex;
  align-items: center;
  gap: 0.75rem;
}

.brand-text .title {
  font-weight: var(--fw-heading);
  font-size: 1rem;
  color: var(--p-text-color);
}

.brand-text .subtitle {
  font-size: 0.85rem;
  color: var(--p-text-muted-color, #94a3b8);
}

.actions {
  margin-left: auto;
  display: flex;
  gap: 0.35rem;
}

.control-btn {
  width: 2.75rem;
  height: 2.75rem;
  border-radius: 12px;
  border: 1px solid var(--p-surface-200);
  background: transparent;
  color: var(--p-text-color);
  display: inline-flex;
  align-items: center;
  justify-content: center;
  cursor: pointer;
  transition: background-color 0.2s, border-color 0.2s;
}

.control-btn:hover {
  background: var(--p-surface-100);
  border-color: var(--p-surface-300);
}

.app-dark .control-btn {
  border-color: var(--p-surface-700);
}

.app-dark .control-btn:hover {
  background: var(--p-surface-800);
  border-color: var(--p-surface-600);
}

.user-menu {
  position: absolute;
  top: calc(100% + 8px);
  right: 0;
  background: var(--p-surface-0);
  border: 1px solid var(--p-surface-200);
  border-radius: 10px;
  padding: 0.5rem 0.75rem;
  min-width: 160px;
  box-shadow: 0 8px 24px rgba(15, 23, 42, 0.08);
  z-index: 10;
}

.app-dark .user-menu {
  background: var(--p-surface-900);
  border-color: var(--p-surface-700);
}

.menu-sub {
  font-size: 0.85rem;
  color: var(--p-text-muted-color, #94a3b8);
  margin-bottom: 0.5rem;
  word-break: break-all;
}

.menu-header {
  font-size: 0.85rem;
  color: var(--p-text-muted-color, #94a3b8);
  padding: 0.25rem 0;
}

.menu-item {
  width: 100%;
  text-align: left;
  padding: 0.45rem 0.35rem;
  border: none;
  background: transparent;
  cursor: pointer;
  border-radius: 8px;
  color: var(--p-text-color);
  transition: background-color 0.15s;
}

.menu-item:hover {
  background: var(--p-surface-100);
}

.app-dark .menu-item:hover {
  background: var(--p-surface-800);
}

.menu-item.active {
  background: var(--p-primary-50);
  color: var(--p-primary-color);
}

.menu-item.danger {
  color: #ef4444;
}

.menu-toggle {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 0.5rem;
}

.submenu {
  display: flex;
  flex-direction: column;
  gap: 0.35rem;
  padding-top: 0.35rem;
}

.profile-grid {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.form-row {
  display: flex;
  align-items: center;
  gap: 12px;
}

.form-label {
  min-width: 110px;
  text-align: right;
  color: var(--text-color);
  font-weight: 600;
}

.form-actions {
  display: flex;
  justify-content: flex-end;
  gap: 0.5rem;
}

.form-row :deep(.p-password),
.form-row :deep(.p-password-input),
.form-row :deep(.p-inputtext),
.form-row :deep(.p-inputwrapper) {
  width: 100%;
}
</style>
