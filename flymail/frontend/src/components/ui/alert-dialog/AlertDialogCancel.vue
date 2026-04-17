<script setup lang="ts">
import { Button } from '@/components/ui/button'
import { DialogClose } from '@/components/ui/dialog'
import type { ButtonProps } from '@/components/ui/button'

interface Props extends /* @vue-ignore */ ButtonProps {
  asChild?: boolean
}

defineProps<Props>()
</script>

<template>
  <DialogClose :as-child="true">
    <Button variant="outline" v-bind="$props">
      <slot />
    </Button>
  </DialogClose>
</template>