<script setup lang="ts">
import { ref, onMounted } from 'vue';
import api from '../services/api';
import DataTable from 'primevue/datatable';
import Column from 'primevue/column';
import Tag from 'primevue/tag';
import Button from 'primevue/button';
import Dialog from 'primevue/dialog';
import { format } from 'date-fns';
import { useI18n } from 'vue-i18n';
import { useToast } from 'primevue/usetoast';
import PageHeader from '../components/PageHeader.vue';
import { getNumber, setString, KEYS } from '../utils/storage';

const { t } = useI18n();
const toast = useToast();

const logs = ref([]);
const loading = ref(false);
const detailVisible = ref(false);
const detailLog = ref<any>(null);
const deleteLogDialog = ref(false);
const deleteTarget = ref<any>(null);
const deleteLoading = ref(false);
const clearLogsDialog = ref(false);
const clearLoading = ref(false);
const rowsOptions = [10, 20, 50];
const logRows = ref(getNumber(KEYS.TABLE_ROWS_LOGS, 10));
const logFirst = ref(0);

const fetchLogs = async () => {
  loading.value = true;
  try {
    const res = await api.get('/logs');
    logs.value = res.data;
  } catch (e) {
    console.error(e);
  } finally {
    loading.value = false;
  }
};

const getSeverity = (status: string) => {
  switch (status) {
    case 'success': return 'success';
    case 'failed': return 'danger';
    case 'received': return 'info';
    default: return 'info';
  }
};

const formatDate = (dateStr: string) => {
  return format(new Date(dateStr), 'yyyy-MM-dd HH:mm:ss');
};

const getPrioritySeverity = (priority: number) => {
  if (priority >= 3) return 'danger';
  if (priority === 2) return 'warning';
  if (priority === 1) return 'info';
  return 'secondary';
};

const getPriorityLabel = (priority: number) => {
  switch (priority) {
    case 3: return t('channels.priority_critical');
    case 2: return t('channels.priority_high');
    case 1: return t('channels.priority_normal');
    default: return t('channels.priority_low');
  }
};

const showDetail = (row: any) => {
  detailLog.value = row;
  detailVisible.value = true;
};

const confirmDeleteLog = (row: any) => {
  deleteTarget.value = row;
  deleteLogDialog.value = true;
};

const deleteLog = async () => {
  if (!deleteTarget.value) return;
  deleteLoading.value = true;
  try {
    const id = deleteTarget.value.id || deleteTarget.value.ID;
    await api.delete(`/logs/${id}`);
    toast.add({ severity: 'success', summary: t('common.success'), detail: t('logs.delete_success'), life: 2500 });
    deleteLogDialog.value = false;
    await fetchLogs();
  } catch (e) {
    console.error(e);
    toast.add({ severity: 'error', summary: t('common.error'), detail: t('logs.delete_error'), life: 2500 });
  } finally {
    deleteLoading.value = false;
  }
};

const confirmClearLogs = () => {
  clearLogsDialog.value = true;
};

const clearLogs = async () => {
  clearLoading.value = true;
  try {
    await api.delete('/logs');
    toast.add({ severity: 'success', summary: t('common.success'), detail: t('logs.clear_success'), life: 2500 });
    clearLogsDialog.value = false;
    await fetchLogs();
  } catch (e) {
    console.error(e);
    toast.add({ severity: 'error', summary: t('common.error'), detail: t('logs.clear_error'), life: 2500 });
  } finally {
    clearLoading.value = false;
  }
};

const onPage = (event: any) => {
  logRows.value = event.rows;
  logFirst.value = event.first;
  setString(KEYS.TABLE_ROWS_LOGS, String(event.rows));
};

onMounted(fetchLogs);

const tablePt = {
  mask: { style: { background: 'transparent', boxShadow: 'none', opacity: 1, pointerEvents: 'none' } },
  loadingOverlay: { style: { background: 'transparent', boxShadow: 'none' } }
};
</script>

