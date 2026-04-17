<script setup lang="ts">
import { ref, onMounted, onUnmounted } from 'vue';
import api from '../services/api';
import { useI18n } from 'vue-i18n';
import PageHeader from '../components/PageHeader.vue';
import Tag from 'primevue/tag';
import Button from 'primevue/button';

interface WorkerLog {
  time: string;
  level: string;
  state: string;
  message: string;
}

interface Worker {
  account_id: number;
  email: string;
  state: string;
  logs: WorkerLog[];
}

interface Stats {
  total: number;
  workers: Worker[];
}

const stats = ref<Stats>({ total: 0, workers: [] });
const selectedWorker = ref<Worker | null>(null);
let timer: ReturnType<typeof setInterval> | null = null;
const { t } = useI18n();
const refreshing = ref(false);

const fetchStats = async () => {
  try {
    const res = await api.get('/debug/stats');
    stats.value = res.data;

    // Update selected worker if it exists
    if (selectedWorker.value) {
      const updated = stats.value.workers.find(w => w.account_id === selectedWorker.value!.account_id);
      if (updated) {
        selectedWorker.value = updated;
      }
    }
  } catch (e) {
    console.error(e);
  }
};

const getStatusColor = (state: string) => {
  switch (state) {
    case 'idle': return 'bg-green-100 border-green-500 text-green-900';
    case 'polling': return 'bg-blue-100 border-blue-500 text-blue-900';
    case 'error': return 'bg-red-100 border-red-500 text-red-900';
    case 'connecting': return 'bg-yellow-100 border-yellow-500 text-yellow-900';
    case 'disconnected': return 'bg-gray-100 border-gray-300 text-gray-500';
    default: return 'bg-gray-100 border-gray-300 text-gray-500';
  }
};

const selectWorker = (worker: Worker) => {
  selectedWorker.value = worker;
};

const getLevelSeverity = (level: string) => {
  switch (level) {
    case 'error': return 'danger';
    case 'warn': return 'warning';
    case 'debug': return 'info';
    default: return 'success';
  }
};

const refresh = async () => {
  refreshing.value = true;
  await fetchStats();
  refreshing.value = false;
};

onMounted(() => {
  fetchStats();
  timer = setInterval(fetchStats, 2000);
});

onUnmounted(() => {
  if (timer) clearInterval(timer);
});
</script>

<template>
  <div class="page">
    <PageHeader :title="t('menu.debug')">
      <template #actions>
        <Button icon="pi pi-refresh" :aria-label="t('common.refresh')" text :loading="refreshing" @click="refresh" />
      </template>
    </PageHeader>

    <div class="page-panel space-y-6">
      <div class="grid grid-cols-1 md:grid-cols-4 gap-4">
        <div class="bg-white dark:bg-surface-900 p-4 rounded border border-surface-200 dark:border-surface-700">
          <div class="text-gray-500 dark:text-surface-500">{{ t('debug.total_workers') }}</div>
          <div class="text-3xl font-bold">{{ stats.total }}</div>
        </div>
      </div>

      <div class="grid grid-cols-2 md:grid-cols-5 lg:grid-cols-8 gap-2">
        <div
          v-for="worker in stats.workers"
          :key="worker.account_id"
          @click="selectWorker(worker)"
          class="aspect-square rounded flex items-center justify-center cursor-pointer border-2 transition-colors"
          :class="getStatusColor(worker.state)"
        >
          <div class="text-center overflow-hidden w-full">
            <div class="font-bold">#{{ worker.account_id }}</div>
            <div class="text-xs truncate px-1">{{ worker.email }}</div>
            <div class="text-[10px] uppercase mt-1">{{ worker.state }}</div>
          </div>
        </div>

        <div
          v-if="stats.workers.length === 0"
          class="col-span-full text-center py-8 text-gray-400 border-2 border-dashed rounded"
        >
          {{ t('debug.no_workers') }}
        </div>
      </div>

      <div
        v-if="selectedWorker"
        class="bg-gray-900 text-green-400 p-4 rounded shadow h-96 overflow-y-auto font-mono text-sm"
      >
        <div class="flex justify-between border-b border-gray-700 pb-2 mb-2">
          <span class="font-bold">{{ t('debug.logs_for', { email: selectedWorker.email }) }}</span>
          <button @click="selectedWorker = null" class="text-gray-500 hover:text-white">{{ t('debug.close') }}</button>
        </div>
        <div v-for="(log, index) in selectedWorker.logs" :key="index" class="whitespace-pre-wrap flex items-center gap-2 text-sm">
          <span class="text-surface-400">{{ new Date(log.time).toLocaleTimeString() }}</span>
          <Tag :value="log.level" :severity="getLevelSeverity(log.level)" />
          <span>{{ log.message }}</span>
        </div>
      </div>
    </div>
  </div>
</template>
