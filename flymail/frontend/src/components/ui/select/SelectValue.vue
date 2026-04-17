<template>
  <span>{{ displayValue || placeholder }}</span>
</template>

<script setup lang="ts">
import { inject, computed } from 'vue'

interface Props {
  placeholder?: string
}

defineProps<Props>()

const select = inject<any>('select')
const displayValue = computed(() => {
  // 如果 select.value 是一个 computed，需要访问它的 value
  const value = select?.value?.value !== undefined ? select.value.value : select?.value
  return value
})
</script>