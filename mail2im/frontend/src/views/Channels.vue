<script setup lang="ts">
import { ref, onMounted, computed } from 'vue';
import { useI18n } from 'vue-i18n';
import { useToast } from 'primevue/usetoast';
import api from '../services/api';
import Dialog from 'primevue/dialog';
import Select from 'primevue/select';
import Tag from 'primevue/tag';
import PageHeader from '../components/PageHeader.vue';
import DataTable from 'primevue/datatable';
import Column from 'primevue/column';
import Button from 'primevue/button';
import InputText from 'primevue/inputtext';
import ToggleSwitch from 'primevue/toggleswitch';
import MultiSelect from 'primevue/multiselect';
import Textarea from 'primevue/textarea';
import { getNumber, setString, KEYS } from '../utils/storage';

const { t } = useI18n();
const toast = useToast();

const channels = ref<any[]>([]);
const loading = ref(false);
const rowsOptions = [10, 20, 50];
const channelRows = ref(getNumber(KEYS.TABLE_ROWS_CHANNELS, 10));
const channelFirst = ref(0);
const showDialog = ref(false);
const saving = ref(false);
const testing = ref(false);
const isEdit = ref(false);
const editingId = ref<number | null>(null);

const form = ref<any>({
    name: '',
    type: 'telegram',
    config: {},
    min_priority: 10,
    status: 'enabled',
    quiet_mode: 'global',
    quiet_enable: false,
    quiet_start: '',
    quiet_end: '',
    subscribed_types: [],
    template: '', // This will be used for custom template or fallback
    template_id: null, // New field for template selection
});

const templates = ref<any[]>([]); // To store fetched templates
const templatesLoading = ref(false);

const telegramConfig = ref({ token: '', chat_id: '' });

const channelTypes = [
    { label: 'Telegram', value: 'telegram' }
];

