<script setup lang="ts">
const props = defineProps({
  title: {
    type: String,
    required: true
  },
  subtitle: {
    type: String,
    default: ''
  },
  showBack: {
    type: Boolean,
    default: false
  },
  searchValue: {
    type: String,
    default: ''
  },
  searchPlaceholder: {
    type: String,
    default: ''
  },
  showSearch: {
    type: Boolean,
    default: false
  },
  loading: {
    type: Boolean,
    default: false
  }
});

const emit = defineEmits<{
  (e: 'update:searchValue', value: string): void;
  (e: 'search'): void;
  (e: 'back'): void;
}>();

const updateSearch = (value: string | undefined) => {
  emit('update:searchValue', value ?? '');
};

const triggerSearch = () => {
  emit('search');
};
</script>

<template>
  <div class="page-header">
    <div class="page-header-left">
      <Button
        v-if="showBack"
        icon="pi pi-arrow-left"
        text
        rounded
        @click="$emit('back')"
        :aria-label="$t('common.back')"
        class="back-btn"
      />
      <div class="page-header-title">
        <h1>{{ title }}</h1>
        <p v-if="subtitle">{{ subtitle }}</p>
      </div>
    </div>
    <div class="page-header-actions">
      <slot name="actions" />
      <div v-if="showSearch" class="page-header-search">
        <span class="relative w-full">
          <i class="pi pi-search absolute top-2/4 -mt-2 left-3 text-surface-400 dark:text-surface-600" />
          <InputText
            :model-value="props.searchValue"
            class="pl-10 w-full"
            :placeholder="props.searchPlaceholder"
            @update:model-value="updateSearch"
            @keydown.enter="triggerSearch"
          />
        </span>
        <Button icon="pi pi-search" :loading="loading" outlined @click="triggerSearch" />
      </div>
    </div>
  </div>
</template>

<style scoped>
.page-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 1rem;
  padding: 0.25rem 0.75rem;
}

.page-header-left {
  display: flex;
  align-items: center;
  gap: 0.5rem;
}

.back-btn {
  margin-right: 0.35rem;
}

.page-header-title h1 {
  margin: 0;
  font-size: 1.5rem;
  font-weight: var(--fw-heading);
}

.page-header-title p {
  margin: 0.25rem 0 0;
  color: var(--p-text-muted-color);
}

.page-header-actions {
  display: flex;
  align-items: center;
  gap: 0.75rem;
  flex-wrap: wrap;
  justify-content: flex-end;
}

.page-header-search {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  min-width: 260px;
}

.page-header-search .p-inputtext {
  width: 100%;
  padding-left: 2.75rem;
  height: 2.75rem;
}

.page-header-search .pi-search {
  left: 0.9rem;
}
</style>
