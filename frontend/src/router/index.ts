import { createRouter, createWebHistory, type RouteRecordRaw } from 'vue-router'
import { useAuthStore } from '@/stores/auth'
import { useAppStore } from '@/stores/app'
import { useAdminSettingsStore } from '@/stores/adminSettings'
import { useAdminComplianceStore } from '@/stores/adminCompliance'
import { useNavigationLoadingState } from '@/composables/useNavigationLoading'
import { useRoutePrefetch } from '@/composables/useRoutePrefetch'
import { getSetupStatus } from '@/api/setup'
import { resolveCompletedSetupRedirectPath } from './setupRedirect'
import { resolveNextAPINavigation } from './nextApiGuard'
import { resolveRouteDocumentTitle } from './title'

export const NEXT_API_HOME = '/admin/accounts'

const adminMeta = (title: string, titleKey: string, descriptionKey?: string) => ({
  requiresAuth: true,
  requiresAdmin: true,
  title,
  titleKey,
  ...(descriptionKey ? { descriptionKey } : {})
})

export const routes: RouteRecordRaw[] = [
  {
    path: '/setup',
    name: 'Setup',
    component: () => import('@/views/setup/SetupWizardView.vue'),
    meta: { requiresAuth: false, title: 'Setup' }
  },
  {
    path: '/login',
    name: 'Login',
    component: () => import('@/views/auth/LoginView.vue'),
    meta: { requiresAuth: false, title: 'Login', titleKey: 'home.login' }
  },
  {
    path: '/legal/:documentId',
    name: 'LegalDocument',
    component: () => import('@/views/public/LegalDocumentView.vue'),
    meta: { requiresAuth: false, title: 'Legal Document' }
  },
  { path: '/', redirect: NEXT_API_HOME },
  { path: '/admin', redirect: NEXT_API_HOME },
  {
    path: '/profile',
    name: 'Profile',
    component: () => import('@/views/user/ProfileView.vue'),
    meta: adminMeta('Profile', 'profile.title', 'profile.description')
  },
  {
    path: '/admin/dashboard',
    name: 'AdminDashboard',
    component: () => import('@/views/admin/DashboardView.vue'),
    meta: adminMeta('Admin Dashboard', 'admin.dashboard.title', 'admin.dashboard.description')
  },
  {
    path: '/admin/groups',
    name: 'AdminGroups',
    component: () => import('@/views/admin/GroupsView.vue'),
    meta: adminMeta('Pool Management', 'admin.groups.title', 'admin.groups.description')
  },
  {
    path: '/admin/accounts',
    name: 'AdminAccounts',
    component: () => import('@/views/admin/AccountsView.vue'),
    meta: adminMeta('Account Management', 'admin.accounts.title', 'admin.accounts.description')
  },
  { path: '/admin/channels', redirect: '/admin/channels/pricing' },
  {
    path: '/admin/channels/pricing',
    name: 'AdminChannels',
    component: () => import('@/views/admin/ChannelsView.vue'),
    meta: adminMeta('Channel Management', 'admin.channels.title', 'admin.channels.description')
  },
  {
    path: '/admin/channels/monitor',
    name: 'AdminChannelMonitor',
    component: () => import('@/views/admin/ChannelMonitorView.vue'),
    meta: adminMeta('Channel Monitor', 'admin.channelMonitor.title', 'admin.channelMonitor.description')
  },
  {
    path: '/admin/proxies',
    name: 'AdminProxies',
    component: () => import('@/views/admin/ProxiesView.vue'),
    meta: adminMeta('Proxy Management', 'admin.proxies.title', 'admin.proxies.description')
  },
  {
    path: '/admin/ops',
    name: 'AdminOps',
    component: () => import('@/views/admin/ops/OpsDashboard.vue'),
    meta: adminMeta('Ops Monitoring', 'admin.ops.title', 'admin.ops.description')
  },
  {
    path: '/admin/usage',
    name: 'AdminUsage',
    component: () => import('@/views/admin/UsageView.vue'),
    meta: adminMeta('Usage Records', 'admin.usage.title', 'admin.usage.description')
  },
  {
    path: '/admin/prompt-audit',
    name: 'AdminPromptAudit',
    component: () => import('@/features/prompt-audit/PromptAuditView.vue'),
    meta: adminMeta('Prompt Audit', 'admin.promptAudit.title', 'admin.promptAudit.description')
  },
  {
    path: '/admin/audit-logs',
    name: 'AdminAuditLogs',
    component: () => import('@/views/admin/AuditLogView.vue'),
    meta: adminMeta('Audit Logs', 'admin.audit.title', 'admin.audit.description')
  },
  {
    path: '/admin/settings',
    name: 'AdminSettings',
    component: () => import('@/views/admin/SettingsView.vue'),
    meta: adminMeta('System Settings', 'admin.settings.title', 'admin.settings.description')
  },
  {
    path: '/:pathMatch(.*)*',
    name: 'NotFound',
    component: () => import('@/views/NotFoundView.vue'),
    meta: { requiresAuth: false, title: '404 Not Found' }
  }
]

