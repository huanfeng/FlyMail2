<script setup lang="ts">
import { computed } from 'vue'
import { CheckboxIndicator, CheckboxRoot } from 'reka-ui'
import { Check } from 'lucide-vue-next'
import { cn } from '@/lib/utils'

interface CheckboxProps {
  class?: string
  checked?: boolean | 'indeterminate'
  defaultChecked?: boolean
  disabled?: boolean
  required?: boolean
  name?: string
  value?: string
}

const props = defineProps<CheckboxProps>()

const emits = defineEmits<{
  'update:checked': [value: boolean | 'indeterminate']
}>()

const delegatedProps = computed(() => {
  const { class: _, defaultChecked, ...delegated } = props
  return {
    ...delegated,
    defaultChecked: defaultChecked as boolean | undefined
  }
})

const checkboxClasses = computed(() =>
  cn(
    'peer h-4 w-4 shrink-0 rounded-sm border border-primary ring-offset-background focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 disabled:cursor-not-allowed disabled:opacity-50 data-[state=checked]:bg-primary data-[state=checked]:text-primary-foreground',
    props.class
  )
)

function handleUpdate(checked: boolean | 'indeterminate') {
  emits('update:checked', checked)
}
</script>

<template>
  <CheckboxRoot
    v-bind="delegatedProps"
    :class="checkboxClasses"
    @update:checked="handleUpdate"
  >
    <CheckboxIndicator class="flex items-center justify-center text-current">
      <Check class="h-4 w-4" />
    </CheckboxIndicator>
  </CheckboxRoot>
</template>