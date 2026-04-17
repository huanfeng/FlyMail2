<script setup lang="ts">
import { ref, onMounted } from 'vue';
import api from '../services/api';
import { useToast } from 'primevue/usetoast';
import DataTable from 'primevue/datatable';
import Column from 'primevue/column';
import Button from 'primevue/button';
import Dialog from 'primevue/dialog';
import InputText from 'primevue/inputtext';
import InputNumber from 'primevue/inputnumber';
import Select from 'primevue/select';
import { useI18n } from 'vue-i18n';
import PageHeader from '../components/PageHeader.vue';
import { getNumber, setString, KEYS } from '../utils/storage';

const { t } = useI18n();
const toast = useToast();
const proxies = ref([]);
const proxyDialog = ref(false);
const deleteProxyDialog = ref(false);
const proxy = ref<any>({});
const submitted = ref(false);
const loading = ref(false);
const rowsOptions = [10, 20, 50];
const proxyRows = ref(getNumber(KEYS.TABLE_ROWS_PROXIES, 10));
const proxyFirst = ref(0);

const proxyTypes = ref([
    { label: 'SOCKS5', value: 'socks5' },
    { label: 'HTTP', value: 'http' }
]);

onMounted(() => {
    fetchProxies();
});

const tablePt = {
    mask: { style: { background: 'transparent', boxShadow: 'none', opacity: 1, pointerEvents: 'none' } },
    loadingOverlay: { style: { background: 'transparent', boxShadow: 'none' } }
};

const fetchProxies = async () => {
    loading.value = true;
    try {
        const res = await api.get('/proxies');
        proxies.value = res.data;
    } catch (error) {
        console.error(error);
        toast.add({ severity: 'error', summary: t('common.error'), detail: t('proxies_view.load_error'), life: 3000 });
    } finally {
        loading.value = false;
    }
};

const openNew = () => {
    proxy.value = { type: 'socks5' };
    submitted.value = false;
    proxyDialog.value = true;
};

const hideDialog = () => {
    proxyDialog.value = false;
    submitted.value = false;
};

const saveProxy = async () => {
    submitted.value = true;

    if (proxy.value.name && proxy.value.host && proxy.value.port) {
        try {
            if (proxy.value.ID) {
                await api.put(`/proxies/${proxy.value.ID}`, proxy.value);
                toast.add({ severity: 'success', summary: t('common.success'), detail: t('proxies_view.save_success'), life: 3000 });
            } else {
                await api.post('/proxies', proxy.value);
                toast.add({ severity: 'success', summary: t('common.success'), detail: t('proxies_view.save_success'), life: 3000 });
            }
            proxyDialog.value = false;
            proxy.value = {};
            fetchProxies();
        } catch (error) {
            toast.add({ severity: 'error', summary: t('common.error'), detail: t('proxies_view.save_error'), life: 3000 });
        }
    }
};

const editProxy = (prod: any) => {
    proxy.value = { ...prod };
    proxyDialog.value = true;
};

const confirmDeleteProxy = (prod: any) => {
    proxy.value = prod;
    deleteProxyDialog.value = true;
};

const deleteProxy = async () => {
    try {
        await api.delete(`/proxies/${proxy.value.ID}`);
        deleteProxyDialog.value = false;
        proxy.value = {};
        toast.add({ severity: 'success', summary: t('common.success'), detail: t('proxies_view.delete_success'), life: 3000 });
        fetchProxies();
    } catch (error) {
        toast.add({ severity: 'error', summary: t('common.error'), detail: t('proxies_view.delete_error'), life: 3000 });
    }
};

const onProxyPage = (event: any) => {
    proxyRows.value = event.rows;
    proxyFirst.value = event.first;
    setString(KEYS.TABLE_ROWS_PROXIES, String(event.rows));
};

</script>

