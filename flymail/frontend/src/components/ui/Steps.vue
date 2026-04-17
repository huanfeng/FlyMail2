<script lang="ts" setup>
import { computed } from 'vue'
import { Check } from 'lucide-vue-next'
import { cn } from '@/lib/utils'

interface Step {
  title: string
  description?: string
}

interface Props {
  current: number
  steps: Step[]
}

const props = defineProps<Props>()

const stepsWithStatus = computed(() => {
  return props.steps.map((step, index) => {
    const stepNumber = index + 1
    let status: 'completed' | 'current' | 'pending' = 'pending'
    
    if (stepNumber < props.current) {
      status = 'completed'
    } else if (stepNumber === props.current) {
      status = 'current'
    }
    
    return {
      ...step,
      number: stepNumber,
      status
    }
  })
})
</script>

<template>
  <div class="flex items-center justify-between">
    <div 
      v-for="(step, index) in stepsWithStatus" 
      :key="step.number"
      class="flex items-center"
    >
      <!-- Step indicator -->
      <div class="flex items-center">
        <div 
          :class="cn(
            'flex items-center justify-center w-8 h-8 border-2 rounded-full text-sm font-medium',
            step.status === 'completed' && 'bg-primary border-primary text-primary-foreground',
            step.status === 'current' && 'border-primary text-primary bg-background',
            step.status === 'pending' && 'border-muted-foreground text-muted-foreground bg-background'
          )"
        >
          <Check v-if="step.status === 'completed'" class="h-4 w-4" />
          <span v-else>{{ step.number }}</span>
        </div>
        <div class="ml-3 hidden sm:block">
          <div 
            :class="cn(
              'text-sm font-medium',
              step.status === 'completed' && 'text-primary',
              step.status === 'current' && 'text-primary',
              step.status === 'pending' && 'text-muted-foreground'
            )"
          >
            {{ step.title }}
          </div>
          <div v-if="step.description" class="text-xs text-muted-foreground">
            {{ step.description }}
          </div>
        </div>
      </div>
      
      <!-- Connector line -->
      <div 
        v-if="index < stepsWithStatus.length - 1"
        :class="cn(
          'flex-1 h-0.5 mx-4',
          step.status === 'completed' ? 'bg-primary' : 'bg-muted'
        )"
      />
    </div>
  </div>
</template>