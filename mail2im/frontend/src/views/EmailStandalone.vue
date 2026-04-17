<script setup lang="ts">
import { ref, onMounted } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import { useI18n } from 'vue-i18n';
import { useToast } from 'primevue/usetoast';
import api from '../services/api';
import { useAuthStore } from '../stores/auth';
import Toast from 'primevue/toast';

const route = useRoute();
const router = useRouter();
const { t } = useI18n();
const toast = useToast();
const auth = useAuthStore();
const appName = 'Mail2IM';

const email = ref<any>(null);
const loading = ref(false);
const htmlSrc = ref('');
const srcDoc = ref('');

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
  const token = (route.params.token as string) || '';

  if (!id && !token) {
    loading.value = false;
    return;
  }

  try {
    if (token) {
        // Public access
        const res = await api.get(`/public/emails/${token}`);
        email.value = res.data;
        srcDoc.value = email.value.html_body || email.value.text_body || '';
        // If no HTML, wrap text
        if (!email.value.html_body && email.value.text_body) {
            srcDoc.value = `<pre style="white-space: pre-wrap; font-family: sans-serif;">${email.value.text_body}</pre>`;
        }
    } else {
        // Private access
        const res = await api.get(`/emails/${id}`);
        email.value = res.data;
        await buildHtmlSrc(id);
    }
    setTitle(email.value?.subject || t('common.email_detail'));
  } catch (error) {
    console.error(error);
    toast.add({ severity: 'error', summary: t('common.error'), detail: t('emails_view.load_error'), life: 3000 });
  } finally {
    loading.value = false;
  }
};

const backToList = () => {
  if (route.params.token) return; // No back button for public view
  const url = router.resolve({ name: 'emails' }).href;
  window.open(url, '_blank');
};

const formatDate = (dateStr: string) => {
  if (!dateStr) return '-';
  const d = new Date(dateStr);
  return isNaN(d.getTime()) ? '-' : d.toLocaleString();
};

onMounted(fetchEmail);
</script>

<template>
  <div class="standalone">
    <Toast position="top-center" />
    <div class="viewer">
      <div class="viewer-header">
        <div class="title-block">
          <div class="subject">{{ email?.subject || '-' }}</div>
          <div class="meta">
            <span>{{ t('emails_view.mailbox') }}: {{ email?.mailbox || email?.mailbox_path || '-' }}</span>
            <span>·</span>
            <span>{{ t('common.from') }}: {{ email?.from || '-' }}</span>
            <span>·</span>
            <span>{{ t('common.to') }}: {{ email?.to || '-' }}</span>
            <span>·</span>
            <span>{{ t('common.received_at') }}: {{ formatDate(email?.received_at) }}</span>
          </div>
        </div>
        <div class="actions">
          <Button icon="pi pi-refresh" :label="t('common.refresh')" text rounded @click="fetchEmail" :loading="loading" />
          <Button icon="pi pi-ellipsis-v" text rounded :aria-label="t('emails_view.more_actions')" />
          <Button icon="pi pi-list" :label="t('emails_view.list')" text rounded @click="backToList" />
        </div>
      </div>

      <div class="iframe-wrapper">
        <iframe
          v-if="!loading"
          :src="srcDoc ? undefined : htmlSrc"
          :srcdoc="srcDoc || undefined"
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
</template>

<style scoped>
.standalone {
  min-height: 100vh;
  background: #f8fafc;
  padding: 0;
  box-sizing: border-box;
  display: flex;
}

.viewer {
  width: 100%;
  min-height: 100vh;
  background: white;
  display: flex;
  flex-direction: column;
  gap: 1rem;
  padding: 1rem 1.25rem;
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
  color: #0f172a;
}

.meta {
  display: flex;
  gap: 0.5rem;
  color: #64748b;
  font-size: 0.95rem;
  flex-wrap: wrap;
}

.actions {
  display: flex;
  gap: 0.5rem;
}

.iframe-wrapper {
  border: 1px solid #e2e8f0;
  border-radius: 12px;
  overflow: hidden;
  flex: 1;
  min-height: 60vh;
  background: #f8fafc;
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
  .viewer {
    padding: 1rem;
  }

  .actions {
    width: 100%;
    justify-content: flex-start;
    flex-wrap: wrap;
  }
}
</style>
