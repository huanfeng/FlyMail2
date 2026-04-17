<script setup lang="ts">
import { ref, reactive, onMounted, watch } from 'vue';
import Button from 'primevue/button';
import Select from 'primevue/select';
import DatePicker from 'primevue/datepicker';
import { useI18n } from 'vue-i18n';
import PageHeader from '../components/PageHeader.vue';
import { useLayout } from '../layout/composables/layout';
import { useToast } from 'primevue/usetoast';
import ToggleSwitch from 'primevue/toggleswitch';
import api from '../services/api';
import Checkbox from 'primevue/checkbox';
import Dialog from 'primevue/dialog';
import axios from 'axios';
import Password from 'primevue/password';
import { getString, setString, KEYS } from '../utils/storage';

const { t, locale } = useI18n();
const { layoutConfig, setPrimary } = useLayout();
const toast = useToast();

const colors = [
  { name: 'Emerald', value: 'emerald' },
  { name: 'Blue', value: 'blue' },
  { name: 'Purple', value: 'purple' },
  { name: 'Amber', value: 'amber' },
  { name: 'Cyan', value: 'cyan' }
];

const appearance = reactive({
  primary: layoutConfig.primary
});

const languageOptions = [
  { label: 'English', value: 'en' },
  { label: '中文', value: 'zh' }
];

const selectedLang = ref(locale.value);
const selectedTimezone = ref('UTC');
const rawTimezones = ref<string[]>([]);
const timezoneOptions = ref<{ label: string; value: string }[]>([]);
const serverTime = ref('');

type TimeWindow = {
  enabled: boolean;
  start: Date | null;
  end: Date | null;
};

const quiet: TimeWindow = reactive({
  enabled: false,
  start: null,
  end: null
});

const night: TimeWindow = reactive({
  enabled: false,
  start: null,
  end: null
});

const loading = ref(false);
const refreshing = ref(false);
const exporting = ref(false);
const importing = ref(false);
const importFileInput = ref<HTMLInputElement | null>(null);
const exportDialog = ref(false);
const importDialog = ref(false);
const importConfirmDialog = ref(false);
const importFileName = ref('');
const exportPassword = ref('');
const exportPasswordFlash = ref(false);
const exportPasswordError = ref('');
const exportSelections = reactive({
  accounts: true,
  proxies: true,
  channels: true,
  settings: true
});
const importSelections = reactive({
  accounts: true,
  proxies: true,
  channels: true,
  settings: true
});
const availableImport = reactive({
  accounts: false,
  proxies: false,
  channels: false,
  settings: false
});
const parsedImportData = ref<any>(null);
const importConflicts = ref<{ accounts?: string[]; proxies?: string[]; channels?: string[] }>({});
const lastImportPayload = ref<any>(null);

const parseTime = (value?: string | null) => {
  if (!value) return null;
  const [hStr = '0', mStr = '0'] = value.split(':');
  const h = Number.parseInt(hStr, 10);
  const m = Number.parseInt(mStr, 10);
  if (Number.isNaN(h) || Number.isNaN(m)) return null;
  const date = new Date();
  date.setHours(h, m, 0, 0);
  return date;
};

const formatTime = (date: Date | null) => {
  if (!date) return '';
  const h = String(date.getHours()).padStart(2, '0');
  const m = String(date.getMinutes()).padStart(2, '0');
  return `${h}:${m}`;
};

const getTimezoneOffsetMinutes = (tz: string) => {
  try {
    const now = new Date();
    const utc = new Date(now.toLocaleString('en-US', { timeZone: 'UTC' }));
    const zoned = new Date(now.toLocaleString('en-US', { timeZone: tz }));
    return Math.round((zoned.getTime() - utc.getTime()) / 60000);
  } catch {
    return 0;
  }
};

const formatOffsetLabel = (minutes: number) => {
  const sign = minutes >= 0 ? '+' : '-';
  const abs = Math.abs(minutes);
  const hours = String(Math.floor(abs / 60)).padStart(2, '0');
  const mins = String(abs % 60).padStart(2, '0');
  return `${sign}${hours}:${mins}`;
};

