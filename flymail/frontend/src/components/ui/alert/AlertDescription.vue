<script setup lang="ts">
import { computed } from 'vue'
import { cn } from '@/lib/utils'

interface AlertDescriptionProps {
  class?: string
}

const props = defineProps<AlertDescriptionProps>()

const descriptionClasses = computed(() =>
  cn('text-sm [&_p]:leading-relaxed', props.class)
)
</script>

<template>
  <div :class="descriptionClasses">
    <slot />
  </div>
</template>