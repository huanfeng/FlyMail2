<script setup lang="ts">
import { onMounted } from 'vue'
import { initializeTheme } from '@/lib/theme'
import { ToastContainer } from '@/components/ui/toast'

onMounted(() => {
  // 初始化主题系统
  initializeTheme()
})
</script>

<template>
  <RouterView />
  <ToastContainer />
</template>
