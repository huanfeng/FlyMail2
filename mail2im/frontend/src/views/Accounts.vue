<script setup lang="ts">
import { ref, onMounted, computed } from 'vue';
import api from '../services/api';
import { useToast } from 'primevue/usetoast';
import DataTable from 'primevue/datatable';
import Column from 'primevue/column';
import Button from 'primevue/button';
import Dialog from 'primevue/dialog';
import InputText from 'primevue/inputtext';
import InputNumber from 'primevue/inputnumber';
import { getNumber, setString, KEYS } from '../utils/storage';
import Select from 'primevue/select';
import Tag from 'primevue/tag';
import PageHeader from '../components/PageHeader.vue';
import ToggleSwitch from 'primevue/toggleswitch';
import Tabs from 'primevue/tabs';
import TabList from 'primevue/tablist';
import Tab from 'primevue/tab';
import TabPanels from 'primevue/tabpanels';
import TabPanel from 'primevue/tabpanel';
import { useI18n } from 'vue-i18n';
import styles from './Accounts.module.css';

const { t } = useI18n();
const toast = useToast();
const accounts = ref<any[]>([]);
const proxies = ref<any[]>([]);
const providers = ref<any[]>([]);
const mailboxes = ref<any[]>([]);
const loading = ref(false);
const mailboxesLoading = ref(false);
const rowsOptions = [10, 20, 50];
const accountRows = ref(getNumber(KEYS.TABLE_ROWS_ACCOUNTS, 10));
const accountFirst = ref(0);

const accountDialog = ref(false);
const deleteAccountDialog = ref(false);
const batchDialog = ref(false);
const isEdit = ref(false);
const changePassword = ref(false);
const testing = ref(false);
const saving = ref(false);
const connectionStatus = ref<{ severity: 'success' | 'warn' | 'error' | null; message: string; details?: string[] } | null>(null);

const form = ref<any>({
  id: null,
  display_name: '',
  email: '',
  login: '',
  password: '',
  provider: '',
  imap_server: '',
  imap_port: 993,
  ssl_mode: 'ssl',
  proxy_id: null,
  use_idle: true,
  poll_interval_day: 60,
  poll_interval_night: 300,
  timezone: 'UTC',
  enabled: true
});

const watchModes = computed(() => [
  { label: t('accounts.watch_mode_idle'), value: 'idle' },
  { label: t('accounts.watch_mode_poll'), value: 'poll' },
  { label: t('accounts.watch_mode_none'), value: 'none' }
]);

const folderTypes = computed(() => [
  { label: t('emails_view.type_primary'), value: 'primary' },
  { label: t('emails_view.type_bill'), value: 'bill' },
  { label: t('emails_view.type_notification'), value: 'notification' },
  { label: t('emails_view.type_promotion'), value: 'promotion' },
  { label: t('emails_view.type_social'), value: 'social' },
  { label: t('emails_view.type_spam'), value: 'spam' },
  { label: t('emails_view.type_trash'), value: 'trash' },
  { label: t('emails_view.type_sent'), value: 'sent' },
  { label: t('emails_view.type_draft'), value: 'draft' },
  { label: t('emails_view.type_unknown'), value: 'unknown' }
]);

const batchConfig = ref<any>({
  provider: '',
  imap_server: '',
  imap_port: 993,
  ssl_mode: 'ssl',
  proxy_id: null,
  use_idle: true,
  poll_interval_day: 60,
  poll_interval_night: 300,
  timezone: 'UTC'
});

const batchAccounts = ref<any[]>([{ display_name: '', email: '', login: '', password: '' }]);

const sslModes = computed(() => [
  { label: t('accounts.ssl_ssl'), value: 'ssl' },
  { label: t('accounts.ssl_starttls'), value: 'starttls' },
  { label: t('accounts.ssl_none'), value: 'none' }
]);

const getStatusSeverity = (status: string) => {
  switch (status) {
    case 'Active': return 'success';
    case 'AuthFailed': return 'danger';
    case 'NetworkError': return 'warn';
    default: return 'info';
  }
};

const getStatusLabel = (status: string) => {
  switch (status) {
    case 'Active':
      return t('accounts.status_active');
    case 'AuthFailed':
      return t('accounts.status_auth_failed');
    case 'NetworkError':
      return t('accounts.status_network_error');
    default:
      return t('accounts.status_unknown');
  }
};

