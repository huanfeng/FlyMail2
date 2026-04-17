<script setup lang="ts">
import { RouterView } from 'vue-router';
import AppLayout from './layout/AppLayout.vue';
</script>

<template>
    <RouterView v-slot="{ Component, route }">
        <component v-if="route.meta.plain" :is="Component" />
        <AppLayout v-else>
            <component :is="Component" />
        </AppLayout>
    </RouterView>
</template>

<style>
body {
    margin: 0;
    padding: 0;
    font-family: var(--font-family);
    overflow: hidden;
}

html, #app {
    height: 100%;
}
</style>
