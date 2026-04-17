<script setup lang="ts">
import { ref, onMounted } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import { useI18n } from 'vue-i18n';
import { useToast } from 'primevue/usetoast';
import api from '../services/api';
import { useAuthStore } from '../stores/auth';
import PageHeader from '../components/PageHeader.vue';

const route = useRoute();
const router = useRouter();
const { t } = useI18n();
const toast = useToast();
const auth = useAuthStore();

const email = ref<any>(null);
const loading = ref(false);
const htmlSrc = ref('');
const appName = 'Mail2IM';

const buildHtmlSrc = async (id: string) => {
    await auth.ensureSession();
    const token = auth.accessToken;
    const tokenQuery = token ? `?access_token=${encodeURIComponent(token)}` : '';
    htmlSrc.value = `/api/emails/${id}/html${tokenQuery}`;
};

const setTitle = (text: string) => {
    document.title = text ? `${text} - ${appName}` : appName;
};

const fetchEmail = async () => {
    loading.value = true;
    const id = (route.params.id as string) || '';
    if (!id) {
        loading.value = false;
        return;
    }
    try {
        const res = await api.get(`/emails/${id}`);
        email.value = res.data;
        const titleText = email.value?.subject || t('common.email_detail');
        setTitle(titleText);
        await buildHtmlSrc(id);
    } catch (error) {
        console.error(error);
        toast.add({ severity: 'error', summary: t('common.error'), detail: t('emails_view.load_error'), life: 3000 });
    } finally {
        loading.value = false;
    }
};

const refreshEmail = () => fetchEmail();

const openStandalone = () => {
    const id = email.value?.id || route.params.id;
    if (!id) return;
    const url = router.resolve({ name: 'email-standalone', params: { id } }).href;
    window.open(url, '_blank');
};

const goBack = () => {
    if (window.history.length > 1) {
        router.back();
    } else {
        router.push('/emails');
    }
};

const formatDate = (dateStr: string) => {
    if (!dateStr) return '';
    return new Date(dateStr).toLocaleString();
};

onMounted(() => {
    fetchEmail();
});
</script>

<template>
    <div class="page detail-page">
        <PageHeader :title="t('common.email_detail')" :show-back="true" @back="goBack">
            <template #actions>
                <Button icon="pi pi-refresh" text rounded :loading="loading" @click="refreshEmail" :aria-label="t('common.refresh')" />
                <Button icon="pi pi-ellipsis-v" text rounded :aria-label="t('emails_view.more_actions')" />
                <Button icon="pi pi-external-link" rounded @click="openStandalone" :label="t('common.view')" />
            </template>
        </PageHeader>

        <div class="page-panel detail-panel">
            <div v-if="loading" class="flex justify-center p-8">
                <i class="pi pi-spin pi-spinner text-4xl"></i>
            </div>

            <div v-else-if="email" class="viewer">
                <div class="viewer-header">
                    <div class="title-block">
                        <div class="subject">{{ email.subject || '-' }}</div>
                        <div class="meta">
                            <span>{{ t('emails_view.mailbox') }}: {{ email.mailbox || email.mailbox_path || '-' }}</span>
                            <span>·</span>
                            <span>{{ t('common.from') }}: {{ email.from || '-' }}</span>
                            <span>·</span>
                            <span>{{ t('common.to') }}: {{ email.to || '-' }}</span>
                            <span>·</span>
                            <span>{{ t('common.received_at') }}: {{ formatDate(email.received_at) }}</span>
                        </div>
                    </div>
                </div>

                <div class="iframe-wrapper">
                    <iframe
                        v-if="!loading"
                        :src="htmlSrc"
                        class="mail-frame"
                        sandbox="allow-same-origin allow-popups"
                        referrerpolicy="no-referrer"
                    ></iframe>
                    <div v-else class="loading">
                        <i class="pi pi-spin pi-spinner text-3xl"></i>
                    </div>
                </div>
            </div>
        </div>
    </div>
</template>

<style scoped>
.detail-page {
    height: 100%;
    display: flex;
    flex-direction: column;
    gap: 1rem;
}

.detail-panel {
    flex: 1;
    display: flex;
    flex-direction: column;
    min-height: 0;
}

.viewer {
    display: flex;
    flex-direction: column;
    gap: 1rem;
    flex: 1;
    min-height: 0;
}

.viewer-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    flex-wrap: wrap;
    gap: 1rem;
}

.title-block {
    display: flex;
    flex-direction: column;
    gap: 0.25rem;
}

.subject {
    font-size: 1.25rem;
    font-weight: var(--fw-heading);
}

.meta {
    display: flex;
    gap: 0.5rem;
    color: var(--p-text-muted-color, #64748b);
    font-size: 0.95rem;
    flex-wrap: wrap;
}

.actions {
    display: flex;
    gap: 0.5rem;
}

.iframe-wrapper {
    border: 1px solid var(--p-surface-200);
    border-radius: 12px;
    overflow: hidden;
    flex: 1;
    min-height: 70vh;
    background: var(--p-surface-50);
}

.mail-frame {
    width: 100%;
    height: 100%;
    border: none;
    background: white;
}

.loading {
    display: flex;
    align-items: center;
    justify-content: center;
    height: 100%;
}

@media (max-width: 768px) {
    .actions {
        width: 100%;
        justify-content: flex-start;
        flex-wrap: wrap;
    }
}
</style>
