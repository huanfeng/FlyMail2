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

const { onMenuToggle, layoutConfig } = useLayout();
const { t } = useI18n();
const router = useRouter();
const toast = useToast();
const auth = useAuthStore(pinia);

const isDark = computed(() => layoutConfig.darkTheme);
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

const toggleDarkMode = () => {
    layoutConfig.darkTheme = !layoutConfig.darkTheme;
};

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
    <div>
        <div class="layout-topbar mobile-topbar">
            <div class="layout-topbar-logo-container">
                <button class="layout-menu-button layout-topbar-action" @click="onMenuToggle">
                    <i class="pi pi-bars"></i>
                </button>
                <router-link to="/" class="layout-topbar-logo">
                    <i class="pi pi-send text-primary text-2xl mr-2"></i>
                    <span>Mail2IM</span>
                </router-link>
            </div>

            <div class="layout-topbar-actions">
                <!-- Dark Mode -->
                <button class="layout-topbar-action" @click="toggleDarkMode">
                    <i :class="['pi', isDark ? 'pi-moon' : 'pi-sun']"></i>
                </button>

                <!-- User -->
                <div class="relative" ref="userMenuRef">
                    <button class="layout-topbar-action" @click.stop="userMenuOpen = !userMenuOpen" :aria-label="t('user.profile')">
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
        </div>

        <Dialog v-model:visible="profileDialog" modal :header="t('user.profile')" :style="{ width: '520px' }">
            <form class="profile-grid" @submit.prevent="saveProfile">
                <div class="form-row">
                    <label class="form-label" for="username">{{ t('user.username') }}</label>
                    <InputText id="username" v-model="profileForm.username" autocomplete="username" class="flex-1" />
                </div>
                <div class="form-row">
                    <label class="form-label" for="email">{{ t('user.email') }}</label>
                    <InputText id="email" v-model="profileForm.email" autocomplete="email" class="flex-1" />
                </div>
                <div class="form-row">
                    <label class="form-label" for="current">{{ t('user.current_password') }}</label>
                    <Password id="current" v-model="profileForm.currentPassword" toggle-mask :feedback="false" autocomplete="current-password" class="flex-1" />
                </div>
                <div class="form-row">
                    <label class="form-label" for="new">{{ t('user.new_password') }}</label>
                    <Password id="new" v-model="profileForm.newPassword" toggle-mask :feedback="false" autocomplete="new-password" class="flex-1" />
                </div>
                <div class="form-row">
                    <label class="form-label" for="confirm">{{ t('user.new_password_confirm') }}</label>
                    <Password id="confirm" v-model="profileForm.confirmPassword" toggle-mask :feedback="false" autocomplete="new-password" class="flex-1" />
                </div>
                <div class="form-actions">
                    <Button type="button" text :label="t('user.cancel')" @click="profileDialog = false" class="mr-2" />
                    <Button type="submit" :label="t('user.save')" icon="pi pi-save" :loading="savingProfile" />
                </div>
            </form>
        </Dialog>
    </div>
</template>

<style lang="scss" scoped>
.layout-topbar {
    position: fixed;
    height: 5rem;
    z-index: 997;
    left: 0;
    top: 0;
    width: 100%;
    padding: 0 1.25rem;
    background-color: var(--surface-card);
    backdrop-filter: blur(10px);
    border-bottom: 1px solid var(--surface-border);
    transition: left 0.2s;
    display: flex;
    align-items: center;
    box-shadow: 0px 3px 5px rgba(0,0,0,.02), 0px 0px 2px rgba(0,0,0,.05), 0px 1px 4px rgba(0,0,0,.08);
}

:deep(.p-popover) {
    background: var(--surface-overlay);
    border: 1px solid var(--surface-border);
    box-shadow: 0 2px 12px rgba(0, 0, 0, 0.1);
    border-radius: 6px;
    padding: 1rem;
}

.layout-topbar-menu {
    margin: 0 0 0 auto;
    padding: 0;
    list-style: none;
    display: flex;

    .layout-topbar-button {
        display: inline-flex;
        justify-content: center;
        align-items: center;
        position: relative;
        color: var(--text-color-secondary);
        border-radius: 50%;
        width: 3rem;
        height: 3rem;
        cursor: pointer;
        transition: background-color 0.2s;

        &:hover {
            color: var(--text-color);
            background-color: var(--surface-hover);
        }

        i {
            font-size: 1.5rem;
        }

        span {
            font-size: 1rem;
            display: none;
        }
    }
}

.layout-topbar-logo-container {
    display: flex;
    align-items: center;
    width: 300px;
}

.layout-topbar-logo {
    display: flex;
    align-items: center;
    color: var(--p-text-color);
    font-size: 1.5rem;
    font-weight: 500;
    width: 100%;
    border-radius: 12px;
}

.layout-topbar-logo:focus {
    outline: none;
    box-shadow: 0 0 0 2px var(--p-primary-color);
}

.layout-topbar-actions {
    display: flex;
    align-items: center;
    margin-left: auto;
    gap: 0.5rem;
}

.layout-topbar-action {
    display: inline-flex;
    justify-content: center;
    align-items: center;
    color: var(--p-text-color); /* Improved visibility */
    border-radius: 50%;
    width: 3rem;
    height: 3rem;
    transition: background-color 0.2s;
    cursor: pointer;
    border: 0 none;
    background: transparent;
}

.layout-topbar-action:hover {
    background-color: var(--p-surface-100);
}

.app-dark .layout-topbar-action:hover {
    background-color: var(--p-surface-800);
}

.layout-topbar-action i {
    font-size: 1.5rem;
}

.layout-topbar-action span.action-label {
    font-size: 1rem;
    display: none;
}

.user-menu {
    position: absolute;
    top: calc(100% + 10px);
    right: 0;
    background: var(--p-surface-0);
    border: 1px solid var(--p-surface-200);
    border-radius: 12px;
    padding: 0.75rem;
    min-width: 180px;
    box-shadow: 0 8px 24px rgba(15, 23, 42, 0.08);
    z-index: 999;
}

.app-dark .user-menu {
    background: var(--p-surface-900);
    border-color: var(--p-surface-700);
}

.menu-header {
    font-size: 0.9rem;
    color: var(--p-text-muted-color, #94a3b8);
    margin-bottom: 0.35rem;
}

.menu-sub {
    font-size: 0.85rem;
    color: var(--p-text-muted-color, #94a3b8);
    margin-bottom: 0.5rem;
    word-break: break-all;
}

.menu-item {
    width: 100%;
    text-align: left;
    padding: 0.5rem;
    border: none;
    background: transparent;
    cursor: pointer;
    border-radius: 10px;
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

.menu-toggle {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 0.5rem;
}

.menu-item.danger {
    color: #ef4444;
}

.layout-menu-button {
    margin-right: 2rem;
    display: none;
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

@media (max-width: 991px) {
    .layout-topbar {
        justify-content: space-between;
    }

    .layout-topbar-logo {
        width: auto;
        order: 2;
    }

    .layout-menu-button {
        display: flex;
        order: 1;
        margin-right: 0;
    }
}

@media (min-width: 992px) {
    .mobile-topbar {
        display: none;
    }
}
</style>