const resolveTimezoneName = (tz: string) => {
  const targetLocale = selectedLang.value || locale.value;
  try {
    const formatter = new Intl.DateTimeFormat(targetLocale, { timeZone: tz, timeZoneName: 'longGeneric' });
    const part = formatter.formatToParts(new Date()).find((p) => p.type === 'timeZoneName');
    if (part?.value && part.value !== tz) {
      return part.value;
    }
  } catch {
  }
  try {
    if (typeof Intl.DisplayNames === 'function') {
      const displayNames = new Intl.DisplayNames([targetLocale], { type: 'timeZone' as any });
      const name = displayNames.of(tz);
      if (name) return name;
    }
  } catch {
    // fall through
  }
  return tz;
};

const buildTimezoneOptions = (list?: string[]) => {
  if (list) rawTimezones.value = list;
  const source = rawTimezones.value.length ? rawTimezones.value : [selectedTimezone.value];
  const unique = Array.from(new Set(source)).filter(Boolean);
  const enriched = unique.map((tz) => {
    const offsetMinutes = getTimezoneOffsetMinutes(tz);
    const offset = formatOffsetLabel(offsetMinutes);
    const name = resolveTimezoneName(tz);
    const label = t('settings.timezone_option', { offset, name });
    return { tz, offsetMinutes, label };
  });

  enriched.sort((a, b) => {
    if (a.offsetMinutes !== b.offsetMinutes) return a.offsetMinutes - b.offsetMinutes;
    return a.label.localeCompare(b.label);
  });

  const labelCount = new Map<string, number>();
  enriched.forEach(({ label }) => {
    labelCount.set(label, (labelCount.get(label) || 0) + 1);
  });

  timezoneOptions.value = enriched.map(({ tz, label }) => {
    const needsDetail = (labelCount.get(label) || 0) > 1;
    const finalLabel = needsDetail ? `${label} - ${tz}` : label;
    return { value: tz, label: finalLabel };
  });
};

const loadPreferences = async () => {
  appearance.primary = layoutConfig.primary;
  selectedLang.value = getString(KEYS.UI_LANGUAGE, locale.value);
  loading.value = true;
  try {
    const res = await api.get('/config');
    const data = res.data || {};
    selectedTimezone.value = data.timezone || 'UTC';
    serverTime.value = data.server_time || '';
    buildTimezoneOptions(data.timezones || [selectedTimezone.value, 'UTC']);
    quiet.enabled = !!data.quiet_enabled;
    quiet.start = parseTime(data.quiet_start);
    quiet.end = parseTime(data.quiet_end);
    night.enabled = !!data.night_enabled;
    night.start = parseTime(data.night_start);
    night.end = parseTime(data.night_end);
  } catch (e) {
    console.error(e);
    toast.add({ severity: 'error', summary: t('common.error'), detail: t('settings.saved_error'), life: 2000 });
  } finally {
    loading.value = false;
  }
};

const saveSettings = async () => {
  loading.value = true;
  try {
    setPrimary(appearance.primary);
    locale.value = selectedLang.value;
    setString(KEYS.UI_LANGUAGE, selectedLang.value);
    await api.post('/config', {
      timezone: selectedTimezone.value,
      quiet_enabled: quiet.enabled,
      quiet_start: formatTime(quiet.start),
      quiet_end: formatTime(quiet.end),
      night_enabled: night.enabled,
      night_start: formatTime(night.start),
      night_end: formatTime(night.end)
    });
    await loadPreferences();
    toast.add({ severity: 'success', summary: t('common.success'), detail: t('settings.saved_success'), life: 2000 });
  } catch (e) {
    console.error(e);
    toast.add({ severity: 'error', summary: t('common.error'), detail: t('settings.saved_error'), life: 2000 });
  } finally {
    loading.value = false;
  }
};

onMounted(() => {
  loadPreferences();
});

const refreshSettings = async () => {
  refreshing.value = true;
  await loadPreferences();
  refreshing.value = false;
};

watch(
  () => locale.value,
  () => {
    buildTimezoneOptions();
  }
);

watch(selectedLang, () => {
  buildTimezoneOptions();
});

const selectedSections = (selection: { [key: string]: boolean }) => Object.entries(selection).filter(([, enabled]) => enabled).map(([key]) => key);

const openExportDialog = () => {
  exportSelections.accounts = true;
  exportSelections.proxies = true;
  exportSelections.channels = true;
  exportSelections.settings = true;
  exportPassword.value = '';
  exportPasswordError.value = '';
  exportPasswordFlash.value = false;
  exportDialog.value = true;
};

