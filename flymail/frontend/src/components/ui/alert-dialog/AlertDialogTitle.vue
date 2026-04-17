<script setup lang="ts">
import { DialogTitle } from '@/components/ui/dialog'
import type { HTMLAttributes } from 'vue'

interface Props {
  class?: HTMLAttributes['class']
}

const props = defineProps<Props>()
</script>

<template>
  <DialogTitle :class="props.class">
    <slot />
  </DialogTitle>
</template>