const subscriptionTypes = computed(() => [
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

const templateOptions = computed(() => {
    const options = templates.value.map(tmpl => ({ label: tmpl.name, value: tmpl.id }));
    options.unshift({ label: t('channels.custom_template'), value: null }); // Option for custom/inline template
    return options;
});

const quietModes = computed(() => [
    { label: t('channels.quiet_global'), value: 'global' },
    { label: t('channels.quiet_override'), value: 'override' },
    { label: t('channels.quiet_off'), value: 'off' }
]);

const priorities = computed(() => [
    { label: t('common.priority_low'), value: 0 },
    { label: t('common.priority_normal'), value: 10 },
    { label: t('common.priority_high'), value: 20 },
    { label: t('common.priority_critical'), value: 30 }
]);

const fetchTemplates = async () => {
    templatesLoading.value = true;
    try {
        const res = await api.get('/templates');
        templates.value = res.data;
    } catch (error) {
        toast.add({ severity: 'error', summary: t('common.error'), detail: t('templates.load_error'), life: 3000 });
    } finally {
        templatesLoading.value = false;
    }
};

const resetForm = () => {
    form.value = {
        name: '',
        type: 'telegram',
        config: {},
        min_priority: 10,
        status: 'enabled',
        quiet_mode: 'global',
        quiet_enable: false,
        quiet_start: '',
        quiet_end: '',
        subscribed_types: [],
        template: '',
        template_id: null, // Reset template selection
    };
    telegramConfig.value = { token: '', chat_id: '' };
    isEdit.value = false;
    editingId.value = null;
};

const loadChannels = async () => {
    loading.value = true;
    try {
        const res = await api.get('/channels');
        channels.value = res.data || [];
    } catch (error) {
        toast.add({ severity: 'error', summary: t('common.error'), detail: t('channels.load_error'), life: 3000 });
    } finally {
        loading.value = false;
    }
};

const openCreate = () => {
    resetForm();
    showDialog.value = true;
};

const openEdit = (row: any) => {
    resetForm();
    isEdit.value = true;
    editingId.value = row.id || row.ID;
    
    let subs = [];
    try {
        const rawSubs = row.subscribed_types || row.SubscribedTypes;
        if (rawSubs) {
            subs = JSON.parse(rawSubs);
        }
    } catch (e) {}

    form.value = {
        name: row.name || row.Name,
        type: row.type || row.Type,
        config: row.config || row.Config,
        min_priority: row.min_priority ?? row.MinPriority ?? 10,
        status: row.status || row.Status || 'enabled',
        quiet_mode: row.quiet_mode || row.QuietMode || 'global',
        quiet_enable: !!(row.quiet_enable ?? row.QuietEnable),
        quiet_start: row.quiet_start || row.QuietStart || '',
        quiet_end: row.quiet_end || row.QuietEnd || '',
        subscribed_types: subs,
        template: row.template || row.Template || '',
        template_id: row.template_id || row.TemplateID || null, // Load template ID
    };
    if (form.value.type === 'telegram' && form.value.config) {
        try {
            telegramConfig.value = typeof form.value.config === 'string' ? JSON.parse(form.value.config) : form.value.config;
        } catch {
            telegramConfig.value = { token: '', chat_id: '' };
        }
    }
    showDialog.value = true;
};

const testEventType = ref('system');
const testEventTypes = computed(() => [
    { label: t('channels.test_type_system'), value: 'system' },
    { label: t('channels.test_type_email'), value: 'email' }
]);

const testChannel = async () => {
    testing.value = true;
    try {
        let configStr = '';
        if (form.value.type === 'telegram') {
            configStr = JSON.stringify(telegramConfig.value);
        }

        await api.post('/channels/test', {
            type: form.value.type,
            config: configStr,
            event_type: testEventType.value
        });
        toast.add({ severity: 'success', summary: t('common.success'), detail: t('channels.test_success'), life: 3000 });
    } catch (error: any) {
        const detail = error.response?.data?.error ? `${t('channels.test_error')}: ${error.response.data.error}` : t('channels.test_error');
        toast.add({ severity: 'error', summary: t('common.error'), detail, life: 3000 });
    } finally {
        testing.value = false;
    }
};

const saveChannel = async () => {
    if (!form.value.name) {
        toast.add({ severity: 'warn', summary: t('common.warning'), detail: t('channels.name_required'), life: 2500 });
        return;
    }

    saving.value = true;
    try {
        const payload: any = {
            ...form.value,
            config: form.value.type === 'telegram' ? JSON.stringify(telegramConfig.value) : form.value.config,
            subscribed_types: JSON.stringify(form.value.subscribed_types),
            // Conditionally set template_id or template
            template_id: form.value.template_id,
            template: form.value.template_id ? '' : form.value.template // If template_id is set, clear inline template
        };

        if (isEdit.value && editingId.value) {
            await api.put(`/channels/${editingId.value}`, payload);
        } else {
            await api.post('/channels', payload);
        }

        toast.add({ severity: 'success', summary: t('common.success'), detail: t('channels.save_success'), life: 3000 });
        showDialog.value = false;
        loadChannels();
    } catch (error) {
        toast.add({ severity: 'error', summary: t('common.error'), detail: t('channels.save_error'), life: 3000 });
    } finally {
        saving.value = false;
    }
};

const deleteChannel = async (id: number) => {
    if (!confirm(t('common.delete_confirm', { name: t('channels.title') }))) return;
    try {
        await api.delete(`/channels/${id}`);
        toast.add({ severity: 'success', summary: t('common.success'), detail: t('channels.delete_success'), life: 3000 });
        loadChannels();
    } catch (error) {
        toast.add({ severity: 'error', summary: t('common.error'), detail: t('channels.delete_error'), life: 3000 });
    }
};

const priorityLabel = (val: number) => priorities.value.find((p) => p.value === val)?.label || val;
const statusSeverity = (status: string) => (status === 'enabled' ? 'success' : 'danger');
const statusLabel = (status: string) => (status === 'enabled' ? t('channels.status_enabled') : t('channels.status_disabled'));
const quietSummary = (row: any) => {
    const mode = (row.quiet_mode || row.QuietMode || 'global').toLowerCase();
    if (mode === 'off') return t('channels.quiet_off');
    if (mode === 'override') {
        const enabled = !!(row.quiet_enable ?? row.QuietEnable);
        if (!enabled) return t('channels.quiet_off');
        const start = row.quiet_start || row.QuietStart || '--';
        const end = row.quiet_end || row.QuietEnd || '--';
        return `${start} - ${end}`;
    }
    return t('channels.quiet_global');
};

onMounted(() => {
    loadChannels();
    fetchTemplates(); // Fetch templates when component mounts
});

const onChannelPage = (event: any) => {
    channelRows.value = event.rows;
    channelFirst.value = event.first;
    setString(KEYS.TABLE_ROWS_CHANNELS, String(event.rows));
};

const tablePt = {
    mask: { style: { background: 'transparent', boxShadow: 'none', opacity: 1, pointerEvents: 'none' } },
    loadingOverlay: { style: { background: 'transparent', boxShadow: 'none' } }
};
</script>

<template>
    <div class="page table-page">
        <PageHeader :title="t('channels.title')">
            <template #actions>
                <Button :label="t('channels.add')" icon="pi pi-plus" @click="openCreate" />
                <Button icon="pi pi-refresh" rounded text :loading="loading" @click="loadChannels" :aria-label="t('common.refresh')" v-tooltip.bottom="t('common.refresh')" />
            </template>
        </PageHeader>

        <div class="page-panel table-panel">
            <div class="table-wrapper">
                <DataTable
                    :value="channels"
                    :loading="loading"
                    tableStyle="min-width: 55rem"
                    paginator
                    :rows="channelRows"
                    :first="channelFirst"
                    :rowsPerPageOptions="rowsOptions"
                    paginatorTemplate="FirstPageLink PrevPageLink PageLinks NextPageLink LastPageLink CurrentPageReport RowsPerPageDropdown"
                    currentPageReportTemplate="{first} to {last} of {totalRecords}"
                    stripedRows
                    scrollable
                    scrollHeight="flex"
                    class="table-fill"
                    :pt="tablePt"
                    @page="onChannelPage"
                >
                    <Column field="name" :header="t('channels.name')"></Column>
                    <Column field="type" :header="t('channels.type')">
                        <template #body="slotProps">
                            <Tag :value="slotProps.data.type || slotProps.data.Type" severity="info" />
                        </template>
                    </Column>
                    <Column field="min_priority" :header="t('channels.priority')">
                        <template #body="slotProps">
                            {{ priorityLabel(slotProps.data.min_priority ?? slotProps.data.MinPriority) }}
                        </template>
                    </Column>
                    <Column :header="t('channels.quiet_mode')">
                        <template #body="slotProps">
                            <span class="text-sm text-surface-600">{{ quietSummary(slotProps.data) }}</span>
                        </template>
                    </Column>
                    <Column field="status" :header="t('channels.status')">
                        <template #body="slotProps">
                            <Tag :value="statusLabel(slotProps.data.status || slotProps.data.Status)" :severity="statusSeverity(slotProps.data.status || slotProps.data.Status)" />
                        </template>
                    </Column>
                    <Column :exportable="false" style="min-width: 10rem">
                        <template #body="slotProps">
                            <Button icon="pi pi-pencil" outlined rounded class="mr-2" @click="openEdit(slotProps.data)" />
                            <Button icon="pi pi-trash" outlined rounded severity="danger" @click="deleteChannel(slotProps.data.id || slotProps.data.ID)" />
                        </template>
                    </Column>
                    <template #empty> {{ t('channels.no_channels') }} </template>
                </DataTable>
            </div>
        </div>

        <Dialog v-model:visible="showDialog" :header="isEdit ? t('channels.edit') : t('channels.add')" :modal="true" class="p-fluid" style="width: 600px">
            <div class="flex flex-col gap-6">
                <!-- Group 1: Basic Info -->
                <div class="form-section">
                    <h4 class="section-title">{{ t('channels.section_basic') }}</h4>
                    <div class="form-grid">
                        <div class="form-row">
                            <label class="form-label">{{ t('channels.status') }}</label>
                            <div class="flex items-center gap-3">
                                <ToggleSwitch v-model="form.status" true-value="enabled" false-value="disabled" />
                                <span class="text-sm text-surface-500">{{ statusLabel(form.status) }}</span>
                            </div>
                        </div>
                        <div class="form-row">
                            <label class="form-label" for="name">{{ t('channels.name') }}</label>
                            <InputText id="name" v-model="form.name" class="flex-1" required autofocus />
                        </div>
                        <div class="form-row">
                            <label class="form-label" for="type">{{ t('channels.type') }}</label>
                            <Select id="type" v-model="form.type" :options="channelTypes" optionLabel="label" optionValue="value" class="flex-1" />
                        </div>
                    </div>
                </div>

                <!-- Group 2: Configuration -->
                <div class="form-section">
                    <h4 class="section-title">{{ t('channels.section_config') }}</h4>
                    <div v-if="form.type === 'telegram'" class="form-grid">
                        <div class="form-row">
                            <label class="form-label" for="token">Bot Token</label>
                            <InputText id="token" v-model="telegramConfig.token" class="flex-1" />
                        </div>
                        <div class="form-row">
                            <label class="form-label" for="chat_id">Chat ID</label>
                            <InputText id="chat_id" v-model="telegramConfig.chat_id" class="flex-1" />
                        </div>
                    </div>
                </div>

                <!-- Group 3: Subscription Rules -->
                <div class="form-section">
                    <h4 class="section-title">{{ t('channels.section_sub') }}</h4>
                    <div class="form-grid">
                        <div class="form-row">
                            <label class="form-label" for="priority">{{ t('channels.priority') }}</label>
                            <Select id="priority" v-model="form.min_priority" :options="priorities" optionLabel="label" optionValue="value" class="flex-1" />
                        </div>
                        <div class="form-row">
                            <label class="form-label" for="quiet">{{ t('channels.quiet_mode') }}</label>
                            <div class="flex-1 flex flex-col gap-2">
                                <Select id="quiet" v-model="form.quiet_mode" :options="quietModes" optionLabel="label" optionValue="value" class="w-full" />
                                <div v-if="form.quiet_mode === 'override'" class="flex flex-col gap-2">
                                    <div class="flex items-center gap-2">
                                        <ToggleSwitch v-model="form.quiet_enable" />
                                        <span class="text-sm">{{ t('settings.quiet_enable') }}</span>
                                    </div>
                                    <div class="flex gap-2">
                                        <input v-model="form.quiet_start" type="time" class="select flex-1" :disabled="!form.quiet_enable" />
                                        <input v-model="form.quiet_end" type="time" class="select flex-1" :disabled="!form.quiet_enable" />
                                    </div>
                                </div>
                            </div>
                        </div>
                        <div class="form-row">
                            <label class="form-label">{{ t('channels.subscription') }}</label>
                            <div class="flex-1">
                                <MultiSelect v-model="form.subscribed_types" :options="subscriptionTypes" optionLabel="label" optionValue="value" :placeholder="t('channels.subscription_hint')" display="chip" class="w-full" />
                                <small class="text-surface-500 block mt-1">{{ t('channels.subscription_hint') }}</small>
                            </div>
                        </div>
                        <div class="form-row items-start">
                            <label class="form-label mt-2">{{ t('channels.template') }}</label>
                            <div class="flex-1">
                                <Select v-model="form.template_id" :options="templateOptions" optionLabel="label" optionValue="value" :placeholder="t('channels.select_template')" class="w-full" showClear :loading="templatesLoading" />
                                <small class="text-surface-500 block mt-1">{{ t('channels.template_hint') }}</small>
                                <Textarea v-if="form.template_id === null" v-model="form.template" rows="3" class="w-full mt-2" :placeholder="t('channels.template_hint_custom')" />
                                <small v-if="form.template_id === null" v-pre class="text-surface-500 block mt-1">{{ t('channels.template_vars') }}: {{.Subject}}, {{.From}}, {{.Link}}, {{.Type}}</small>
                            </div>
                        </div>
                    </div>
                </div>
            </div>

            <template #footer>
                <div class="flex justify-between w-full pt-4 border-t border-surface-200">
                    <div class="flex gap-2 items-center">
                        <Select v-model="testEventType" :options="testEventTypes" optionLabel="label" optionValue="value" class="w-40" size="small" />
                        <Button :label="t('channels.test_send')" icon="pi pi-send" @click="testChannel" :loading="testing" outlined severity="info" size="small" />
                    </div>
                    <div class="flex gap-2">
                        <Button :label="t('common.cancel')" icon="pi pi-times" outlined severity="secondary" @click="showDialog = false" />
                        <Button :label="t('common.save')" icon="pi pi-check" @click="saveChannel" :loading="saving" />
                    </div>
                </div>
            </template>
        </Dialog>
    </div>
</template>

<style scoped>
.form-grid {
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
    min-width: 96px;
    text-align: right;
    color: var(--text-color);
    font-weight: 600;
}
</style>