const triggerExportPasswordFlash = () => {
  exportPasswordFlash.value = false;
  requestAnimationFrame(() => {
    exportPasswordFlash.value = true;
    setTimeout(() => {
      exportPasswordFlash.value = false;
    }, 1200);
  });
};

const exportConfig = async () => {
  const sections = selectedSections(exportSelections);
  if (!sections.length) {
    toast.add({ severity: 'warn', summary: t('common.warning'), detail: t('settings.export_select_warning'), life: 2000 });
    return;
  }
  if (sections.includes('accounts') && !exportPassword.value) {
    exportPasswordError.value = t('settings.export_password_required');
    triggerExportPasswordFlash();
    return;
  }
  exportPasswordError.value = '';
  exportPasswordFlash.value = false;

  exporting.value = true;
  try {
    const res = await api.post('/config/export', { sections, password: sections.includes('accounts') ? exportPassword.value : '' });
    const blob = new Blob([JSON.stringify(res.data, null, 2)], { type: 'application/json' });
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = `mail2im_export_${new Date().toISOString()}.json`;
    document.body.appendChild(a);
    a.click();
    document.body.removeChild(a);
    URL.revokeObjectURL(url);
    toast.add({ severity: 'success', summary: t('common.success'), detail: t('settings.export_success'), life: 2500 });
    exportDialog.value = false;
    exportPassword.value = '';
  } catch (e) {
    console.error(e);
    if (axios.isAxiosError(e) && e.response?.data?.error === 'invalid_password') {
      exportPasswordError.value = t('settings.export_password_invalid');
      triggerExportPasswordFlash();
    } else {
      toast.add({ severity: 'error', summary: t('common.error'), detail: t('settings.export_error'), life: 2500 });
    }
  } finally {
    exporting.value = false;
  }
};

const clearExportPasswordError = () => {
  exportPasswordError.value = '';
  exportPasswordFlash.value = false;
};

const openImportDialog = () => {
  importSelections.accounts = false;
  importSelections.proxies = false;
  importSelections.channels = false;
  importSelections.settings = false;
  availableImport.accounts = false;
  availableImport.proxies = false;
  availableImport.channels = false;
  availableImport.settings = false;
  parsedImportData.value = null;
  importFileName.value = '';
  importDialog.value = true;
  importConfirmDialog.value = false;
  importConflicts.value = {};
  lastImportPayload.value = null;
};

const triggerImport = () => {
  importFileInput.value?.click();
};

const handleImportFile = async (event: Event) => {
  const input = event.target as HTMLInputElement;
  const file = input.files?.[0];
  if (!file) return;

  let json: any = null;
  try {
    const text = await file.text();
    json = JSON.parse(text);
  } catch {
    toast.add({ severity: 'error', summary: t('common.error'), detail: t('settings.import_parse_error'), life: 2500 });
    input.value = '';
    return;
  }

  parsedImportData.value = json;
  importFileName.value = file.name;
  availableImport.accounts = Array.isArray(json.accounts) && json.accounts.length > 0;
  availableImport.proxies = Array.isArray(json.proxies) && json.proxies.length > 0;
  availableImport.channels = Array.isArray(json.channels) && json.channels.length > 0;
  availableImport.settings = !!json.system_settings && Object.keys(json.system_settings).length > 0;

  importSelections.accounts = availableImport.accounts;
  importSelections.proxies = availableImport.proxies;
  importSelections.channels = availableImport.channels;
  importSelections.settings = availableImport.settings;

  if (!availableImport.accounts && !availableImport.proxies && !availableImport.settings && !availableImport.channels) {
    toast.add({ severity: 'warn', summary: t('common.warning'), detail: t('settings.import_empty_warning'), life: 2500 });
    input.value = '';
    parsedImportData.value = null;
    importFileName.value = '';
  }
};

const buildImportPayload = () => {
  if (!parsedImportData.value) return null;
  const payload: any = {};
  if (importSelections.accounts && availableImport.accounts) payload.accounts = parsedImportData.value.accounts;
  if (importSelections.proxies && availableImport.proxies) payload.proxies = parsedImportData.value.proxies;
  if (importSelections.channels && availableImport.channels) payload.channels = parsedImportData.value.channels;
  if (importSelections.settings && availableImport.settings) payload.system_settings = parsedImportData.value.system_settings;
  return payload;
};

