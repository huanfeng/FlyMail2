<template>
  <div
    v-if="isOpen"
    class="absolute z-50 min-w-[8rem] overflow-hidden rounded-md border bg-popover text-popover-foreground shadow-md data-[state=open]:animate-in data-[state=closed]:animate-out data-[state=closed]:fade-out-0 data-[state=open]:fade-in-0 data-[state=closed]:zoom-out-95 data-[state=open]:zoom-in-95 data-[side=bottom]:slide-in-from-top-2 data-[side=left]:slide-in-from-right-2 data-[side=right]:slide-in-from-left-2 data-[side=top]:slide-in-from-bottom-2"
  >
    <div class="p-1">
      <slot />
    </div>
  </div>
</template>

<script setup lang="ts">
import { inject } from 'vue'

const select = inject<any>('select')
const isOpen = select?.isOpen
</script>