<template>
  <div
    :class="cn(
      'relative flex w-full cursor-default select-none items-center rounded-sm py-1.5 pl-8 pr-2 text-sm outline-none focus:bg-accent focus:text-accent-foreground data-[disabled]:pointer-events-none data-[disabled]:opacity-50',
      props.class
    )"
    @click="handleClick"
  >
    <span class="absolute left-2 flex h-3.5 w-3.5 items-center justify-center">
      <Check v-if="isSelected" class="h-4 w-4" />
    </span>
    <slot />
  </div>
</template>

<script setup lang="ts">
import { inject, computed } from 'vue'
import { Check } from 'lucide-vue-next'
import { cn } from '@/lib/utils'

interface Props {
  value: string | number
  class?: string
}

const props = defineProps<Props>()

const select = inject<any>('select')
const isSelected = computed(() => select?.value === props.value)

const handleClick = () => {
  if (select && !select.disabled) {
    select.setValue(props.value)
  }
}
</script>