const performImport = async (overwrite = false, payloadOverride?: any) => {
  const payload = payloadOverride || buildImportPayload();
  if (!payload || Object.keys(payload).length === 0) {
    toast.add({ severity: 'warn', summary: t('common.warning'), detail: t('settings.import_select_warning'), life: 2500 });
    return;
  }

  importing.value = true;
  try {
    const body = { ...payload, overwrite };
    await api.post('/config/import', body);
    toast.add({ severity: 'success', summary: t('common.success'), detail: t('settings.import_success'), life: 2500 });
    importDialog.value = false;
    importConfirmDialog.value = false;
    importFileName.value = '';
    parsedImportData.value = null;
    await loadPreferences();
  } catch (e: any) {
    console.error(e);
    if (axios.isAxiosError(e) && e.response?.status === 409) {
      lastImportPayload.value = buildImportPayload();
      importConflicts.value = e.response.data?.conflicts || {};
      importConfirmDialog.value = true;
    } else {
      toast.add({ severity: 'error', summary: t('common.error'), detail: t('settings.import_error'), life: 2500 });
    }
  } finally {
    importing.value = false;
  }
};

const confirmOverwriteImport = async () => {
  const payload = lastImportPayload.value || buildImportPayload();
  if (!payload) {
    importConfirmDialog.value = false;
    return;
  }
  await performImport(true, payload);
};
</script>

