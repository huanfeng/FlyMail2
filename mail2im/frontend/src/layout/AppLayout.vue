<script setup lang="ts">
import { computed } from 'vue';
import Toast from 'primevue/toast';
import AppTopbar from './AppTopbar.vue';
import AppMenu from './AppMenu.vue';
import SideControls from './SideControls.vue';
import { useLayout } from './composables/layout';

const { layoutConfig, layoutState, onMenuToggle } = useLayout();

const containerClass = computed(() => ({
    'layout-theme-light': !layoutConfig.darkTheme,
    'layout-theme-dark': layoutConfig.darkTheme,
    'layout-mobile-active': layoutState.staticMenuMobileActive
}));
</script>

<template>
    <div class="layout-wrapper" :class="containerClass">
        <Toast position="top-center" />
        <app-topbar />

        <div class="layout-grid">
            <aside
                class="side-rail"
                :class="{ 'mobile-open': layoutState.staticMenuMobileActive }"
            >
                <SideControls class="side-card sticky-card" />
                <div class="side-card menu-card">
                    <app-menu />
                </div>
            </aside>

            <main class="content-rail">
                <div class="content-shell">
                    <slot />
                </div>
            </main>
        </div>

        <div class="layout-mask" v-if="layoutState.staticMenuMobileActive" @click="onMenuToggle"></div>
    </div>
</template>

<style lang="scss" scoped>
.layout-wrapper {
    min-height: 100vh;
    background-color: var(--p-surface-50);
}

@media (min-width: 992px) {
    .layout-wrapper {
        overflow: visible;
    }
}

.app-dark .layout-wrapper {
    background-color: var(--p-surface-950);
}

.layout-grid {
    display: grid;
    grid-template-columns: 320px 1fr;
    gap: 0.75rem;
    min-height: calc(100vh - 1rem);
    padding: 1rem 1rem 1rem 1rem;
    box-sizing: border-box;
}

.side-rail {
    display: flex;
    flex-direction: column;
    gap: 1rem;
    height: auto;
    position: relative;
    transition: transform 0.2s ease;
    z-index: 999;
    padding: 0.2rem;
    max-height: calc(100vh - 1rem);
    overflow-y: auto;
}

.side-card {
    background: var(--p-surface-0);
    border-radius: 16px;
    padding: 0.9rem 1rem;
    border: 1px solid var(--p-surface-200);
    box-shadow: 0px 6px 18px rgba(15, 23, 42, 0.05);
}

.app-dark .side-card {
    background: var(--p-surface-900);
    border-color: var(--p-surface-700);
}

.sticky-card {
    position: sticky;
    top: 0;
    z-index: 2;
}

.menu-card {
    flex: 1;
    overflow: auto;
}

.content-rail {
    min-height: calc(100vh - 1rem);
    max-height: calc(100vh - 1rem);
    display: flex;
    flex-direction: column;
    background: transparent;
    min-width: 0; // 重要: 如果注释, 右栏宽度会超出
}

.content-shell {
    flex: 1;
    min-height: 0;
    display: flex;
    flex-direction: column;
    padding: 0.2rem; // 右栏的总padding
    box-sizing: border-box;
    overflow: auto; // 关键: 如果注释, 则右栏没有滚动条
}

@media (max-width: 900px) {
    .layout-grid {
        grid-template-columns: 1fr;
        padding: 5rem 1rem 1rem 1rem;
    }

    .side-rail {
        position: fixed;
        top: 0;
        left: 0;
        width: 80%;
        max-width: 320px;
        max-height: calc(100vh - 0rem);
        height: 100vh;
        padding: 0.2rem;
        box-sizing: border-box;
        background: var(--p-surface-50);
        transform: translateX(-100%);
        box-shadow: 0 12px 40px rgba(15, 23, 42, 0.2);
    }

    .app-dark .side-rail {
        background: var(--p-surface-900);
    }

    .side-rail.mobile-open {
        transform: translateX(0);
    }

    .content-shell {
        height: calc(100vh - 5rem);
        max-height: calc(100vh - 5rem);
    }

    .layout-mask {
        display: block;
        position: fixed;
        top: 0;
        left: 0;
        z-index: 998;
        width: 100%;
        height: 100%;
        background-color: rgba(0, 0, 0, 0.45);
    }
}
</style>
