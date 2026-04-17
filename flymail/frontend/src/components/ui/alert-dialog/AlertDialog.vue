<script setup lang="ts">
import { Dialog } from '@/components/ui/dialog'

interface Props {
  open?: boolean
  defaultOpen?: boolean
}

defineProps<Props>()

const emit = defineEmits<{
  'update:open': [value: boolean]
}>()
</script>

<template>
  <Dialog 
    :open="open" 
    :default-open="defaultOpen"
    @update:open="emit('update:open', $event)"
  >
    <slot />
  </Dialog>
</template>