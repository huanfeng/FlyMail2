<script setup lang="ts">
import { ref, onMounted } from 'vue';
import { useI18n } from 'vue-i18n';
import { useRouter } from 'vue-router';
import { useToast } from 'primevue/usetoast';
import api from '../services/api';
import PageHeader from '../components/PageHeader.vue';
import Tag from 'primevue/tag';
import Dialog from 'primevue/dialog';
import { getNumber, setString, KEYS } from '../utils/storage';

const { t } = useI18n();
const router = useRouter();
const toast = useToast();

const emails = ref([]);
const loading = ref(false);
const totalRecords = ref(0);
const rowsOptions = [10, 20, 50];
const savedEmailRows = getNumber(KEYS.TABLE_ROWS_EMAILS, 10);
const lazyParams = ref({
    first: 0,
    rows: savedEmailRows,
    page: 1,
    sortField: 'received_at',
    sortOrder: -1,
    filters: {}
});
const searchQuery = ref('');

const onPage = (event: any) => {
    lazyParams.value = event;
    setString(KEYS.TABLE_ROWS_EMAILS, String(event.rows));
    loadEmails();
};

const onSort = (event: any) => {
    lazyParams.value = { ...lazyParams.value, ...event, first: 0 };
    loadEmails();
};

const loadEmails = async () => {
    loading.value = true;
    try {
        const page = Math.floor(lazyParams.value.first / lazyParams.value.rows) + 1;
        const pageSize = lazyParams.value.rows;

        const res = await api.get('/emails', {
            params: {
                page,
                pageSize,
                search: searchQuery.value,
                sortField: lazyParams.value.sortField,
                sortOrder: lazyParams.value.sortOrder === 1 ? 'asc' : 'desc'
            }
        });

        emails.value = res.data.data;
        totalRecords.value = res.data.total;
    } catch (error) {
        console.error(error);
        toast.add({ severity: 'error', summary: t('common.error'), detail: t('emails_view.load_error'), life: 3000 });
    } finally {
        loading.value = false;
    }
};

const onSearch = () => {
    lazyParams.value.first = 0;
    loadEmails();
};

const deleteEmailDialog = ref(false);
const deleteEmailAllDialog = ref(false);
const targetEmail = ref<any>(null);
const deleting = ref(false);
const clearing = ref(false);

const confirmDeleteEmail = (row: any) => {
    targetEmail.value = row;
    deleteEmailDialog.value = true;
};

const deleteEmail = async () => {
    if (!targetEmail.value) return;
    deleting.value = true;
    try {
        const id = getId(targetEmail.value);
        await api.delete(`/emails/${id}`);
        toast.add({ severity: 'success', summary: t('common.success'), detail: t('emails_view.delete_success'), life: 2500 });
        deleteEmailDialog.value = false;
        await loadEmails();
    } catch (error) {
        console.error(error);
        toast.add({ severity: 'error', summary: t('common.error'), detail: t('emails_view.delete_error'), life: 2500 });
    } finally {
        deleting.value = false;
    }
};

const confirmDeleteAll = () => {
    deleteEmailAllDialog.value = true;
};

const deleteAllEmails = async () => {
    clearing.value = true;
    try {
        await api.delete('/emails');
        toast.add({ severity: 'success', summary: t('common.success'), detail: t('emails_view.delete_all_success'), life: 2500 });
        deleteEmailAllDialog.value = false;
        await loadEmails();
    } catch (error) {
        console.error(error);
        toast.add({ severity: 'error', summary: t('common.error'), detail: t('emails_view.delete_all_error'), life: 2500 });
    } finally {
        clearing.value = false;
    }
};

const mailTypeLabel = (type: string) => {
    switch ((type || '').toLowerCase()) {
        case 'important':
            return t('emails_view.type_important');
        case 'primary':
            return t('emails_view.type_primary');
        case 'promotion':
            return t('emails_view.type_promotion');
        case 'social':
            return t('emails_view.type_social');
        case 'spam':
            return t('emails_view.type_spam');
        case 'trash':
            return t('emails_view.type_trash');
        case 'draft':
            return t('emails_view.type_draft');
        case 'sent':
            return t('emails_view.type_sent');
        default:
            return t('emails_view.type_normal');
    }
};

const mailTypeSeverity = (type: string) => {
    switch ((type || '').toLowerCase()) {
        case 'important':
            return 'warning';
        case 'promotion':
        case 'social':
            return 'info';
        case 'spam':
        case 'trash':
            return 'danger';
        default:
            return 'secondary';
    }
};

const getId = (row: any) => (row?.id ?? row?.ID);

const viewEmail = (row: any) => {
    const id = typeof row === 'number' ? row : getId(row);
    if (!id) {
        toast.add({ severity: 'warn', summary: t('common.warning'), detail: t('emails_view.load_error'), life: 2500 });
        return;
    }
    router.push(`/emails/${id}`);
};

const openEmailNewTab = (row: any) => {
    const id = typeof row === 'number' ? row : getId(row);
    if (!id) {
        toast.add({ severity: 'warn', summary: t('common.warning'), detail: t('emails_view.load_error'), life: 2500 });
        return;
    }
    const routeUrl = router.resolve({ name: 'email-standalone', params: { id } });
    window.open(routeUrl.href, '_blank');
};

const formatDate = (dateStr: string) => {
    if (!dateStr) return '-';
    const d = new Date(dateStr);
    return isNaN(d.getTime()) ? '-' : d.toLocaleString();
};

onMounted(() => {
    loadEmails();
});

const tablePt = {
    mask: { style: { background: 'transparent', boxShadow: 'none', opacity: 1, pointerEvents: 'none' } },
    loadingOverlay: { style: { background: 'transparent', boxShadow: 'none' } }
};
</script>

