<script setup lang="ts">
import { computed } from 'vue'
import { cn } from '@/lib/utils'

interface CardFooterProps {
  class?: string
}

const props = defineProps<CardFooterProps>()

const footerClasses = computed(() =>
  cn('flex items-center p-6 pt-0', props.class)
)
</script>

<template>
  <div :class="footerClasses">
    <slot />
  </div>
</template>