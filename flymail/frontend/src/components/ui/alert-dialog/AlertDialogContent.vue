<script setup lang="ts">
import { DialogContent } from '@/components/ui/dialog'
import { cn } from '@/lib/utils'
import type { HTMLAttributes } from 'vue'

interface Props {
  class?: HTMLAttributes['class']
}

const props = defineProps<Props>()
</script>

<template>
  <DialogContent :class="cn('max-w-md', props.class)">
    <slot />
  </DialogContent>
</template>