<template>
  <div class="page">
    <PageHeader :title="t('settings.title')">
      <template #actions>
        <Button :label="t('common.cancel')" text class="mr-2" @click="loadPreferences" :disabled="loading" />
        <Button :label="t('settings.save')" icon="pi pi-save" @click="saveSettings" :loading="loading" class="mr-2" />
        <Button icon="pi pi-refresh" rounded text :loading="refreshing" @click="refreshSettings" :aria-label="t('common.refresh')" v-tooltip.bottom="t('common.refresh')" />
      </template>
    </PageHeader>

    <div class="page-panel settings-panel">
      <div class="settings-scroll">
        <div class="section">
          <div class="section-head">
            <div>
              <div class="section-title">{{ t('settings.appearance') }}</div>
              <div class="section-desc">{{ t('settings.appearance_desc') }}</div>
            </div>
          </div>
          <div class="setting-row">
            <div class="setting-label">{{ t('settings.theme_color') }}</div>
            <div class="setting-content">
              <div class="color-grid">
                <button
                  v-for="color in colors"
                  :key="color.value"
                  type="button"
                  class="color-dot"
                  :class="{ active: appearance.primary === color.value }"
                  :style="{ backgroundColor: `var(--p-${color.value}-500)` }"
                  @click="appearance.primary = color.value"
                >
                  <i v-if="appearance.primary === color.value" class="pi pi-check text-white text-sm"></i>
                </button>
              </div>
              <small class="text-muted">{{ t('settings.theme_hint') }}</small>
            </div>
          </div>

        </div>

        <div class="section">
          <div class="section-head">
            <div>
              <div class="section-title">{{ t('settings.locale_block') }}</div>
              <div class="section-desc">{{ t('settings.locale_desc') }}</div>
            </div>
          </div>
          <div class="setting-row">
            <div class="setting-label">{{ t('settings.language') }}</div>
            <div class="setting-content">
              <Select v-model="selectedLang" :options="languageOptions" optionLabel="label" optionValue="value" class="w-full sm:w-60" />
            </div>
          </div>

          <div class="setting-row">
            <div class="setting-label">{{ t('settings.timezone') }}</div>
            <div class="setting-content">
              <Select
                v-model="selectedTimezone"
                :options="timezoneOptions"
                optionLabel="label"
                optionValue="value"
                filter
                class="w-full"
              />
              <small class="text-muted">{{ t('settings.server_time', { time: serverTime || '--' }) }}</small>
            </div>
          </div>

        </div>

        <div class="section">
          <div class="section-head">
            <div>
              <div class="section-title">{{ t('settings.delivery_block') }}</div>
              <div class="section-desc">{{ t('settings.delivery_desc') }}</div>
            </div>
          </div>

          <div class="setting-row">
            <div class="setting-label">{{ t('settings.quiet') }}</div>
            <div class="setting-content gap-3">
              <div class="flex items-center gap-3">
                <ToggleSwitch v-model="quiet.enabled" />
                <span>{{ t('settings.quiet_enable') }}</span>
              </div>
              <div class="time-row">
                <DatePicker
                  v-model="quiet.start"
                  timeOnly
                  hourFormat="24"
                  showIcon
                  :placeholder="t('settings.quiet_start')"
                  inputClass="w-full"
                  class="w-full"
                  :disabled="!quiet.enabled"
                />
                <DatePicker
                  v-model="quiet.end"
                  timeOnly
                  hourFormat="24"
                  showIcon
                  :placeholder="t('settings.quiet_end')"
                  inputClass="w-full"
                  class="w-full"
                  :disabled="!quiet.enabled"
                />
              </div>
              <small class="text-muted">{{ t('settings.quiet_desc') }}</small>
            </div>
          </div>

          <div class="setting-row">
            <div class="setting-label">{{ t('settings.night_window') }}</div>
            <div class="setting-content gap-3">
              <div class="flex items-center gap-3">
                <ToggleSwitch v-model="night.enabled" />
                <span>{{ t('settings.night_enable') }}</span>
              </div>
              <div class="time-row">
                <DatePicker
                  v-model="night.start"
                  timeOnly
                  hourFormat="24"
                  showIcon
                  :placeholder="t('settings.night_start')"
                  inputClass="w-full"
                  class="w-full"
                  :disabled="!night.enabled"
                />
                <DatePicker
                  v-model="night.end"
                  timeOnly
                  hourFormat="24"
                  showIcon
                  :placeholder="t('settings.night_end')"
                  inputClass="w-full"
                  class="w-full"
                  :disabled="!night.enabled"
                />
              </div>
              <small class="text-muted">{{ t('settings.night_desc') }}</small>
            </div>
          </div>
        </div>

        <div class="section">
          <div class="section-head">
            <div>
              <div class="section-title">{{ t('settings.data_transfer') }}</div>
              <div class="section-desc">{{ t('settings.data_transfer_desc') }}</div>
            </div>
            <div class="flex gap-2">
              <Button icon="pi pi-download" :label="t('settings.export_btn')" outlined @click="openExportDialog" />
              <Button icon="pi pi-upload" :label="t('settings.import_btn')" @click="openImportDialog" />
            </div>
          </div>
          <small class="text-muted">{{ t('settings.data_transfer_hint') }}</small>
        </div>

      </div>

    </div>

    <Dialog v-model:visible="exportDialog" modal :header="t('settings.export_title')" :style="{ width: '40rem' }">
      <form id="exportForm" class="dialog-body" @submit.prevent="exportConfig">
        <div class="dialog-row">
          <div class="dialog-label">{{ t('settings.export_title') }}</div>
          <div class="dialog-content">
            <div class="checkbox-grid">
              <label class="checkbox-item">
                <Checkbox v-model="exportSelections.accounts" binary inputId="export-accounts" />
                <span>{{ t('settings.export_accounts') }}</span>
              </label>
              <label class="checkbox-item">
                <Checkbox v-model="exportSelections.proxies" binary inputId="export-proxies" />
                <span>{{ t('settings.export_proxies') }}</span>
              </label>
              <label class="checkbox-item">
                <Checkbox v-model="exportSelections.channels" binary inputId="export-channels" />
                <span>{{ t('settings.export_channels') }}</span>
              </label>
              <label class="checkbox-item">
                <Checkbox v-model="exportSelections.settings" binary inputId="export-settings" />
                <span>{{ t('settings.export_system') }}</span>
              </label>
            </div>
            <span class="text-muted">{{ t('settings.export_hint') }}</span>
          </div>
        </div>

        <div class="dialog-row">
          <div class="dialog-label inline-label">
            <span>{{ t('settings.export_password') }}</span>
            <i class="pi pi-info-circle info-icon" v-tooltip.bottom="t('settings.export_password_hint')" />
          </div>
          <div class="dialog-content">
            <Password
              v-model="exportPassword"
              :feedback="false"
              toggleMask
              :disabled="!exportSelections.accounts"
              :inputProps="{ autocomplete: 'current-password' }"
              :inputClass="['w-full', exportPasswordError ? 'p-invalid' : '']"
              :class="['w-full', exportPasswordError ? 'password-invalid' : '']"
              @focus="clearExportPasswordError"
            />
            <div class="hint-line" :class="{ 'flash-error': exportPasswordFlash }">
              <small v-if="exportPasswordError" class="text-error">{{ exportPasswordError }}</small>
            </div>
          </div>
        </div>

        <div class="dialog-actions">
          <Button :label="t('common.cancel')" text @click="exportDialog = false" />
          <Button type="submit" form="exportForm" :label="t('settings.export_btn')" icon="pi pi-download" :loading="exporting" />
        </div>
      </form>
    </Dialog>

    <Dialog v-model:visible="importDialog" modal :header="t('settings.import_title')" :style="{ width: '40rem' }">
      <div class="dialog-body">
        <div class="dialog-row">
          <div class="dialog-label">{{ t('settings.import_select_file') }}</div>
          <div class="dialog-content">
            <div class="file-row">
              <input ref="importFileInput" type="file" accept="application/json" class="hidden-input" @change="handleImportFile" />
              <Button icon="pi pi-folder-open" :label="t('settings.import_select_file')" outlined @click="triggerImport" />
              <span class="text-muted truncate" v-if="importFileName">{{ importFileName }}</span>
              <span class="text-muted" v-else>{{ t('settings.import_no_file') }}</span>
            </div>
          </div>
        </div>

        <div class="dialog-row">
          <div class="dialog-label">{{ t('settings.import_sections') }}</div>
          <div class="dialog-content">
            <div class="checkbox-grid">
              <label class="checkbox-item" :class="{ disabled: !availableImport.accounts }">
                <Checkbox v-model="importSelections.accounts" binary inputId="import-accounts" :disabled="!availableImport.accounts" />
                <span>{{ t('settings.export_accounts') }}</span>
              </label>
              <label class="checkbox-item" :class="{ disabled: !availableImport.proxies }">
                <Checkbox v-model="importSelections.proxies" binary inputId="import-proxies" :disabled="!availableImport.proxies" />
                <span>{{ t('settings.export_proxies') }}</span>
              </label>
              <label class="checkbox-item" :class="{ disabled: !availableImport.channels }">
                <Checkbox v-model="importSelections.channels" binary inputId="import-channels" :disabled="!availableImport.channels" />
                <span>{{ t('settings.export_channels') }}</span>
              </label>
              <label class="checkbox-item" :class="{ disabled: !availableImport.settings }">
                <Checkbox v-model="importSelections.settings" binary inputId="import-settings" :disabled="!availableImport.settings" />
                <span>{{ t('settings.export_system') }}</span>
              </label>
            </div>
            <small class="text-muted">{{ t('settings.import_hint') }}</small>
          </div>
        </div>

        <div class="dialog-actions">
          <Button :label="t('common.cancel')" text @click="importDialog = false" />
          <Button :label="t('settings.import_start')" icon="pi pi-upload" :loading="importing" :disabled="!parsedImportData" @click="performImport(false)" />
        </div>
      </div>
    </Dialog>

    <Dialog v-model:visible="importConfirmDialog" modal :header="t('settings.import_conflict_title')" :style="{ width: '32rem' }">
      <div class="flex flex-column gap-3">
        <p class="text-muted">{{ t('settings.import_conflict_desc') }}</p>
        <div v-if="importConflicts.accounts?.length" class="conflict-block">
          <div class="block-title">{{ t('menu.accounts') }}</div>
          <ul>
            <li v-for="item in importConflicts.accounts" :key="item">{{ item }}</li>
          </ul>
        </div>
        <div v-if="importConflicts.proxies?.length" class="conflict-block">
          <div class="block-title">{{ t('menu.proxies') }}</div>
          <ul>
            <li v-for="item in importConflicts.proxies" :key="item">{{ item }}</li>
          </ul>
        </div>
        <div v-if="importConflicts.channels?.length" class="conflict-block">
          <div class="block-title">{{ t('menu.channels') }}</div>
          <ul>
            <li v-for="item in importConflicts.channels" :key="item">{{ item }}</li>
          </ul>
        </div>
        <div class="flex justify-end gap-2">
          <Button :label="t('common.cancel')" text @click="importConfirmDialog = false" />
          <Button :label="t('settings.import_overwrite')" icon="pi pi-check" :loading="importing" severity="warning" @click="confirmOverwriteImport" />
        </div>
      </div>
    </Dialog>
  </div>