<template>
  <div class="page table-page">
    <PageHeader :title="t('logs.title')">
      <template #actions>
        <Button icon="pi pi-trash" severity="danger" text rounded @click="confirmClearLogs" :aria-label="t('logs.clear')" v-tooltip.bottom="t('logs.clear')" />
        <Button icon="pi pi-refresh" @click="fetchLogs" :loading="loading" rounded text :aria-label="t('common.refresh')" v-tooltip.bottom="t('common.refresh')" />
      </template>
    </PageHeader>

    <div class="page-panel table-panel">
      <div class="table-wrapper">
        <DataTable
          :value="logs"
          paginator
          :rows="logRows"
          :first="logFirst"
          :loading="loading"
          stripedRows
          tableStyle="min-width: 60rem"
          scrollable
          scrollHeight="flex"
          class="table-fill"
          :pt="tablePt"
          :rowsPerPageOptions="rowsOptions"
          paginatorTemplate="FirstPageLink PrevPageLink PageLinks NextPageLink LastPageLink CurrentPageReport RowsPerPageDropdown"
          currentPageReportTemplate="{first} to {last} of {totalRecords}"
          @page="onPage"
        >
          <Column field="priority" :header="t('logs.priority')" style="width: 8rem">
            <template #body="slotProps">
              <Tag :value="getPriorityLabel(slotProps.data.priority)" :severity="getPrioritySeverity(slotProps.data.priority)" />
            </template>
          </Column>
          <Column field="status" :header="t('logs.status')">
            <template #body="slotProps">
              <Tag :value="slotProps.data.status" :severity="getSeverity(slotProps.data.status)" />
            </template>
          </Column>
          <Column field="action" :header="t('logs.action')">
            <template #body="slotProps">
              {{ slotProps.data.action || '-' }}
            </template>
          </Column>
          <Column :header="t('logs.channel')">
            <template #body="slotProps">
              <div class="flex flex-column gap-1">
                <span class="font-medium">{{ slotProps.data.channel_name || slotProps.data.channel || '-' }}</span>
                <small v-if="slotProps.data.channel_id" class="text-color-secondary">#{{ slotProps.data.channel_id }}</small>
              </div>
            </template>
          </Column>
          <Column field="received_at" :header="t('logs.time')">
            <template #body="slotProps">
              {{ formatDate(slotProps.data.received_at || slotProps.data.forwarded_at) }}
            </template>
          </Column>
          <Column field="from" :header="t('logs.from')"></Column>
          <Column field="subject" :header="t('logs.subject')"></Column>
          <Column field="error" :header="t('logs.error')">
            <template #body="slotProps">
              <span v-if="slotProps.data.error" class="text-red-400">{{ slotProps.data.error }}</span>
              <span v-else>-</span>
            </template>
          </Column>
          <Column :header="t('logs.details')" style="width: 10rem" :exportable="false">
            <template #body="slotProps">
              <Button icon="pi pi-search" text rounded class="mr-2" @click="showDetail(slotProps.data)" :aria-label="t('logs.details')" />
              <Button icon="pi pi-trash" text rounded severity="danger" @click="confirmDeleteLog(slotProps.data)" :aria-label="t('logs.delete')" />
            </template>
          </Column>
        </DataTable>
      </div>
    </div>

    <Dialog v-model:visible="detailVisible" modal :header="t('logs.details')" :style="{ width: '40rem' }">
      <div v-if="detailLog" class="log-detail">
        <div class="detail-row">
          <span class="label">{{ t('logs.action') }}</span>
          <span>{{ detailLog.action || '-' }}</span>
        </div>
        <div class="detail-row">
          <span class="label">{{ t('logs.status') }}</span>
          <Tag :value="detailLog.status" :severity="getSeverity(detailLog.status)" />
        </div>
        <div class="detail-row">
          <span class="label">{{ t('logs.priority') }}</span>
          <Tag :value="getPriorityLabel(detailLog.priority)" :severity="getPrioritySeverity(detailLog.priority)" />
        </div>
        <div class="detail-row">
          <span class="label">{{ t('logs.channel') }}</span>
          <span>{{ detailLog.channel_name || detailLog.channel || '-' }} <span v-if="detailLog.channel_id" class="text-color-secondary">(#{{ detailLog.channel_id }})</span></span>
        </div>
        <div class="detail-row">
          <span class="label">{{ t('logs.from') }}</span>
          <span>{{ detailLog.from || '-' }}</span>
        </div>
        <div class="detail-row">
          <span class="label">{{ t('logs.subject') }}</span>
          <span>{{ detailLog.subject || '-' }}</span>
        </div>
        <div class="detail-row">
          <span class="label">{{ t('logs.message_id') }}</span>
          <span>{{ detailLog.message_id || '-' }}</span>
        </div>
        <div class="detail-row">
          <span class="label">{{ t('logs.time') }}</span>
          <span>{{ formatDate(detailLog.received_at || detailLog.forwarded_at) }}</span>
        </div>
        <div class="detail-block" v-if="detailLog.request">
          <div class="block-title">{{ t('logs.request') }}</div>
          <pre>{{ detailLog.request }}</pre>
        </div>
        <div class="detail-block" v-if="detailLog.response">
          <div class="block-title">{{ t('logs.response') }}</div>
          <pre>{{ detailLog.response }}</pre>
        </div>
        <div class="detail-block" v-if="detailLog.error">
          <div class="block-title text-red-500">{{ t('logs.error') }}</div>
          <pre class="text-red-500">{{ detailLog.error }}</pre>
        </div>
      </div>
    </Dialog>

    <Dialog v-model:visible="deleteLogDialog" modal :header="t('common.confirm')" :style="{ width: '26rem' }">
      <p class="mb-4">{{ t('logs.delete_confirm') }}</p>
      <div class="flex justify-end gap-2">
        <Button :label="t('common.no')" icon="pi pi-times" text @click="deleteLogDialog = false" />
        <Button :label="t('common.yes')" icon="pi pi-check" :loading="deleteLoading" severity="danger" @click="deleteLog" />
      </div>
    </Dialog>

    <Dialog v-model:visible="clearLogsDialog" modal :header="t('common.confirm')" :style="{ width: '28rem' }">
      <p class="mb-4">{{ t('logs.clear_confirm') }}</p>
      <div class="flex justify-end gap-2">
        <Button :label="t('common.no')" icon="pi pi-times" text @click="clearLogsDialog = false" />
        <Button :label="t('common.yes')" icon="pi pi-check" :loading="clearLoading" severity="danger" @click="clearLogs" />
      </div>
    </Dialog>
  </div>
</template>

<style scoped>
.log-detail {
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
}

.detail-row {
  display: flex;
  align-items: center;
  gap: 0.75rem;
  font-size: 0.95rem;
}

.detail-row .label {
  min-width: 7rem;
  color: var(--text-color-secondary, #6b7280);
  font-weight: 600;
}

.detail-block {
  margin-top: 0.25rem;
  padding: 0.75rem;
  background: #f8fafc;
  border: 1px solid #e5e7eb;
  border-radius: 8px;
}

.block-title {
  font-weight: 600;
  margin-bottom: 0.25rem;
}

pre {
  white-space: pre-wrap;
  word-break: break-all;
  margin: 0;
  font-family: Menlo, Monaco, Consolas, 'Courier New', monospace;
}
</style>