const fetchProviders = async () => {
  try {
    const res = await api.get('/providers');
    providers.value = res.data.providers || [];
  } catch (error) {
    console.error(error);
  }
};

const applyProviderDefaults = (target: any) => {
  const provider = providers.value.find((p) => p.id === target.provider);
  if (!provider) return;

  const server = provider.servers?.[target.ssl_mode];
  if (server?.host) target.imap_server = server.host;
  if (server?.port) target.imap_port = server.port;
};

const fetchMailboxes = async (accountId: number) => {
  if (!accountId) return;
  mailboxesLoading.value = true;
  try {
    const res = await api.get(`/accounts/${accountId}/mailboxes`);
    mailboxes.value = res.data;
  } catch (error) {
    console.error(error);
    toast.add({ severity: 'error', summary: t('common.error'), detail: t('accounts.load_folders_error'), life: 3000 });
  } finally {
    mailboxesLoading.value = false;
  }
};

const syncMailboxes = async () => {
  if (!form.value.id) return;
  mailboxesLoading.value = true;
  try {
    const res = await api.post(`/accounts/${form.value.id}/mailboxes/sync`);
    mailboxes.value = res.data;
    toast.add({ severity: 'success', summary: t('common.success'), detail: t('accounts.sync_folders_success'), life: 3000 });
  } catch (error) {
    toast.add({ severity: 'error', summary: t('common.error'), detail: t('accounts.sync_folders_error'), life: 3000 });
  } finally {
    mailboxesLoading.value = false;
  }
};

const updateMailbox = async (mb: any) => {
  try {
    await api.put(`/mailboxes/${mb.ID}`, {
      watch_mode: mb.watch_mode,
      type: mb.type
    });
    toast.add({ severity: 'success', summary: t('common.success'), detail: t('accounts.update_folder_success'), life: 1000 });
  } catch (error) {
    toast.add({ severity: 'error', summary: t('common.error'), detail: t('accounts.update_folder_error'), life: 3000 });
  }
};

const fetchAccounts = async () => {
  loading.value = true;
  try {
    const res = await api.get('/accounts');
    accounts.value = res.data;
  } catch (error) {
    console.error(error);
    toast.add({ severity: 'error', summary: t('common.error'), detail: t('accounts.load_error'), life: 3000 });
  } finally {
    loading.value = false;
  }
};

const fetchProxies = async () => {
  try {
    const res = await api.get('/proxies');
    proxies.value = res.data.map((p: any) => ({ label: p.name, value: p.ID }));
  } catch (error) {
    console.error(error);
  }
};

const resetForm = () => {
  form.value = {
    id: null,
    display_name: '',
    email: '',
    login: '',
    password: '',
    provider: providers.value[0]?.id || '',
    imap_server: '',
    imap_port: 993,
    ssl_mode: 'ssl',
    proxy_id: null,
    use_idle: true,
  poll_interval_day: 60,
  poll_interval_night: 300,
  timezone: 'UTC',
  enabled: true
};
  changePassword.value = false;
  applyProviderDefaults(form.value);
};

const openCreate = () => {
  isEdit.value = false;
  resetForm();
  connectionStatus.value = null;
  accountDialog.value = true;
};

const openEdit = async (row: any) => {
  isEdit.value = true;
  changePassword.value = false;
  connectionStatus.value = null;
  mailboxes.value = [];
  try {
    const res = await api.get(`/accounts/${row.ID}`);
    const acc = res.data;
    form.value = {
      id: acc.ID,
      display_name: acc.display_name,
      email: acc.email,
      login: acc.login,
      password: '',
      provider: acc.provider,
      imap_server: acc.imap_server,
      imap_port: acc.imap_port,
      ssl_mode: acc.ssl_mode || (acc.use_ssl ? 'ssl' : 'none'),
      proxy_id: acc.proxy_id,
      use_idle: acc.use_idle,
      poll_interval_day: acc.poll_interval_day,
      poll_interval_night: acc.poll_interval_night,
      timezone: acc.timezone || 'UTC',
      enabled: typeof acc.enabled === 'boolean' ? acc.enabled : true
    };
    fetchMailboxes(acc.ID);
  } catch (error) {
    toast.add({ severity: 'error', summary: t('common.error'), detail: t('accounts.load_error'), life: 3000 });
    return;
  }
    accountDialog.value = true;
};

const hideDialog = () => {
    accountDialog.value = false;
    connectionStatus.value = null;
};

