<script setup lang="ts">
import { computed } from 'vue'
import { cn } from '@/lib/utils'

interface SkeletonProps {
  class?: string
}

const props = defineProps<SkeletonProps>()

const skeletonClasses = computed(() =>
  cn('animate-pulse rounded-md bg-muted', props.class)
)
</script>

<template>
  <div :class="skeletonClasses" />
</template>