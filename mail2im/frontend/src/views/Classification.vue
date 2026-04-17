<script setup lang="ts">
import { ref, onMounted, computed } from 'vue';
import { useI18n } from 'vue-i18n';
import { useToast } from 'primevue/usetoast';
import api from '../services/api';
import PageHeader from '../components/PageHeader.vue';
import DataTable from 'primevue/datatable';
import Column from 'primevue/column';
import Button from 'primevue/button';
import InputText from 'primevue/inputtext';
import InputNumber from 'primevue/inputnumber';
import Select from 'primevue/select';
import Tag from 'primevue/tag';
import Dialog from 'primevue/dialog';
import Tabs from 'primevue/tabs';
import TabList from 'primevue/tablist';
import Tab from 'primevue/tab';
import TabPanels from 'primevue/tabpanels';
import TabPanel from 'primevue/tabpanel';
// ToggleSwitch removed (unused)

const { t } = useI18n();
const toast = useToast();

const activeTab = ref('mailTypes');

// --- Mail Types ---
const mailTypes = ref<any[]>([]);
const mailTypesLoading = ref(false);
const mailTypeDialog = ref(false);
const mailTypeForm = ref<any>({ id: null, key: '', name: '', priority: 10, is_system: false });
const isEditMailType = ref(false);

const priorities = computed(() => [
  { label: t('common.priority_low'), value: 0 },
  { label: t('common.priority_normal'), value: 10 },
  { label: t('common.priority_high'), value: 20 },
  { label: t('common.priority_critical'), value: 30 }
]);

const fetchMailTypes = async () => {
  mailTypesLoading.value = true;
  try {
    const res = await api.get('/mailtypes');
    mailTypes.value = res.data;
  } catch (error) {
    toast.add({ severity: 'error', summary: t('common.error'), detail: t('classification.mail_types_load_error'), life: 3000 });
  } finally {
    mailTypesLoading.value = false;
  }
};

const openCreateMailType = () => {
  mailTypeForm.value = { id: null, key: '', name: '', priority: 10, is_system: false };
  isEditMailType.value = false;
  mailTypeDialog.value = true;
};

const openEditMailType = (type: any) => {
  mailTypeForm.value = { ...type };
  isEditMailType.value = true;
  mailTypeDialog.value = true;
};

const saveMailType = async () => {
  try {
    if (isEditMailType.value && mailTypeForm.value.id) {
      await api.put(`/mailtypes/${mailTypeForm.value.id}`, mailTypeForm.value);
      toast.add({ severity: 'success', summary: t('common.success'), detail: t('classification.mail_type_update_success'), life: 3000 });
    } else {
      await api.post('/mailtypes', mailTypeForm.value);
      toast.add({ severity: 'success', summary: t('common.success'), detail: t('classification.mail_type_create_success'), life: 3000 });
    }
    mailTypeDialog.value = false;
    fetchMailTypes();
  } catch (error: any) {
    toast.add({ severity: 'error', summary: t('common.error'), detail: error.response?.data?.error || t('classification.mail_type_save_error'), life: 3000 });
  }
};

const deleteMailType = async (type: any) => {
  if (type.is_system) {
    toast.add({ severity: 'warn', summary: t('common.warning'), detail: t('classification.system_mail_type_delete_forbidden'), life: 3000 });
    return;
  }
  if (!confirm(t('common.delete_confirm', { name: type.name }))) return;
  try {
    await api.delete(`/mailtypes/${type.id}`);
    toast.add({ severity: 'success', summary: t('common.success'), detail: t('classification.mail_type_delete_success'), life: 3000 });
    fetchMailTypes();
  } catch (error: any) {
    toast.add({ severity: 'error', summary: t('common.error'), detail: error.response?.data?.error || t('classification.mail_type_delete_error'), life: 3000 });
  }
};

// --- Folder Rules ---
const folderRules = ref<any[]>([]);
const folderRulesLoading = ref(false);
const folderRuleDialog = ref(false);
const folderRuleForm = ref<any>({ id: null, name: '', pattern: '', target_type: '', order: 10, is_active: true });
const isEditFolderRule = ref(false);

const mailTypeOptions = computed(() => mailTypes.value.map(type => ({ label: type.name, value: type.key })));