const onProviderChange = (target: any) => {
  applyProviderDefaults(target);
};

const testConnection = async () => {
  testing.value = true;
  connectionStatus.value = { severity: 'warn', message: t('accounts.testing') };
  try {
    if (!form.value.password && (!isEdit.value || changePassword.value)) {
      toast.add({ severity: 'warn', summary: t('common.warning'), detail: t('accounts.password_required'), life: 2500 });
      testing.value = false;
      return;
    }
    if (isEdit.value && !changePassword.value) {
      toast.add({ severity: 'warn', summary: t('common.warning'), detail: t('accounts.password_change_hint'), life: 2500 });
      testing.value = false;
      return;
    }
    const res = await api.post('/accounts/test', { ...form.value });
    const data = res.data || {};
    const details: string[] = [];
    if (data.security) {
      details.push(t('accounts.test_security', { mode: data.security }));
    }
    if (typeof data.latency_ms !== 'undefined') {
      details.push(t('accounts.test_latency', { ms: data.latency_ms }));
    }
    if (typeof data.supports_idle !== 'undefined') {
      details.push(t('accounts.test_idle', { status: data.supports_idle ? t('common.yes') : t('common.no') }));
    }
    if (data.capabilities && Array.isArray(data.capabilities) && data.capabilities.length) {
      details.push(t('accounts.test_capabilities', { caps: data.capabilities.join(', ') }));
    }
    toast.add({ severity: 'success', summary: t('common.success'), detail: data.message || t('accounts.test_success'), life: 2000 });
    connectionStatus.value = { severity: 'success', message: data.message || t('accounts.test_success'), details };
  } catch (error: any) {
    toast.add({ severity: 'error', summary: t('common.error'), detail: error.response?.data?.error || 'Failed', life: 3000 });
    connectionStatus.value = { severity: 'error', message: error.response?.data?.error || 'Failed' };
  } finally {
    testing.value = false;
  }
};

const saveAccount = async () => {
  if (!isEdit.value && !form.value.password) {
    toast.add({ severity: 'warn', summary: t('common.warning'), detail: t('accounts.password_required'), life: 2500 });
    return;
  }
  if (isEdit.value && changePassword.value && !form.value.password) {
    toast.add({ severity: 'warn', summary: t('common.warning'), detail: t('accounts.password_required'), life: 2500 });
    return;
  }

  saving.value = true;
  try {
    const payload = { ...form.value };
    if (isEdit.value && !changePassword.value) {
      payload.password = '';
    }
    if (!payload.login) {
      payload.login = payload.email;
    }

    if (isEdit.value && form.value.id) {
      await api.put(`/accounts/${form.value.id}`, payload);
      toast.add({ severity: 'success', summary: t('common.success'), detail: t('accounts.update_success'), life: 3000 });
    } else {
      await api.post('/accounts', payload);
      toast.add({ severity: 'success', summary: t('common.success'), detail: t('accounts.create_success'), life: 3000 });
    }
    accountDialog.value = false;
    fetchAccounts();
  } catch (error) {
    const detail = (error as any).response?.data?.error || t('accounts.create_error');
    toast.add({ severity: 'error', summary: t('common.error'), detail, life: 3000 });
  } finally {
    saving.value = false;
  }
};

const toggleEnabled = async (row: any, value: boolean) => {
  const id = row.ID || row.id;
  if (!id) return;
  const previous = row.enabled;
  row.enabled = value;
  const payload = {
    email: row.email,
    display_name: row.display_name,
    login: row.login || row.email,
    password: '',
    provider: row.provider,
    imap_server: row.imap_server,
    imap_port: row.imap_port,
    ssl_mode: row.ssl_mode || (row.use_ssl ? 'ssl' : 'none'),
    proxy_id: row.proxy_id,
    use_idle: row.use_idle,
    poll_interval_day: row.poll_interval_day,
    poll_interval_night: row.poll_interval_night,
    timezone: row.timezone,
    auth_type: row.auth_type,
    enabled: value
  };

  try {
    await api.put(`/accounts/${id}`, payload);
    toast.add({ severity: 'success', summary: t('common.success'), detail: t('accounts.update_success'), life: 2500 });
    fetchAccounts();
  } catch (error) {
    row.enabled = previous;
    toast.add({ severity: 'error', summary: t('common.error'), detail: t('accounts.update_error'), life: 3000 });
  }
};

