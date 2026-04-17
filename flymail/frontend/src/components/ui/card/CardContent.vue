<script setup lang="ts">
import { computed } from 'vue'
import { cn } from '@/lib/utils'

interface CardContentProps {
  class?: string
}

const props = defineProps<CardContentProps>()

const contentClasses = computed(() =>
  cn('p-6 pt-0', props.class)
)
</script>

<template>
  <div :class="contentClasses">
    <slot />
  </div>
</template>