const fetchFolderRules = async () => {
  folderRulesLoading.value = true;
  try {
    const res = await api.get('/rules');
    folderRules.value = res.data;
  } catch (error) {
    toast.add({ severity: 'error', summary: t('common.error'), detail: t('classification.folder_rules_load_error'), life: 3000 });
  } finally {
    folderRulesLoading.value = false;
  }
};

const openCreateFolderRule = () => {
  folderRuleForm.value = { id: null, name: '', pattern: '', target_type: '', order: 10, is_active: true };
  isEditFolderRule.value = false;
  folderRuleDialog.value = true;
};

const openEditFolderRule = (rule: any) => {
  folderRuleForm.value = { ...rule };
  isEditFolderRule.value = true;
  folderRuleDialog.value = true;
};

const saveFolderRule = async () => {
  try {
    if (isEditFolderRule.value && folderRuleForm.value.id) {
      await api.put(`/rules/${folderRuleForm.value.id}`, folderRuleForm.value);
      toast.add({ severity: 'success', summary: t('common.success'), detail: t('classification.folder_rule_update_success'), life: 3000 });
    } else {
      await api.post('/rules', folderRuleForm.value);
      toast.add({ severity: 'success', summary: t('common.success'), detail: t('classification.folder_rule_create_success'), life: 3000 });
    }
    folderRuleDialog.value = false;
    fetchFolderRules();
  } catch (error: any) {
    toast.add({ severity: 'error', summary: t('common.error'), detail: error.response?.data?.error || t('classification.folder_rule_save_error'), life: 3000 });
  }
};

const deleteFolderRule = async (rule: any) => {
  if (!confirm(t('common.delete_confirm', { name: rule.name }))) return;
  try {
    await api.delete(`/rules/${rule.id}`);
    toast.add({ severity: 'success', summary: t('common.success'), detail: t('classification.folder_rule_delete_success'), life: 3000 });
    fetchFolderRules();
  } catch (error: any) {
    toast.add({ severity: 'error', summary: t('common.error'), detail: error.response?.data?.error || t('classification.folder_rule_delete_error'), life: 3000 });
  }
};

onMounted(() => {
  fetchMailTypes();
  fetchFolderRules();
});
</script>

