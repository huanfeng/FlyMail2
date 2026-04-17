<script setup lang="ts">
import AppMenu from './AppMenu.vue';
</script>

<template>
    <div class="layout-sidebar">
        <app-menu></app-menu>
    </div>
</template>

<style lang="scss" scoped>
.layout-sidebar {
    position: fixed;
    width: 300px;
    height: calc(100vh - 7rem);
    z-index: 999;
    overflow-y: auto;
    user-select: none;
    top: 5.5rem;
    left: 1.25rem;
    transition: transform 0.2s, left 0.2s;
    background-color: var(--p-surface-0);
    border-radius: 12px;
    padding: 1.25rem 1.5rem;
    box-shadow: 0px 3px 5px rgba(0, 0, 0, 0.02), 0px 0px 2px rgba(0, 0, 0, 0.05), 0px 1px 4px rgba(0, 0, 0, 0.08);
}

.app-dark .layout-sidebar {
    background-color: var(--p-surface-900);
}
</style>
