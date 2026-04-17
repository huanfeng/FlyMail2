<script setup lang="ts">
// @ts-ignore TransitionGroup is used in template
import { TransitionGroup } from 'vue'
import Toast from './Toast.vue'
import { useToast } from '@/components/ui/toast/useToast'

const { toasts, removeToast } = useToast()
</script>

<template>
  <Teleport to="body">
    <div class="fixed bottom-0 right-0 z-[100] p-4 md:bottom-4 md:right-4 md:max-w-[420px] pointer-events-none">
      <TransitionGroup
        name="toast"
        tag="div"
        class="flex flex-col gap-2"
      >
        <Toast
          v-for="toast in toasts"
          :key="toast.id"
          :title="toast.title"
          :description="toast.description"
          :variant="toast.variant"
          :duration="toast.duration"
          :closable="toast.closable"
          :action="toast.action"
          @close="removeToast(toast.id)"
        />
      </TransitionGroup>
    </div>
  </Teleport>
</template>

<style scoped>
.toast-move,
.toast-enter-active,
.toast-leave-active {
  transition: all 0.3s ease;
}

.toast-enter-from {
  transform: translateX(100%);
  opacity: 0;
}

.toast-leave-to {
  transform: translateX(100%);
  opacity: 0;
}

.toast-leave-active {
  position: absolute;
  right: 0;
}
</style>