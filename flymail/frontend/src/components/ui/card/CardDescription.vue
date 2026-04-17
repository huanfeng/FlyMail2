<script setup lang="ts">
import { computed } from 'vue'
import { cn } from '@/lib/utils'

interface CardDescriptionProps {
  class?: string
}

const props = defineProps<CardDescriptionProps>()

const descriptionClasses = computed(() =>
  cn('text-sm text-muted-foreground', props.class)
)
</script>

<template>
  <p :class="descriptionClasses">
    <slot />
  </p>
</template>