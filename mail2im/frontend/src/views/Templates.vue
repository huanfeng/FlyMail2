<script setup lang="ts">
import { ref, onMounted, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import { useToast } from 'primevue/usetoast';
import api from '../services/api';
import PageHeader from '../components/PageHeader.vue';
import DataTable from 'primevue/datatable';
import Column from 'primevue/column';
import Button from 'primevue/button';
import InputText from 'primevue/inputtext';
import Textarea from 'primevue/textarea';
import Dialog from 'primevue/dialog';
import Select from 'primevue/select';
import Tag from 'primevue/tag';

const { t } = useI18n();
const toast = useToast();

interface TemplateVar {
  name: string;
  description: string;
  example: string;
}

interface Template {
  id?: number;
  name: string;
  content: string;
  channel_type: string;
  is_default: boolean;
  description: string;
}

const templates = ref<Template[]>([]);
const loading = ref(false);
const templateDialog = ref(false);
const templateForm = ref<Template>({ name: '', content: '', channel_type: 'all', is_default: false, description: '' });
const isEditTemplate = ref(false);

const variables = ref<TemplateVar[]>([]);
const preview = ref('');
const previewLoading = ref(false);
let previewTimer: ReturnType<typeof setTimeout> | null = null;

const channelTypeOptions = [
  { label: 'All', value: 'all' },
  { label: 'Telegram', value: 'telegram' },
  { label: 'Discord', value: 'discord' },
];

const defaultTemplates = ref<Template[]>([]);

const fetchTemplates = async () => {
  loading.value = true;
  try {
    const res = await api.get('/templates');
    templates.value = res.data;
  } catch {
    toast.add({ severity: 'error', summary: t('common.error'), detail: t('templates.load_error'), life: 3000 });
  } finally {
    loading.value = false;
  }
};

const fetchVariables = async () => {
  try {
    const res = await api.get('/templates/variables');
    variables.value = res.data;
  } catch {
    // non-critical
  }
};

const fetchDefaultTemplates = async () => {
  try {
    const res = await api.get('/templates/defaults');
    defaultTemplates.value = res.data;
  } catch {
    // non-critical
  }
};

const fetchPreview = async () => {
  if (!templateForm.value.content) {
    preview.value = '';
    return;
  }
  previewLoading.value = true;
  try {
    const res = await api.post('/templates/preview', {
      content: templateForm.value.content,
      channel_type: templateForm.value.channel_type,
    });
    preview.value = res.data.preview;
  } catch {
    preview.value = t('templates.preview_error');
  } finally {
    previewLoading.value = false;
  }
};

const debouncedPreview = () => {
  if (previewTimer) clearTimeout(previewTimer);
  previewTimer = setTimeout(fetchPreview, 500);
};

watch(() => templateForm.value.content, debouncedPreview);
watch(() => templateForm.value.channel_type, debouncedPreview);

const openCreateTemplate = () => {
  templateForm.value = { name: '', content: '', channel_type: 'all', is_default: false, description: '' };
  isEditTemplate.value = false;
  preview.value = '';
  templateDialog.value = true;
};

const openEditTemplate = (tmpl: Template) => {
  templateForm.value = { ...tmpl };
  isEditTemplate.value = true;
  preview.value = '';
  templateDialog.value = true;
  // Trigger preview
  debouncedPreview();
};

const saveTemplate = async () => {
  try {
    if (isEditTemplate.value && templateForm.value.id) {
      await api.put(`/templates/${templateForm.value.id}`, templateForm.value);
      toast.add({ severity: 'success', summary: t('common.success'), detail: t('templates.update_success'), life: 3000 });
    } else {
      await api.post('/templates', templateForm.value);
      toast.add({ severity: 'success', summary: t('common.success'), detail: t('templates.create_success'), life: 3000 });
    }
    templateDialog.value = false;
    fetchTemplates();
  } catch (error: any) {
    toast.add({ severity: 'error', summary: t('common.error'), detail: error.response?.data?.error || t('templates.save_error'), life: 3000 });
  }
};

const deleteTemplate = async (tmpl: Template) => {
  if (!confirm(t('common.delete_confirm', { name: tmpl.name }))) return;
  try {
    await api.delete(`/templates/${tmpl.id}`);
    toast.add({ severity: 'success', summary: t('common.success'), detail: t('templates.delete_success'), life: 3000 });
    fetchTemplates();
  } catch (error: any) {
    toast.add({ severity: 'error', summary: t('common.error'), detail: error.response?.data?.error || t('templates.delete_error'), life: 3000 });
  }
};

const insertVariable = (varName: string) => {
  const textarea = document.getElementById('templateContent') as HTMLTextAreaElement | null;
  const insertion = `{{.${varName}}}`;
  if (textarea) {
    const start = textarea.selectionStart;
    const end = textarea.selectionEnd;
    const value = templateForm.value.content;
    templateForm.value.content = value.substring(0, start) + insertion + value.substring(end);
    // Restore cursor position after insertion
    setTimeout(() => {
      textarea.focus();
      textarea.selectionStart = textarea.selectionEnd = start + insertion.length;
    }, 0);
  } else {
    templateForm.value.content += insertion;
  }
};

const loadDefaultTemplate = () => {
  const channelType = templateForm.value.channel_type;
  const match = defaultTemplates.value.find(
    t => t.channel_type === channelType
  ) || defaultTemplates.value.find(
    t => t.channel_type === 'all'
  );
  if (match) {
    templateForm.value.content = match.content;
  }
};

const channelTypeLabel = (type: string) => {
  const opt = channelTypeOptions.find(o => o.value === type);
  return opt ? opt.label : type;
};

const channelTypeSeverity = (type: string): "secondary" | "info" | "success" | "warn" | "danger" | "contrast" | undefined => {
  switch (type) {
    case 'telegram': return 'info';
    case 'discord': return 'success';
    default: return 'secondary';
  }
};

onMounted(() => {
  fetchTemplates();
  fetchVariables();
  fetchDefaultTemplates();
});
</script>

<template>
  <div class="page table-page">
    <PageHeader :title="t('templates.title')">
      <template #actions>
        <Button :label="t('templates.add')" icon="pi pi-plus" class="mr-2" @click="openCreateTemplate" />
        <Button :aria-label="t('common.refresh')" icon="pi pi-refresh" rounded text :loading="loading" @click="fetchTemplates" v-tooltip.bottom="t('common.refresh')" />
      </template>
    </PageHeader>

    <div class="page-panel table-panel">
      <DataTable
        :value="templates"
        :loading="loading"
        stripedRows
        scrollable
        scrollHeight="flex"
        class="table-fill"
      >
        <Column field="name" :header="t('templates.name')"></Column>
        <Column field="channel_type" :header="t('templates.channel_type')" style="width: 120px">
          <template #body="slotProps">
            <Tag :value="channelTypeLabel(slotProps.data.channel_type)" :severity="channelTypeSeverity(slotProps.data.channel_type)" />
          </template>
        </Column>
        <Column field="description" :header="t('templates.description')">
          <template #body="slotProps">
            <span class="text-sm text-surface-500">{{ slotProps.data.description }}</span>
          </template>
        </Column>
        <Column field="content" :header="t('templates.content')">
          <template #body="slotProps">
            <span class="text-xs text-surface-500 line-clamp-2">{{ slotProps.data.content }}</span>
          </template>
        </Column>
        <Column :exportable="false" style="min-width: 10rem">
          <template #body="slotProps">
            <Button icon="pi pi-pencil" outlined rounded class="mr-2" @click="openEditTemplate(slotProps.data)" />
            <Button icon="pi pi-trash" outlined rounded severity="danger" @click="deleteTemplate(slotProps.data)" />
          </template>
        </Column>
      </DataTable>
    </div>

    <!-- Template Editor Dialog -->
    <Dialog v-model:visible="templateDialog" :style="{ width: '1000px', maxWidth: '95vw' }" :header="isEditTemplate ? t('templates.edit') : t('templates.add')" :modal="true" class="p-fluid">
      <!-- Top row: name, channel type, description -->
      <div class="flex gap-4 mb-4">
        <div class="field flex-1">
          <label for="templateName">{{ t('templates.name') }}</label>
          <InputText id="templateName" v-model="templateForm.name" required class="w-full" />
        </div>
        <div class="field" style="width: 160px">
          <label for="templateChannelType">{{ t('templates.channel_type') }}</label>
          <Select id="templateChannelType" v-model="templateForm.channel_type" :options="channelTypeOptions" optionLabel="label" optionValue="value" class="w-full" />
        </div>
      </div>
      <div class="field mb-4">
        <label for="templateDesc">{{ t('templates.description') }}</label>
        <InputText id="templateDesc" v-model="templateForm.description" class="w-full" :placeholder="t('templates.description_hint')" />
      </div>

      <!-- Split pane: editor + preview -->
      <div class="editor-split">
        <div class="editor-left">
          <div class="flex items-center justify-between mb-2">
            <label class="font-medium">{{ t('templates.content') }}</label>
            <Button :label="t('templates.load_default')" icon="pi pi-file" text size="small" @click="loadDefaultTemplate" />
          </div>
          <Textarea id="templateContent" v-model="templateForm.content" rows="14" class="w-full font-mono text-sm" :placeholder="t('templates.content_placeholder')" />

          <!-- Variable tags -->
          <div class="var-tags mt-2">
            <span class="text-sm text-surface-500 mr-2">{{ t('templates.variables') }}:</span>
            <Button
              v-for="v in variables"
              :key="v.name"
              :label="v.name"
              size="small"
              text
              severity="secondary"
              class="var-tag"
              @click="insertVariable(v.name)"
              v-tooltip.top="v.description + ' — ' + v.example"
            />
          </div>
        </div>

        <div class="editor-right">
          <div class="flex items-center justify-between mb-2">
            <label class="font-medium">{{ t('templates.preview') }}</label>
            <i v-if="previewLoading" class="pi pi-spin pi-spinner text-surface-400" />
          </div>
          <div class="preview-box">
            <pre class="preview-content">{{ preview || t('templates.preview_empty') }}</pre>
          </div>
        </div>
      </div>

      <template #footer>
        <Button :label="t('common.cancel')" icon="pi pi-times" outlined @click="templateDialog = false" />
        <Button :label="t('common.save')" icon="pi pi-check" @click="saveTemplate" />
      </template>
    </Dialog>
  </div>
</template>

<style scoped>
.table-fill {
  min-height: 400px;
}

.field label {
  display: block;
  margin-bottom: 0.5rem;
  font-weight: 500;
  color: var(--text-color);
}

.editor-split {
  display: flex;
  gap: 1rem;
  min-height: 400px;
}

.editor-left {
  flex: 1;
  display: flex;
  flex-direction: column;
}

.editor-right {
  flex: 1;
  display: flex;
  flex-direction: column;
}

.preview-box {
  flex: 1;
  border: 1px solid var(--p-surface-300);
  border-radius: var(--p-border-radius);
  padding: 0.75rem;
  background: var(--p-surface-50);
  overflow: auto;
  min-height: 320px;
}

:deep(.app-dark) .preview-box,
.app-dark .preview-box {
  background: var(--p-surface-800);
  border-color: var(--p-surface-600);
}

.preview-content {
  margin: 0;
  white-space: pre-wrap;
  word-break: break-word;
  font-family: inherit;
  font-size: 0.875rem;
  line-height: 1.5;
  color: var(--text-color);
}

.var-tags {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 0.25rem;
}

.var-tag {
  font-size: 0.75rem !important;
  padding: 0.15rem 0.5rem !important;
}

.font-mono {
  font-family: 'JetBrains Mono', 'Fira Code', 'Consolas', monospace;
}

@media (max-width: 768px) {
  .editor-split {
    flex-direction: column;
  }
}
</style>