<template>
  <div class="page table-page">
    <PageHeader :title="t('classification.title')">
      <template #actions>
        <Button :label="t('classification.add_mail_type')" icon="pi pi-plus" class="mr-2" @click="openCreateMailType" />
        <Button :label="t('classification.add_folder_rule')" icon="pi pi-plus" @click="openCreateFolderRule" />
        <Button :aria-label="t('common.refresh')" icon="pi pi-refresh" rounded text :loading="mailTypesLoading || folderRulesLoading" @click="fetchMailTypes(); fetchFolderRules();" v-tooltip.bottom="t('common.refresh')" />
      </template>
    </PageHeader>

    <div class="page-panel table-panel">
      <Tabs :value="activeTab">
        <TabList>
          <Tab value="mailTypes">{{ t('classification.mail_types_tab') }}</Tab>
          <Tab value="folderRules">{{ t('classification.folder_rules_tab') }}</Tab>
        </TabList>
        <TabPanels>
          <TabPanel value="mailTypes">
            <DataTable
              :value="mailTypes"
              :loading="mailTypesLoading"
              stripedRows
              scrollable
              scrollHeight="flex"
              class="table-fill"
            >
              <Column field="key" :header="t('classification.mail_type_key')"></Column>
              <Column field="name" :header="t('classification.mail_type_name')"></Column>
              <Column field="priority" :header="t('classification.mail_type_priority')">
                <template #body="slotProps">
                  <Tag :value="priorities.find(p => p.value === slotProps.data.priority)?.label || slotProps.data.priority" />
                </template>
              </Column>
              <Column field="is_system" :header="t('classification.mail_type_is_system')">
                <template #body="slotProps">
                  <i :class="slotProps.data.is_system ? 'pi pi-check-circle text-green-500' : 'pi pi-times-circle text-red-500'"></i>
                </template>
              </Column>
              <Column :exportable="false" style="min-width: 10rem">
                <template #body="slotProps">
                  <Button icon="pi pi-pencil" outlined rounded class="mr-2" @click="openEditMailType(slotProps.data)" />
                  <Button icon="pi pi-trash" outlined rounded severity="danger" @click="deleteMailType(slotProps.data)" :disabled="slotProps.data.is_system" />
                </template>
              </Column>
            </DataTable>
          </TabPanel>

          <TabPanel value="folderRules">
            <DataTable
              :value="folderRules"
              :loading="folderRulesLoading"
              stripedRows
              scrollable
              scrollHeight="flex"
              class="table-fill"
            >
              <Column field="name" :header="t('classification.rule_name')"></Column>
              <Column field="pattern" :header="t('classification.rule_pattern')"></Column>
              <Column field="target_type" :header="t('classification.rule_target_type')">
                 <template #body="slotProps">
                   <Tag :value="mailTypes.find(mt => mt.key === slotProps.data.target_type)?.name || slotProps.data.target_type" severity="info" />
                 </template>
              </Column>
              <Column field="order" :header="t('classification.rule_order')"></Column>
              <Column :exportable="false" style="min-width: 10rem">
                <template #body="slotProps">
                  <Button icon="pi pi-pencil" outlined rounded class="mr-2" @click="openEditFolderRule(slotProps.data)" />
                  <Button icon="pi pi-trash" outlined rounded severity="danger" @click="deleteFolderRule(slotProps.data)" />
                </template>
              </Column>
            </DataTable>
          </TabPanel>
        </TabPanels>
      </Tabs>
    </div>

    <!-- Mail Type Dialog -->
    <Dialog v-model:visible="mailTypeDialog" :style="{ width: '450px' }" :header="isEditMailType ? t('classification.edit_mail_type') : t('classification.add_mail_type')" :modal="true" class="p-fluid">
      <div class="field">
        <label for="mailTypeKey">{{ t('classification.mail_type_key') }}</label>
        <InputText id="mailTypeKey" v-model="mailTypeForm.key" required :disabled="mailTypeForm.is_system" />
        <small class="text-surface-500">{{ t('classification.mail_type_key_hint') }}</small>
      </div>
      <div class="field">
        <label for="mailTypeName">{{ t('classification.mail_type_name') }}</label>
        <InputText id="mailTypeName" v-model="mailTypeForm.name" required />
      </div>
      <div class="field">
        <label for="mailTypePriority">{{ t('classification.mail_type_priority') }}</label>
        <Select id="mailTypePriority" v-model="mailTypeForm.priority" :options="priorities" optionLabel="label" optionValue="value" class="w-full" />
      </div>
      <template #footer>
        <Button :label="t('common.cancel')" icon="pi pi-times" outlined @click="mailTypeDialog = false" />
        <Button :label="t('common.save')" icon="pi pi-check" @click="saveMailType" />
      </template>
    </Dialog>

    <!-- Folder Rule Dialog -->
    <Dialog v-model:visible="folderRuleDialog" :style="{ width: '450px' }" :header="isEditFolderRule ? t('classification.edit_folder_rule') : t('classification.add_folder_rule')" :modal="true" class="p-fluid">
      <div class="field">
        <label for="folderRuleName">{{ t('classification.rule_name') }}</label>
        <InputText id="folderRuleName" v-model="folderRuleForm.name" required />
      </div>
      <div class="field">
        <label for="folderRulePattern">{{ t('classification.rule_pattern') }}</label>
        <InputText id="folderRulePattern" v-model="folderRuleForm.pattern" required />
        <small class="text-surface-500">{{ t('classification.rule_pattern_hint') }}</small>
      </div>
      <div class="field">
        <label for="folderRuleTargetType">{{ t('classification.rule_target_type') }}</label>
        <Select id="folderRuleTargetType" v-model="folderRuleForm.target_type" :options="mailTypeOptions" optionLabel="label" optionValue="value" class="w-full" required />
      </div>
      <div class="field">
        <label for="folderRuleOrder">{{ t('classification.rule_order') }}</label>
        <InputNumber id="folderRuleOrder" v-model="folderRuleForm.order" :useGrouping="false" />
        <small class="text-surface-500">{{ t('classification.rule_order_hint') }}</small>
      </div>
      <template #footer>
        <Button :label="t('common.cancel')" icon="pi pi-times" outlined @click="folderRuleDialog = false" />
        <Button :label="t('common.save')" icon="pi pi-check" @click="saveFolderRule" />
      </template>
    </Dialog>
  </div>
</template>

<style scoped>
.table-fill {
  min-height: 400px; /* Ensure tables have some height */
}
</style>