<template>
    <div class="page table-page">
    <PageHeader :title="t('menu.proxies')">
            <template #actions>
                <Button :label="t('common.new')" icon="pi pi-plus" class="mr-2" @click="openNew" />
                <Button :aria-label="t('common.refresh')" icon="pi pi-refresh" rounded text :loading="loading" @click="fetchProxies" v-tooltip.bottom="t('common.refresh')" />
            </template>
        </PageHeader>

        <div class="page-panel table-panel">
            <div class="table-wrapper">
                <DataTable
                    :value="proxies"
                    :loading="loading"
                    tableStyle="min-width: 50rem"
                    paginator
                    :rows="proxyRows"
                    :first="proxyFirst"
                    :rowsPerPageOptions="rowsOptions"
                    paginatorTemplate="FirstPageLink PrevPageLink PageLinks NextPageLink LastPageLink CurrentPageReport RowsPerPageDropdown"
                    currentPageReportTemplate="{first} to {last} of {totalRecords}"
                    stripedRows
                    scrollable
                    scrollHeight="flex"
                    class="table-fill"
                    :pt="tablePt"
                    @page="onProxyPage"
                >
                    <Column field="name" :header="t('proxies.name')"></Column>
                    <Column field="type" :header="t('proxies.type')"></Column>
                    <Column field="host" :header="t('proxies.host')"></Column>
                    <Column field="port" :header="t('proxies.port')"></Column>
                    <Column field="username" :header="t('proxies.username')"></Column>
                    <Column :exportable="false" style="min-width: 8rem">
                        <template #body="slotProps">
                            <Button icon="pi pi-pencil" outlined rounded class="mr-2" @click="editProxy(slotProps.data)" />
                            <Button icon="pi pi-trash" outlined rounded severity="danger" @click="confirmDeleteProxy(slotProps.data)" />
                        </template>
                    </Column>
                </DataTable>
            </div>
        </div>

        <Dialog v-model:visible="proxyDialog" :style="{ width: '520px' }" :header="t('proxies.details')" :modal="true" class="p-fluid">
            <form id="proxyForm" class="form-grid" @submit.prevent="saveProxy">
                <div class="form-row">
                    <label class="form-label" for="name">{{ t('proxies.name') }}</label>
                    <InputText id="name" v-model.trim="proxy.name" :required="true" autofocus :class="{ 'p-invalid': submitted && !proxy.name }" class="flex-1" />
                </div>
                <small class="p-error" v-if="submitted && !proxy.name">{{ t('common.required') }}</small>

                <div class="form-row">
                    <label class="form-label" for="type">{{ t('proxies.type') }}</label>
                    <Select id="type" v-model="proxy.type" :options="proxyTypes" optionLabel="label" optionValue="value" class="flex-1" />
                </div>

                <div class="form-row">
                    <label class="form-label" for="host">{{ t('proxies.host') }}</label>
                    <InputText id="host" v-model.trim="proxy.host" :required="true" :class="{ 'p-invalid': submitted && !proxy.host }" class="flex-1" />
                </div>
                <div class="form-row">
                    <label class="form-label" for="port">{{ t('proxies.port') }}</label>
                    <InputNumber id="port" v-model="proxy.port" :required="true" :useGrouping="false" :class="{ 'p-invalid': submitted && !proxy.port }" class="flex-1" />
                </div>
                <div class="form-row">
                    <label class="form-label" for="username">{{ t('proxies.username') }}</label>
                    <InputText id="username" v-model.trim="proxy.username" class="flex-1" />
                </div>
                <div class="form-row">
                    <label class="form-label" for="password">{{ t('proxies.password') }}</label>
                    <InputText id="password" v-model.trim="proxy.password" type="password" class="flex-1" />
                </div>

                <div class="flex justify-end gap-2 pt-2">
                    <Button type="button" :label="t('common.cancel')" icon="pi pi-times" outlined severity="secondary" @click="hideDialog" />
                    <Button type="submit" :label="t('common.save')" icon="pi pi-check" />
                </div>
            </form>
        </Dialog>

        <Dialog v-model:visible="deleteProxyDialog" :style="{ width: '450px' }" :header="t('common.confirm')" :modal="true">
            <div class="flex items-center gap-4">
                <i class="pi pi-exclamation-triangle !text-3xl" />
                <span v-if="proxy">{{ t('common.delete_confirm', { name: proxy.name }) }}</span>
            </div>
            <template #footer>
                <Button :label="t('common.no')" icon="pi pi-times" text @click="deleteProxyDialog = false" />
                <Button :label="t('common.yes')" icon="pi pi-check" text @click="deleteProxy" />
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