<template>
    <div class="page table-page">
        <PageHeader
            :title="t('common.emails')"
            :search-placeholder="t('common.search')"
            :search-value="searchQuery"
            :show-search="true"
            :loading="loading"
            @update:searchValue="searchQuery = $event"
        @search="onSearch"
    >
        <template #actions>
            <Button
                icon="pi pi-trash"
                severity="danger"
                text
                rounded
                @click="confirmDeleteAll"
                :disabled="loading || !totalRecords"
                :aria-label="t('emails_view.delete_all')"
                v-tooltip.bottom="t('emails_view.delete_all')"
            />
            <Button icon="pi pi-refresh" @click="loadEmails" :loading="loading" rounded text :aria-label="t('common.refresh')" v-tooltip.bottom="t('common.refresh')" />
        </template>
    </PageHeader>

        <div class="page-panel table-panel">
            <div class="table-wrapper">
                <DataTable
                    :value="emails"
                    :lazy="true"
                    paginator
                    :rows="lazyParams.rows"
                    :first="lazyParams.first"
                    :totalRecords="totalRecords"
                    :loading="loading"
                    :sortField="lazyParams.sortField"
                    :sortOrder="lazyParams.sortOrder"
                    sortMode="single"
                    stripedRows
                    resizableColumns
                    columnResizeMode="fit"
                    scrollable
                    scrollHeight="flex"
                    @page="onPage"
                    @sort="onSort"
                    tableStyle="min-width: 60rem table-layout: fixed"
                    paginatorTemplate="FirstPageLink PrevPageLink PageLinks NextPageLink LastPageLink CurrentPageReport RowsPerPageDropdown"
                    :rowsPerPageOptions="rowsOptions"
                    currentPageReportTemplate="{first} to {last} of {totalRecords}"
                    class="table-fill"
                    :pt="tablePt"
                >
                    <Column field="subject" :header="t('common.subject')" :sortable="true" :style="{ minWidth: '22rem' }">
                        <template #body="slotProps">
                            <span
                                class="font-medium cursor-pointer hover:text-primary hover:underline"
                                :class="{ 'font-bold': !slotProps.data.is_read }"
                                @click="viewEmail(slotProps.data)"
                            >
                                {{ slotProps.data.subject || '-' }}
                            </span>
                        </template>
                    </Column>
                    <Column field="mailbox" :header="t('emails_view.mailbox')" :sortable="true" :style="{ width: '14rem' }">
                        <template #body="slotProps">
                            {{ slotProps.data.mailbox || slotProps.data.mailbox_path || '-' }}
                        </template>
                    </Column>
                    <Column field="mail_type" :header="t('emails_view.mail_type')" :style="{ width: '10rem' }" :sortable="true">
                        <template #body="slotProps">
                            <Tag :value="mailTypeLabel(slotProps.data.mail_type)" :severity="mailTypeSeverity(slotProps.data.mail_type)" />
                        </template>
                    </Column>
                    <Column field="received_at" :header="t('common.received_at')" :sortable="true" :style="{ width: '14rem' }">
                        <template #body="slotProps">
                            {{ formatDate(slotProps.data.received_at) }}
                        </template>
                    </Column>
                    <Column field="from" :header="t('common.from')" :sortable="true" :style="{ width: '10rem', maxWidth: '16rem' }">
                        <template #body="slotProps">
                            {{ slotProps.data.from || '-' }}
                        </template>
                    </Column>
                    <Column field="to" :header="t('common.to')" :sortable="true" :style="{ width: '10rem', maxWidth: '16rem' }">
                        <template #body="slotProps">
                            {{ slotProps.data.to || '-' }}
                        </template>
                    </Column>
                    <Column frozen alignFrozen="right" :exportable="false" style="min-width: 8rem">
                        <template #body="slotProps">
                            <Button icon="pi pi-eye" outlined rounded class="mr-2" @click="viewEmail(slotProps.data)" />
                            <Button icon="pi pi-external-link" text rounded @click="openEmailNewTab(slotProps.data)" />
                            <Button icon="pi pi-trash" text rounded severity="danger" class="ml-2" @click="confirmDeleteEmail(slotProps.data)" />
                        </template>
                    </Column>
                    <template #empty> {{ t('common.no_emails') }} </template>
                </DataTable>
            </div>
        </div>

        <Dialog v-model:visible="deleteEmailDialog" :style="{ width: '28rem' }" :header="t('common.confirm')" :modal="true">
            <p class="mb-3">{{ t('emails_view.delete_confirm') }}</p>
            <p class="text-sm text-color-secondary mb-5">{{ t('emails_view.delete_notice') }}</p>
            <div class="flex justify-end gap-2">
                <Button :label="t('common.no')" icon="pi pi-times" text @click="deleteEmailDialog = false" />
                <Button :label="t('common.yes')" icon="pi pi-check" :loading="deleting" severity="danger" @click="deleteEmail" />
            </div>
        </Dialog>

        <Dialog v-model:visible="deleteEmailAllDialog" :style="{ width: '30rem' }" :header="t('common.confirm')" :modal="true">
            <p class="mb-3">{{ t('emails_view.delete_all_confirm') }}</p>
            <p class="text-sm text-color-secondary mb-5">{{ t('emails_view.delete_notice') }}</p>
            <div class="flex justify-end gap-2">
                <Button :label="t('common.no')" icon="pi pi-times" text @click="deleteEmailAllDialog = false" />
                <Button :label="t('common.yes')" icon="pi pi-check" :loading="clearing" severity="danger" @click="deleteAllEmails" />
            </div>
        </Dialog>
    </div>
</template>