const confirmDeleteAccount = (acc: any) => {
  deleteAccountDialog.value = true;
  form.value.id = acc.ID;
  form.value.email = acc.email;
};

const deleteAccount = async () => {
  try {
    await api.delete(`/accounts/${form.value.id}`);
    deleteAccountDialog.value = false;
    toast.add({ severity: 'success', summary: t('common.success'), detail: t('accounts.delete_success'), life: 3000 });
    fetchAccounts();
  } catch (error) {
    toast.add({ severity: 'error', summary: t('common.error'), detail: t('accounts.delete_error'), life: 3000 });
  }
};

const openBatch = () => {
  batchDialog.value = true;
  batchAccounts.value = [{ display_name: '', email: '', login: '', password: '' }];
  batchConfig.value.provider = providers.value[0]?.id || '';
  batchConfig.value.ssl_mode = 'ssl';
  batchConfig.value.imap_server = '';
  batchConfig.value.imap_port = 993;
  batchConfig.value.proxy_id = null;
  batchConfig.value.use_idle = true;
  applyProviderDefaults(batchConfig.value);
};

const addBatchRow = () => {
  batchAccounts.value.push({ display_name: '', email: '', login: '', password: '' });
};

const removeBatchRow = (idx: number) => {
  batchAccounts.value.splice(idx, 1);
};

const saveBatchAccounts = async () => {
  const payload = batchAccounts.value
    .filter((acc) => acc.email && acc.password)
    .map((acc) => ({
      email: acc.email,
      display_name: acc.display_name,
      login: acc.login || acc.email,
      password: acc.password,
      provider: batchConfig.value.provider,
      imap_server: batchConfig.value.imap_server,
      imap_port: batchConfig.value.imap_port,
      ssl_mode: batchConfig.value.ssl_mode,
      proxy_id: batchConfig.value.proxy_id,
      use_idle: batchConfig.value.use_idle,
      poll_interval_day: batchConfig.value.poll_interval_day,
      poll_interval_night: batchConfig.value.poll_interval_night,
      timezone: batchConfig.value.timezone,
      enabled: true
    }));

  if (payload.length === 0) {
    toast.add({ severity: 'warn', summary: t('common.warning'), detail: t('accounts.batch_empty'), life: 3000 });
    return;
  }

  try {
    await api.post('/accounts/batch', payload);
    toast.add({ severity: 'success', summary: t('common.success'), detail: t('accounts.create_success'), life: 3000 });
    batchDialog.value = false;
    fetchAccounts();
  } catch (error) {
    toast.add({ severity: 'error', summary: t('common.error'), detail: t('accounts.create_error'), life: 3000 });
  }
};

const onAccountPage = (event: any) => {
  accountRows.value = event.rows;
  accountFirst.value = event.first;
  setString(KEYS.TABLE_ROWS_ACCOUNTS, String(event.rows));
};

onMounted(() => {
  fetchProviders();
  fetchAccounts();
  fetchProxies();
});

const tablePt = {
  mask: { style: { background: 'transparent', boxShadow: 'none', opacity: 1, pointerEvents: 'none' } },
  loadingOverlay: { style: { background: 'transparent', boxShadow: 'none' } }
};
</script>

