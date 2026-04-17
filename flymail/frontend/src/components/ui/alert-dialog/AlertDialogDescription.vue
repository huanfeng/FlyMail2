<script setup lang="ts">
import { DialogDescription } from '@/components/ui/dialog'
import type { HTMLAttributes } from 'vue'

interface Props {
  class?: HTMLAttributes['class']
}

const props = defineProps<Props>()
</script>

<template>
  <DialogDescription :class="props.class">
    <slot />
  </DialogDescription>
</template>