</template>

<style scoped>
.settings-panel {
  display: flex;
  flex-direction: column;
  height: calc(100vh - 103px);
  overflow: hidden;
  padding: 0;
}

.settings-scroll {
  flex: 1;
  min-height: 0;
  overflow: auto;
  padding: 0.75rem 1rem 1.25rem;
  display: flex;
  flex-direction: column;
  gap: 1.25rem;
}

.section {
  display: flex;
  flex-direction: column;
  gap: 1rem;
  padding-bottom: 1rem;
  border-bottom: 1px solid var(--p-surface-200);
}

.section:last-of-type {
  border-bottom: none;
  padding-bottom: 0.5rem;
}

.section-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
}

.section-title {
  font-size: 1.1rem;
  font-weight: var(--fw-heading);
}

.section-desc {
  color: var(--p-text-muted-color, #64748b);
  font-size: 0.95rem;
}

.setting-row {
  display: grid;
  grid-template-columns: 220px 1fr;
  gap: 1rem;
  align-items: start;
}

.setting-label {
  font-weight: 600;
  color: var(--p-text-color);
}

.setting-content {
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
}

.color-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(60px, 1fr));
  gap: 0.5rem;
  max-width: 380px;
}

.color-dot {
  height: 44px;
  border-radius: 12px;
  border: 2px solid transparent;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  cursor: pointer;
  box-shadow: 0 4px 12px rgba(0, 0, 0, 0.1);
  transition: transform 0.15s, box-shadow 0.15s, border-color 0.15s;
}

