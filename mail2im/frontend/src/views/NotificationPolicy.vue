<script setup lang="ts">
import { ref, onMounted } from 'vue';
import { useI18n } from 'vue-i18n';
import { useToast } from 'primevue/usetoast';
import api from '../services/api';
import PageHeader from '../components/PageHeader.vue';
import Button from 'primevue/button';
import Checkbox from 'primevue/checkbox';
import Select from 'primevue/select';
import Tag from 'primevue/tag';

const { t } = useI18n();
const toast = useToast();

interface PolicyChannel {
  id: number;
  name: string;
  type: string;
  selected: boolean;
}

interface PolicyItem {
  ID: number;
  key: string;
  name: string;
  priority: number;
  is_system: boolean;
  action: string;
  channel_ids: string;
  channels: PolicyChannel[];
}

const items = ref<PolicyItem[]>([]);
const loading = ref(false);
const saving = ref<string | null>(null);

const actionOptions = [
  { label: () => t('policy.action_notify'), value: 'notify' },
  { label: () => t('policy.action_silent'), value: 'silent' },
  { label: () => t('policy.action_ignore'), value: 'ignore' },
];

const fetchPolicy = async () => {
  loading.value = true;
  try {
    const res = await api.get('/notification-policy');
    items.value = res.data;
  } catch {
    toast.add({ severity: 'error', summary: t('common.error'), detail: t('policy.load_error'), life: 3000 });
  } finally {
    loading.value = false;
  }
};

const updatePolicy = async (item: PolicyItem) => {
  saving.value = item.key;
  try {
    // Build channel_ids from selected channels
    const selectedIds = item.channels.filter(c => c.selected).map(c => c.id);
    await api.put(`/notification-policy/${item.key}`, {
      channel_ids: JSON.stringify(selectedIds),
      action: item.action,
      priority: item.priority,
    });
    toast.add({ severity: 'success', summary: t('common.success'), detail: t('policy.update_success'), life: 2000 });
  } catch {
    toast.add({ severity: 'error', summary: t('common.error'), detail: t('policy.update_error'), life: 3000 });
  } finally {
    saving.value = null;
  }
};

const toggleChannel = (item: PolicyItem, channel: PolicyChannel) => {
  channel.selected = !channel.selected;
  updatePolicy(item);
};

const changeAction = (item: PolicyItem, newAction: string) => {
  item.action = newAction;
  updatePolicy(item);
};

const typeIcon = (key: string) => {
  const icons: Record<string, string> = {
    primary: 'pi pi-inbox',
    bill: 'pi pi-wallet',
    important: 'pi pi-star',
    notification: 'pi pi-bell',
    unknown: 'pi pi-question-circle',
    promotion: 'pi pi-megaphone',
    social: 'pi pi-users',
    spam: 'pi pi-ban',
    trash: 'pi pi-trash',
    draft: 'pi pi-file-edit',
    sent: 'pi pi-send',
  };
  return icons[key] || 'pi pi-tag';
};

const priorityLabel = (p: number) => {
  if (p >= 20) return t('common.priority_high');
  if (p >= 10) return t('common.priority_normal');
  return t('common.priority_low');
};

onMounted(fetchPolicy);
</script>

<template>
  <div class="page table-page">
    <PageHeader :title="t('policy.title')" :subtitle="t('policy.subtitle')">
      <template #actions>
        <Button :aria-label="t('common.refresh')" icon="pi pi-refresh" rounded text :loading="loading" @click="fetchPolicy" v-tooltip.bottom="t('common.refresh')" />
      </template>
    </PageHeader>

    <div class="page-panel policy-panel">
      <div v-if="loading" class="flex items-center justify-center p-8">
        <i class="pi pi-spin pi-spinner text-2xl" />
      </div>

      <table v-else class="policy-table">
        <thead>
          <tr>
            <th class="col-type">{{ t('policy.mail_type') }}</th>
            <th class="col-priority">{{ t('policy.priority') }}</th>
            <th class="col-channels">{{ t('policy.channels') }}</th>
            <th class="col-action">{{ t('policy.action') }}</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="item in items" :key="item.key" :class="{ 'row-muted': item.action === 'ignore' || item.action === 'silent' }">
            <td class="col-type">
              <div class="flex items-center gap-2">
                <i :class="typeIcon(item.key)" />
                <span class="font-medium">{{ item.name }}</span>
                <Tag v-if="item.is_system" value="System" severity="secondary" class="text-xs" />
              </div>
            </td>
            <td class="col-priority">
              <span class="text-sm">{{ priorityLabel(item.priority) }}</span>
            </td>
            <td class="col-channels">
              <div v-if="item.action === 'notify'" class="flex flex-wrap gap-2">
                <div
                  v-for="ch in item.channels"
                  :key="ch.id"
                  class="channel-check flex items-center gap-1"
                >
                  <Checkbox
                    :modelValue="ch.selected"
                    :binary="true"
                    @update:modelValue="toggleChannel(item, ch)"
                  />
                  <span class="text-sm">{{ ch.name }}</span>
                </div>
                <span v-if="item.channels.length === 0" class="text-sm text-surface-400">{{ t('policy.no_channels') }}</span>
              </div>
              <span v-else class="text-sm text-surface-400">--</span>
            </td>
            <td class="col-action">
              <Select
                :modelValue="item.action"
                :options="actionOptions"
                :optionLabel="(o: any) => o.label()"
                optionValue="value"
                class="action-select"
                @update:modelValue="(v: string) => changeAction(item, v)"
              />
              <i v-if="saving === item.key" class="pi pi-spin pi-spinner ml-2 text-sm" />
            </td>
          </tr>
        </tbody>
      </table>
    </div>
  </div>
</template>

<style scoped>
.policy-panel {
  overflow-x: auto;
}

.policy-table {
  width: 100%;
  border-collapse: collapse;
}

.policy-table th {
  text-align: left;
  padding: 0.75rem 1rem;
  font-weight: 600;
  font-size: 0.875rem;
  color: var(--p-text-muted-color);
  border-bottom: 2px solid var(--p-surface-200);
}

.policy-table td {
  padding: 0.75rem 1rem;
  border-bottom: 1px solid var(--p-surface-100);
  vertical-align: middle;
}

.row-muted {
  opacity: 0.6;
}

.col-type {
  min-width: 180px;
}

.col-priority {
  width: 100px;
}

.col-channels {
  min-width: 250px;
}

.col-action {
  width: 160px;
}

.action-select {
  width: 130px;
}

.channel-check {
  cursor: pointer;
}

:deep(.app-dark) .policy-table th {
  border-bottom-color: var(--p-surface-600);
}

:deep(.app-dark) .policy-table td {
  border-bottom-color: var(--p-surface-700);
}
</style>
