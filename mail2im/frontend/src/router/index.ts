import { createRouter, createWebHistory } from 'vue-router';
import Logs from '../views/Logs.vue';
import Settings from '../views/Settings.vue';
import Dashboard from '../views/Dashboard.vue';
import { useAuthStore } from '../stores/auth';
import { pinia } from '../stores';
import i18n from '../i18n';

const appName = 'Mail2IM';
const titleMap: Record<string, string> = {
  dashboard: 'dashboard.title',
  logs: 'logs.title',
  settings: 'settings.title',
  debug: 'debug.title',
  proxies: 'menu.proxies',
  accounts: 'menu.accounts',
  emails: 'emails_view.list',
  channels: 'channels.title',
  'email-detail': 'common.email_detail',
  login: 'auth.title'
};

const router = createRouter({
  history: createWebHistory(),
  routes: [
    {
      path: '/login',
      name: 'login',
      component: () => import('../views/Login.vue'),
      meta: { plain: true, public: true, titleKey: 'auth.title' }
    },
    {
      path: '/',
      name: 'dashboard',
      component: Dashboard,
      meta: { titleKey: 'dashboard.title' }
    },
    {
      path: '/logs',
      name: 'logs',
      component: Logs,
      meta: { titleKey: 'logs.title' }
    },
    {
      path: '/settings',
      name: 'settings',
      component: Settings,
      meta: { titleKey: 'settings.title' }
    },
    {
      path: '/dev',
      name: 'debug',
      component: () => import('../views/Debug.vue'),
      meta: { titleKey: 'debug.title' }
    },
    {
      path: '/accounts',
      name: 'accounts',
      component: () => import('../views/Accounts.vue'),
      meta: { titleKey: 'menu.accounts' }
    },
    {
      path: '/emails',
      name: 'emails',
      component: () => import('../views/Emails.vue'),
      meta: { titleKey: 'emails_view.list' }
    },
    {
      path: '/emails/:id',
      name: 'email-detail',
      component: () => import('../views/EmailDetail.vue'),
      meta: { titleKey: 'common.email_detail' }
    },
    {
      path: '/email-view/:id',
      name: 'email-standalone',
      component: () => import('../views/EmailStandalone.vue'),
      meta: { plain: true, titleKey: 'common.email_detail' }
    },
    {
      path: '/share/:token',
      name: 'share-view',
      component: () => import('../views/EmailStandalone.vue'),
      meta: { plain: true, public: true, titleKey: 'common.email_detail' }
    },
    {
      path: '/proxies',
      name: 'proxies',
      component: () => import('../views/Proxies.vue'),
      meta: { titleKey: 'menu.proxies' }
    },
    {
      path: '/channels',
      name: 'channels',
      component: () => import('../views/Channels.vue'),
      meta: { titleKey: 'menu.channels' }
    },
    {
      path: '/notification-policy',
      name: 'notification-policy',
      component: () => import('../views/NotificationPolicy.vue'),
      meta: { titleKey: 'menu.notification_policy' }
    },
    {
      path: '/classification',
      name: 'classification',
      component: () => import('../views/Classification.vue'),
      meta: { titleKey: 'menu.classification' }
    },
    {
      path: '/templates',
      name: 'templates',
      component: () => import('../views/Templates.vue'),
      meta: { titleKey: 'menu.templates' }
    }
  ]
});

router.beforeEach(async (to) => {
  const auth = useAuthStore(pinia);
  await auth.ensureSession();

  const isPublic = (to.meta as any)?.public;
  if (isPublic) return true;

  if (!auth.isAuthenticated) {
    return { name: 'login', query: { redirect: to.fullPath } };
  }

  if (to.name === 'login' && auth.isAuthenticated) {
    return { name: 'dashboard' };
  }

  return true;
});

router.afterEach((to) => {
  const t = i18n.global.t;
  const metaTitle = (to.meta as any)?.titleKey || (to.name ? titleMap[to.name as string] : '');
  const titleText = metaTitle ? t(metaTitle as string) : (to.name ? String(to.name) : '');
  document.title = titleText ? `${titleText} - ${appName}` : appName;
});

export default router;