.color-dot:hover {
  transform: translateY(-2px);
  box-shadow: 0 8px 20px rgba(0, 0, 0, 0.12);
}

.color-dot.active {
  border-color: var(--p-surface-0);
  outline: 2px solid var(--p-primary-400);
}

.text-muted {
  color: var(--p-text-muted-color, #94a3b8);
}

.time-row {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(180px, 1fr));
  gap: 0.75rem;
}

.checkbox-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(160px, 1fr));
  gap: 0.5rem 1rem;
}

.checkbox-item {
  display: inline-flex;
  align-items: center;
  gap: 0.5rem;
  font-weight: 500;
}

.checkbox-item.disabled {
  opacity: 0.5;
}

.hidden-input {
  display: none;
}

.dialog-body {
  display: flex;
  flex-direction: column;
  gap: 1rem;
}

.dialog-row {
  display: grid;
  grid-template-columns: 140px 1fr;
  gap: 0.75rem;
  align-items: start;
}

.dialog-label {
  font-weight: 600;
  color: var(--p-text-color);
  margin-top: 0.35rem;
}

.dialog-content {
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
}

.inline-label {
  display: inline-flex;
  align-items: center;
  gap: 0.35rem;
}

.info-icon {
  color: var(--p-text-muted-color, #94a3b8);
  font-size: 0.9rem;
  cursor: pointer;
}

.hint-line {
  min-height: 1.25rem;
}

.password-invalid :deep(.p-inputtext) {
  border-color: var(--p-red-500, #ef4444);
}

@keyframes flashError {
  0%, 100% { color: var(--p-red-500, #ef4444); }
  50% { color: var(--p-red-700, #b91c1c); }
}

.flash-error {
  animation: flashError 0.9s ease-in-out 2;
}

.dialog-actions {
  display: flex;
  justify-content: flex-end;
  gap: 0.5rem;
}

.file-row {
  display: flex;
  flex-wrap: wrap;
  gap: 0.5rem;
  align-items: center;
}

.conflict-block {
  padding: 0.5rem 0.75rem;
  border: 1px solid var(--p-surface-200);
  border-radius: 8px;
  background: var(--p-surface-0);
}

.conflict-block ul {
  margin: 0.25rem 0 0;
  padding-left: 1.25rem;
}

.truncate {
  max-width: 200px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.app-dark .settings-panel {
  background: var(--p-surface-900);
}

.app-dark .section {
  border-color: var(--p-surface-700);
}

@media (max-width: 768px) {
  .settings-panel {
    height: auto;
  }

  .setting-row {
    grid-template-columns: 1fr;
    align-items: flex-start;
  }

  .setting-label {
    margin-bottom: -0.35rem;
  }
}
</style>
