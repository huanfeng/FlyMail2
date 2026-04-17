<script setup lang="ts">
import { computed } from 'vue';
import AppMenuItem from './AppMenuItem.vue';
import { useI18n } from 'vue-i18n';

const { t } = useI18n();

interface MenuItem {
    label?: string;
    icon?: string;
    to?: string;
    items?: MenuItem[];
    separator?: boolean;
}

const model = computed<MenuItem[]>(() => [
    {
        label: t('menu.workspace'),
        items: [
            { label: t('menu.dashboard'), icon: 'pi pi-fw pi-home', to: '/' },
            { label: t('menu.logs'), icon: 'pi pi-fw pi-chart-bar', to: '/logs' }
        ]
    },
    {
        label: t('menu.inbox'),
        items: [
            { label: t('menu.emails'), icon: 'pi pi-fw pi-envelope', to: '/emails' }
        ]
    },
    {
        label: t('menu.configuration'),
        items: [
            { label: t('menu.accounts'), icon: 'pi pi-fw pi-users', to: '/accounts' },
            { label: t('menu.channels'), icon: 'pi pi-fw pi-bell', to: '/channels' },
            { label: t('menu.notification_policy'), icon: 'pi pi-fw pi-sliders-h', to: '/notification-policy' },
            { label: t('menu.templates'), icon: 'pi pi-fw pi-file', to: '/templates' },
            { label: t('menu.proxies'), icon: 'pi pi-fw pi-globe', to: '/proxies' },
            { label: t('menu.settings'), icon: 'pi pi-fw pi-cog', to: '/settings' }
        ]
    },
    {
        label: t('menu.tools'),
        items: [
            { label: t('menu.classification'), icon: 'pi pi-fw pi-tags', to: '/classification' },
            { label: t('menu.debug'), icon: 'pi pi-fw pi-code', to: '/dev' }
        ]
    }
]);
</script>

<template>
    <ul class="layout-menu">
        <template v-for="(item, i) in model" :key="item.label || i">
            <app-menu-item v-if="!item.separator" :item="item" :index="i"></app-menu-item>
            <li v-if="item.separator" class="menu-separator"></li>
        </template>
    </ul>
</template>

<style lang="scss" scoped>
.layout-menu {
    margin: 0;
    padding: 0;
    list-style-type: none;
}
</style>