const router = createRouter({
  history: createWebHistory(import.meta.env.BASE_URL),
  routes,
  scrollBehavior(_to, _from, savedPosition) {
    return savedPosition || { top: 0 }
  }
})

let authInitialized = false
const navigationLoading = useNavigationLoadingState()
let routePrefetch: ReturnType<typeof useRoutePrefetch> | null = null

router.beforeEach(async (to, _from, next) => {
  navigationLoading.startNavigation()

  const authStore = useAuthStore()
  if (!authInitialized) {
    authStore.checkAuth()
    authInitialized = true
  }

  const appStore = useAppStore()
  const adminSettingsStore = useAdminSettingsStore()
  document.title = resolveRouteDocumentTitle(to, appStore.siteName, adminSettingsStore.customMenuItems)

  if (to.path === '/setup') {
    try {
      const status = await getSetupStatus()
      if (!status.needs_setup) {
        next(resolveCompletedSetupRedirectPath(authStore.isAuthenticated, authStore.isAdmin))
        return
      }
    } catch {
      // Keep setup reachable when bootstrap status cannot be read.
    }
  }

  const redirect = resolveNextAPINavigation(to.path, to.meta, {
    isAuthenticated: authStore.isAuthenticated,
    isAdmin: authStore.isAdmin
  })
  if (redirect) {
    next(redirect)
    return
  }

  if (to.meta.requiresAdmin === true) {
    const compliance = useAdminComplianceStore()
    if (!compliance.initialized) {
      try {
        await compliance.fetchStatus()
      } catch (error) {
        const err = error as { status?: number; code?: string; metadata?: Record<string, string> }
        if (err.status === 423 && err.code === 'ADMIN_COMPLIANCE_ACK_REQUIRED') {
          compliance.requireAcknowledgement(err.metadata)
        }
      }
    }
  }

  next()
})

router.afterEach((to) => {
  navigationLoading.endNavigation()
  if (!routePrefetch) {
    routePrefetch = useRoutePrefetch(router)
  }
  routePrefetch.triggerPrefetch(to)
})

router.onError((error) => {
  console.error('Router error:', error)
  const isChunkLoadError =
    error.message?.includes('Failed to fetch dynamically imported module') ||
    error.message?.includes('Loading chunk') ||
    error.message?.includes('Loading CSS chunk') ||
    error.name === 'ChunkLoadError'

  if (!isChunkLoadError) return

  const reloadKey = 'chunk_reload_attempted'
  const lastReload = sessionStorage.getItem(reloadKey)
  const now = Date.now()
  if (!lastReload || now - parseInt(lastReload) > 10000) {
    sessionStorage.setItem(reloadKey, now.toString())
    window.location.reload()
  }
})

export default router
