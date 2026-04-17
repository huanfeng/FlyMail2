<template>
  <div class="relative">
    <slot />
  </div>
</template>

<script setup lang="ts">
import { provide, ref, computed } from 'vue'

interface Props {
  modelValue?: string | number
  disabled?: boolean
}

interface Emits {
  (e: 'update:modelValue', value: string | number): void
}

const props = defineProps<Props>()
const emit = defineEmits<Emits>()

const isOpen = ref(false)

// 使用 computed 使 value 响应式
const currentValue = computed(() => props.modelValue)

provide('select', {
  value: currentValue,
  isOpen,
  disabled: props.disabled,
  setValue: (value: string | number) => {
    emit('update:modelValue', value)
    isOpen.value = false
  }
})
</script>