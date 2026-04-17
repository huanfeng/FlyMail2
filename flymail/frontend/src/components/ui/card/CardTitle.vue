<script setup lang="ts">
import { computed } from 'vue'
import { cn } from '@/lib/utils'

interface CardTitleProps {
  class?: string
  as?: string
}

const props = withDefaults(defineProps<CardTitleProps>(), {
  as: 'h3'
})

const titleClasses = computed(() =>
  cn('text-2xl font-semibold leading-none tracking-tight', props.class)
)
</script>

<template>
  <component :is="as" :class="titleClasses">
    <slot />
  </component>
</template>