<script setup lang="ts">
import { DialogHeader } from '@/components/ui/dialog'
import type { HTMLAttributes } from 'vue'

interface Props {
  class?: HTMLAttributes['class']
}

const props = defineProps<Props>()
</script>

<template>
  <DialogHeader :class="props.class">
    <slot />
  </DialogHeader>
</template>