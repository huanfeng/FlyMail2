<script setup lang="ts">
import { ref, onMounted } from 'vue';
import { useI18n } from 'vue-i18n';
import api from '../services/api';
import PageHeader from '../components/PageHeader.vue';
import Button from 'primevue/button';
import Tag from 'primevue/tag';

const { t } = useI18n();

const stats = ref({
  emails: 0,
  accounts: 0,
  proxies: 0,
  channels: 0
});

const loading = ref(false);
const refreshing = ref(false);
const recentLogs = ref<any[]>([]);

const loadData = async () => {
  loading.value = true;
  try {
    const [emailsRes, accountsRes, proxiesRes, channelsRes, logsRes] = await Promise.all([
      api.get('/emails', { params: { page: 1, pageSize: 1 } }),
      api.get('/accounts'),
      api.get('/proxies'),
      api.get('/channels'),
      api.get('/logs')
    ]);

    stats.value = {
      emails: emailsRes.data.total || emailsRes.data?.data?.length || 0,
      accounts: accountsRes.data?.length || 0,
      proxies: proxiesRes.data?.length || 0,
      channels: channelsRes.data?.length || 0
    };

    recentLogs.value = (logsRes.data || []).slice(0, 5);
  } catch (error) {
    console.error(error);
  } finally {
    loading.value = false;
    refreshing.value = false;
  }
};

const refresh = async () => {
  refreshing.value = true;
  await loadData();
};

onMounted(loadData);
</script>

<template>
  <div class="page">
    <PageHeader :title="t('dashboard.title')">
      <template #actions>
        <Button icon="pi pi-refresh" :aria-label="t('common.refresh')" text rounded :loading="refreshing" @click="refresh" v-tooltip.bottom="t('common.refresh')" />
      </template>
    </PageHeader>

    <div class="page-panel">
      <div class="grid grid-cols-1 md:grid-cols-4 gap-4 mb-6">
        <div class="stat-card">
          <div class="label">{{ t('dashboard.emails') }}</div>
          <div class="value">{{ stats.emails }}</div>
        </div>
        <div class="stat-card">
          <div class="label">{{ t('dashboard.accounts') }}</div>
          <div class="value">{{ stats.accounts }}</div>
        </div>
        <div class="stat-card">
          <div class="label">{{ t('dashboard.proxies') }}</div>
          <div class="value">{{ stats.proxies }}</div>
        </div>
        <div class="stat-card">
          <div class="label">{{ t('dashboard.channels') }}</div>
          <div class="value">{{ stats.channels }}</div>
        </div>
      </div>

      <div class="flex flex-col gap-4 flex-1 min-h-0">
        <div class="section-header">
          <h3>{{ t('dashboard.recent_logs') }}</h3>
        </div>
        <div class="dashboard-logs-container">
          <div v-if="loading" class="text-sm text-surface-500">{{ t('dashboard.loading') }}</div>
          <div v-else-if="recentLogs.length === 0" class="text-sm text-surface-500">{{ t('dashboard.no_logs') }}</div>
          <div v-else class="dashboard-logs-content">
            <div v-for="(log, index) in recentLogs" :key="index" class="log-row">
              <div class="log-meta">
                <Tag :value="log.status" severity="info" />
                <span class="time">{{ new Date(log.received_at).toLocaleString() }}</span>
              </div>
              <div class="log-content">
                <div class="subject">{{ log.subject || '-' }}</div>
                <div class="from text-sm text-surface-500">{{ log.from }}</div>
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.stat-card {
  background: var(--p-surface-0);
  border: 1px solid var(--p-surface-200);
  border-radius: 10px;
  padding: 1rem 1.25rem;
  display: flex;
  flex-direction: column;
  gap: 0.35rem;
  box-shadow: 0 1px 4px rgba(0, 0, 0, 0.04);
}

.app-dark .stat-card {
  background: var(--p-surface-900);
  border-color: var(--p-surface-800);
}

.stat-card .label {
  color: var(--p-text-muted-color);
  font-size: 0.95rem;
}

.stat-card .value {
  font-weight: 700;
  font-size: 1.5rem;
}

.section-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
}

.section-header h3 {
  margin: 0;
}

.log-row {
  border: 1px solid var(--p-surface-200);
  border-radius: 10px;
  padding: 0.75rem 1rem;
}

.app-dark .log-row {
  border-color: var(--p-surface-800);
}

.log-meta {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  margin-bottom: 0.25rem;
}

.log-meta .time {
  color: var(--p-text-muted-color);
  font-size: 0.85rem;
}

.log-content .subject {
  font-weight: 600;
  margin-bottom: 0.25rem;
}

/* Dashboard logs container for scroll */
.dashboard-logs-container {
  flex: 1;
  min-height: 0;
  overflow: auto;
}

.dashboard-logs-content {
  display: flex;
  flex-direction: column;
  gap: 0.75rem;
}
</style>