<template>
  <div class="page table-page">
    <PageHeader :title="t('menu.accounts')">
      <template #actions>
        <Button :label="t('accounts.add')" icon="pi pi-user-plus" class="mr-2" @click="openCreate" />
        <Button :label="t('accounts.batch_add')" icon="pi pi-plus" class="mr-2" @click="openBatch" />
        <Button :aria-label="t('common.refresh')" icon="pi pi-refresh" rounded text :loading="loading" @click="fetchAccounts" v-tooltip.bottom="t('common.refresh')" />
      </template>
    </PageHeader>

    <div class="page-panel table-panel">
      <div class="table-wrapper">
        <DataTable
          :value="accounts"
          :loading="loading"
          tableStyle="min-width: 50rem"
          paginator
          :rows="accountRows"
          :first="accountFirst"
          :rowsPerPageOptions="rowsOptions"
          paginatorTemplate="FirstPageLink PrevPageLink PageLinks NextPageLink LastPageLink CurrentPageReport RowsPerPageDropdown"
          currentPageReportTemplate="{first} to {last} of {totalRecords}"
          stripedRows
          scrollable
          scrollHeight="flex"
          class="table-fill"
          :pt="tablePt"
          @page="onAccountPage"
        >
          <Column field="display_name" :header="t('accounts.display_name')">
            <template #body="slotProps">
              {{ slotProps.data.display_name || '-' }}
            </template>
          </Column>
          <Column field="email" :header="t('accounts.email')"></Column>
          <Column field="provider" :header="t('accounts.provider')"></Column>
          <Column field="imap_server" :header="t('accounts.imap_server')"></Column>
          <Column field="enabled" :header="t('accounts.enabled')" style="width: 8rem">
            <template #body="slotProps">
              <ToggleSwitch :modelValue="slotProps.data.enabled !== false" @update:modelValue="(val: boolean) => toggleEnabled(slotProps.data, val)" />
            </template>
          </Column>
          <Column field="status" :header="t('accounts.status')">
            <template #body="slotProps">
              <Tag :value="getStatusLabel(slotProps.data.status)" :severity="getStatusSeverity(slotProps.data.status)" />
            </template>
          </Column>
          <Column field="last_sync_at" :header="t('accounts.last_sync')">
            <template #body="slotProps">
              {{ slotProps.data.last_sync_at ? new Date(slotProps.data.last_sync_at).toLocaleString() : '-' }}
            </template>
          </Column>
          <Column :exportable="false" style="min-width: 10rem">
            <template #body="slotProps">
              <Button icon="pi pi-pencil" outlined rounded class="mr-2" @click="openEdit(slotProps.data)" />
              <Button icon="pi pi-trash" outlined rounded severity="danger" @click="confirmDeleteAccount(slotProps.data)" />
            </template>
          </Column>
        </DataTable>
      </div>
    </div>

    <Dialog v-model:visible="accountDialog" :style="{ width: '800px' }" :header="isEdit ? t('accounts.edit') : t('accounts.add')" :modal="true" class="p-fluid">
      <Tabs value="0">
        <TabList>
            <Tab value="0">{{ t('accounts.basic_tab') }}</Tab>
            <Tab value="1" :disabled="!isEdit">{{ t('accounts.folders_tab') }}</Tab>
        </TabList>
        <TabPanels>
            <TabPanel value="0">
                <form id="accountForm" :class="styles.dialogBody" @submit.prevent="saveAccount">
                    <div :class="styles.section">
                    <h4>{{ t('accounts.section_basic') }}</h4>
                    <div :class="styles.formGrid">
                        <div :class="styles.formRow">
                        <label :class="styles.formLabel">{{ t('accounts.display_name') }}</label>
                        <InputText v-model="form.display_name" class="flex-1" />
                        </div>
                        <div :class="styles.formRow">
                        <label :class="styles.formLabel">{{ t('accounts.email') }}</label>
                        <InputText v-model="form.email" class="flex-1" />
                        </div>
                        <div :class="styles.formRow">
                        <label :class="styles.formLabel">{{ t('accounts.login') }}</label>
                        <InputText v-model="form.login" :placeholder="t('accounts.login_placeholder')" class="flex-1" />
                        </div>
                        <div :class="styles.formRow">
                        <label :class="styles.formLabel">{{ t('accounts.password') }}</label>
                        <div class="flex-1">
                            <InputText v-if="!isEdit || changePassword" type="password" v-model="form.password" class="w-full" />
                            <div v-else class="flex items-center gap-2">
                            <span class="text-sm text-surface-500">{{ t('accounts.password_hidden') }}</span>
                            <Button text size="small" @click="changePassword = true">{{ t('accounts.password_change') }}</Button>
                            </div>
                        </div>
                        </div>
                        <div :class="styles.formRow">
                        <label :class="styles.formLabel">{{ t('accounts.enabled') }}</label>
                        <div class="flex items-center gap-3 flex-1">
                            <ToggleSwitch v-model="form.enabled" />
                        </div>
                        </div>
                    </div>
                    </div>

                    <div :class="styles.section">
                    <h4>{{ t('accounts.section_server') }}</h4>
                    <div :class="styles.formGrid">
                        <div :class="styles.formRow">
                        <label :class="styles.formLabel">{{ t('accounts.provider') }}</label>
                        <Select v-model="form.provider" :options="providers" optionLabel="name" optionValue="id" class="flex-1" @change="onProviderChange(form)" />
                        </div>
                        <div :class="styles.formRow">
                        <label :class="styles.formLabel">{{ t('accounts.ssl_mode') }}</label>
                        <Select v-model="form.ssl_mode" :options="sslModes" optionLabel="label" optionValue="value" class="flex-1" @change="onProviderChange(form)" />
                        </div>
                        <div :class="styles.formRow">
                        <label :class="styles.formLabel">{{ t('accounts.imap_server') }}</label>
                        <InputText v-model="form.imap_server" class="flex-1" />
                        </div>
                        <div :class="styles.formRow">
                        <label :class="styles.formLabel">{{ t('accounts.port') }}</label>
                        <InputNumber v-model="form.imap_port" :useGrouping="false" class="flex-1" />
                        </div>
                    </div>
                    </div>

                    <div :class="styles.section">
                    <h4>{{ t('accounts.section_policy') }}</h4>
                    <div :class="styles.formGrid">
                        <div :class="styles.formRow">
                        <label :class="styles.formLabel">{{ t('accounts.proxy') }}</label>
                        <Select v-model="form.proxy_id" :options="proxies" optionLabel="label" optionValue="value" :placeholder="t('accounts.no_proxy')" showClear class="flex-1" />
                        </div>
                        <div :class="styles.formRow">
                        <label :class="styles.formLabel">{{ t('accounts.use_idle') }}</label>
                        <div class="flex items-center gap-3 flex-1">
                            <ToggleSwitch v-model="form.use_idle" />
                        </div>
                        </div>
                        <div :class="styles.formRow">
                        <label :class="styles.formLabel">{{ t('accounts.poll_day') }}</label>
                        <InputNumber v-model="form.poll_interval_day" :useGrouping="false" class="flex-1" />
                        </div>
                        <div :class="styles.formRow">
                        <label :class="styles.formLabel">{{ t('accounts.poll_night') }}</label>
                        <InputNumber v-model="form.poll_interval_night" :useGrouping="false" class="flex-1" />
                        </div>
                    </div>
                    </div>

                    <div v-if="connectionStatus" class="mt-3 space-y-2">
                    <Tag :severity="connectionStatus.severity || 'info'" :value="connectionStatus.message" />
                    <ul v-if="connectionStatus.details?.length" class="text-sm text-surface-600 list-disc ml-4 space-y-1">
                        <li v-for="(item, idx) in connectionStatus.details" :key="idx">{{ item }}</li>
                    </ul>
                    </div>
                </form>
            </TabPanel>
            <TabPanel value="1">
                <div class="flex justify-between items-center mb-4">
                    <span class="text-surface-600">{{ t('accounts.folders_desc') }}</span>
                    <Button :label="t('accounts.sync_folders')" icon="pi pi-sync" size="small" @click="syncMailboxes" :loading="mailboxesLoading" />
                </div>
                <DataTable :value="mailboxes" size="small" stripedRows scrollable scrollHeight="400px" :loading="mailboxesLoading">
                    <Column field="name" :header="t('accounts.folder_name')">
                        <template #body="slotProps">
                            <div class="flex flex-col">
                                <span class="font-medium">{{ slotProps.data.name }}</span>
                                <span class="text-xs text-surface-500">{{ slotProps.data.path }}</span>
                            </div>
                        </template>
                    </Column>
                    <Column :header="t('accounts.watch_mode')" style="width: 180px">
                        <template #body="slotProps">
                            <Select v-model="slotProps.data.watch_mode" :options="watchModes" optionLabel="label" optionValue="value" size="small" class="w-full" @change="updateMailbox(slotProps.data)" />
                        </template>
                    </Column>
                    <Column :header="t('accounts.folder_type')" style="width: 160px">
                        <template #body="slotProps">
                            <Select v-model="slotProps.data.type" :options="folderTypes" optionLabel="label" optionValue="value" size="small" class="w-full" @change="updateMailbox(slotProps.data)" />
                        </template>
                    </Column>
                    <Column :header="t('accounts.status')" style="width: 80px">
                         <template #body="slotProps">
                            <Tag :value="slotProps.data.watch_status" :severity="slotProps.data.watch_status === 'verified' ? 'success' : 'warn'" />
                        </template>
                    </Column>
                </DataTable>
            </TabPanel>
        </TabPanels>
      </Tabs>

      <template #footer>
        <div class="flex gap-2 justify-end w-full">
          <Button type="button" :label="t('accounts.test')" icon="pi pi-link" @click="testConnection" :loading="testing" outlined />
          <Button type="button" :label="t('common.cancel')" icon="pi pi-times" outlined severity="secondary" @click="hideDialog" />
          <Button type="submit" form="accountForm" :label="t('common.save')" icon="pi pi-check" :loading="saving" />
        </div>
      </template>
    </Dialog>

    <Dialog v-model:visible="batchDialog" :style="{ width: '820px' }" :header="t('accounts.batch_add')" :modal="true" class="p-fluid">
      <form id="batchForm" @submit.prevent="saveBatchAccounts">
        <div class="grid grid-cols-3 gap-4 mb-4">
        <div class="field">
          <label class="block mb-2">{{ t('accounts.provider') }}</label>
          <Select v-model="batchConfig.provider" :options="providers" optionLabel="name" optionValue="id" @change="onProviderChange(batchConfig)" />
        </div>
        <div class="field">
          <label class="block mb-2">{{ t('accounts.ssl_mode') }}</label>
          <Select v-model="batchConfig.ssl_mode" :options="sslModes" optionLabel="label" optionValue="value" @change="onProviderChange(batchConfig)" />
        </div>
        <div class="field">
          <label class="block mb-2">{{ t('accounts.proxy') }}</label>
          <Select v-model="batchConfig.proxy_id" :options="proxies" optionLabel="label" optionValue="value" :placeholder="t('accounts.no_proxy')" showClear />
        </div>
        <div class="field">
          <label class="block mb-2">{{ t('accounts.imap_server') }}</label>
          <InputText v-model="batchConfig.imap_server" />
        </div>
        <div class="field">
          <label class="block mb-2">{{ t('accounts.port') }}</label>
          <InputNumber v-model="batchConfig.imap_port" :useGrouping="false" />
        </div>
        <div class="field">
          <label class="block mb-2">{{ t('accounts.use_idle') }}</label>
          <ToggleSwitch v-model="batchConfig.use_idle" />
        </div>
        <div class="field">
          <label class="block mb-2">{{ t('accounts.poll_day') }}</label>
          <InputNumber v-model="batchConfig.poll_interval_day" :useGrouping="false" />
        </div>
        <div class="field">
          <label class="block mb-2">{{ t('accounts.poll_night') }}</label>
          <InputNumber v-model="batchConfig.poll_interval_night" :useGrouping="false" />
        </div>
      </div>

      <div class="space-y-4">
        <div v-for="(acc, idx) in batchAccounts" :key="idx" class="grid grid-cols-4 gap-3 items-end">
          <div class="field">
            <label class="block mb-1">{{ t('accounts.display_name') }}</label>
            <InputText v-model="acc.display_name" />
          </div>
          <div class="field">
            <label class="block mb-1">{{ t('accounts.email') }}</label>
            <InputText v-model="acc.email" />
          </div>
          <div class="field">
            <label class="block mb-1">{{ t('accounts.login') }}</label>
            <InputText v-model="acc.login" :placeholder="t('accounts.login_placeholder')" />
          </div>
          <div class="field">
            <label class="block mb-1">{{ t('accounts.password') }}</label>
            <div class="flex gap-2">
              <InputText v-model="acc.password" type="password" class="flex-1" />
              <Button icon="pi pi-trash" text severity="danger" @click="removeBatchRow(idx)" v-if="batchAccounts.length > 1" />
            </div>
          </div>
        </div>
        <Button icon="pi pi-plus" text @click="addBatchRow">{{ t('accounts.add_row') }}</Button>
        </div>
      </form>

      <template #footer>
        <Button type="button" :label="t('common.cancel')" icon="pi pi-times" text @click="batchDialog = false" />
        <Button type="submit" form="batchForm" :label="t('common.save')" icon="pi pi-check" />
      </template>
    </Dialog>

    <Dialog v-model:visible="deleteAccountDialog" :style="{ width: '450px' }" :header="t('common.confirm')" :modal="true">
      <div class="flex items-center gap-4">
        <i class="pi pi-exclamation-triangle !text-3xl" />
        <span>{{ t('common.delete_confirm', { name: form.email }) }}</span>
      </div>
      <template #footer>
        <Button :label="t('common.no')" icon="pi pi-times" text @click="deleteAccountDialog = false" />
        <Button :label="t('common.yes')" icon="pi pi-check" text @click="deleteAccount" />
      </template>
    </Dialog>
  </div>
</template>
