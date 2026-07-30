<template>
  <aside
    class="sidebar"
    :class="[
      sidebarCollapsed ? 'w-[72px]' : 'w-64',
      { '-translate-x-full lg:translate-x-0': !mobileOpen }
    ]"
  >
    <div class="sidebar-header" :class="{ 'sidebar-header-collapsed': sidebarCollapsed }">
      <router-link
        :to="homePath"
        class="sidebar-logo flex h-9 w-9 items-center justify-center overflow-hidden rounded-xl shadow-glow transition-opacity hover:opacity-80"
        @click="handleMenuItemClick"
      >
        <img v-if="settingsLoaded" :src="siteLogo || '/logo.svg'" alt="Logo" class="h-full w-full object-contain" />
      </router-link>
      <div
        class="sidebar-brand"
        :class="{ 'sidebar-brand-collapsed': sidebarCollapsed }"
        :aria-hidden="sidebarCollapsed ? 'true' : 'false'"
      >
        <router-link
          :to="homePath"
          class="sidebar-brand-title text-lg font-bold text-gray-900 transition-colors hover:text-primary-600 dark:text-white dark:hover:text-primary-400"
          @click="handleMenuItemClick"
        >
          {{ siteName }}
        </router-link>
        <VersionBadge :version="siteVersion" />
      </div>
    </div>

    <nav ref="sidebarNavRef" class="sidebar-nav scrollbar-hide">
      <div class="sidebar-section">
        <router-link
          v-for="item in adminNavItems"
          :id="item.path === '/admin/accounts' ? 'sidebar-channel-manage' : item.path === '/admin/groups' ? 'sidebar-group-manage' : undefined"
          :key="item.path"
          :to="item.path"
          class="sidebar-link mb-1"
          :class="{
            'sidebar-link-active': isActive(item.path),
            'sidebar-link-collapsed': sidebarCollapsed
          }"
          :title="sidebarCollapsed ? item.label : undefined"
          @click="handleMenuItemClick"
        >
          <Icon :name="item.icon" size="md" class="flex-shrink-0" />
          <span
            class="sidebar-label"
            :class="{ 'sidebar-label-collapsed': sidebarCollapsed }"
            :aria-hidden="sidebarCollapsed ? 'true' : 'false'"
          >
            {{ item.label }}
          </span>
        </router-link>
      </div>
    </nav>

    <div class="mt-auto border-t border-gray-100 p-3 dark:border-dark-800">
      <router-link
        to="/profile"
        class="sidebar-link mb-2 w-full"
        :class="{
          'sidebar-link-active': isActive('/profile'),
          'sidebar-link-collapsed': sidebarCollapsed
        }"
        :title="sidebarCollapsed ? t('nav.profile') : undefined"
        @click="handleMenuItemClick"
      >
        <Icon name="user" size="md" class="flex-shrink-0" />
        <span
          class="sidebar-label"
          :class="{ 'sidebar-label-collapsed': sidebarCollapsed }"
          :aria-hidden="sidebarCollapsed ? 'true' : 'false'"
        >
          {{ t('nav.profile') }}
        </span>
      </router-link>

      <button
        class="sidebar-link mb-2 w-full"
        :class="{ 'sidebar-link-collapsed': sidebarCollapsed }"
        :title="sidebarCollapsed ? (isDark ? t('nav.lightMode') : t('nav.darkMode')) : undefined"
        @click="toggleTheme"
      >
        <Icon :name="isDark ? 'sun' : 'moon'" size="md" :class="isDark ? 'text-amber-500' : ''" />
        <span
          class="sidebar-label"
          :class="{ 'sidebar-label-collapsed': sidebarCollapsed }"
          :aria-hidden="sidebarCollapsed ? 'true' : 'false'"
        >
          {{ isDark ? t('nav.lightMode') : t('nav.darkMode') }}
        </span>
      </button>

      <button
        class="sidebar-link w-full"
        :class="{ 'sidebar-link-collapsed': sidebarCollapsed }"
        :title="sidebarCollapsed ? t('nav.expand') : t('nav.collapse')"
        @click="toggleSidebar"
      >
        <Icon :name="sidebarCollapsed ? 'chevronRight' : 'chevronLeft'" size="md" />
        <span
          class="sidebar-label"
          :class="{ 'sidebar-label-collapsed': sidebarCollapsed }"
          :aria-hidden="sidebarCollapsed ? 'true' : 'false'"
        >
          {{ t('nav.collapse') }}
        </span>
      </button>
    </div>
  </aside>

  <transition name="fade">
    <div v-if="mobileOpen" class="fixed inset-0 z-30 bg-black/50 lg:hidden" @click="closeMobile"></div>
  </transition>
</template>

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { useAdminSettingsStore, useAppStore } from '@/stores'
import VersionBadge from '@/components/common/VersionBadge.vue'
import Icon from '@/components/icons/Icon.vue'
import { sanitizeUrl } from '@/utils/url'
import { FeatureFlags, makeSidebarFlag } from '@/utils/featureFlags'

type IconName = 'grid' | 'chart' | 'database' | 'globe' | 'server' | 'sync' | 'shield' | 'clipboard' | 'cog'

interface NavItem {
  path: string
  label: string
  icon: IconName
  featureFlag?: () => boolean | undefined
}

const { t } = useI18n()
const route = useRoute()
const appStore = useAppStore()
const adminSettingsStore = useAdminSettingsStore()

const homePath = '/admin/accounts'
const sidebarCollapsed = computed(() => appStore.sidebarCollapsed)
const mobileOpen = computed(() => appStore.mobileOpen)
const sidebarNavRef = ref<HTMLElement | null>(null)
const isDark = ref(document.documentElement.classList.contains('dark'))
const siteName = computed(() => appStore.siteName)
const siteLogo = computed(() => sanitizeUrl(appStore.siteLogo || '', { allowRelative: true, allowDataUrl: true }))
const siteVersion = computed(() => appStore.siteVersion)
const settingsLoaded = computed(() => appStore.publicSettingsLoaded)

const flagChannelMonitor = makeSidebarFlag(FeatureFlags.channelMonitor)
const flagOpsMonitoring = () => adminSettingsStore.opsMonitoringEnabled

const adminNavItems = computed<NavItem[]>(() => {
  const items: NavItem[] = [
    { path: '/admin/dashboard', label: t('nav.dashboard'), icon: 'grid' },
    { path: '/admin/groups', label: t('nav.groups'), icon: 'database' },
    { path: '/admin/accounts', label: t('nav.accounts'), icon: 'globe' },
    { path: '/admin/channels/pricing', label: t('nav.channelPricing'), icon: 'chart' },
    { path: '/admin/channels/monitor', label: t('nav.channelMonitor'), icon: 'sync', featureFlag: flagChannelMonitor },
    { path: '/admin/proxies', label: t('nav.proxies'), icon: 'server' },
    { path: '/admin/ops', label: t('nav.ops'), icon: 'chart', featureFlag: flagOpsMonitoring },
    { path: '/admin/usage', label: t('nav.usage'), icon: 'clipboard' },
    { path: '/admin/prompt-audit', label: t('nav.promptAudit'), icon: 'shield' },
    { path: '/admin/audit-logs', label: t('nav.auditLogs'), icon: 'shield' },
    { path: '/admin/settings', label: t('nav.settings'), icon: 'cog' }
  ]

  return items.filter((item) => item.featureFlag?.() !== false)
})

function isActive(path: string): boolean {
  return route.path === path
}

function handleMenuItemClick() {
  if (mobileOpen.value) {
    appStore.setMobileOpen(false)
  }
}

function toggleSidebar() {
  appStore.toggleSidebar()
}

function toggleTheme() {
  isDark.value = !isDark.value
  document.documentElement.classList.toggle('dark', isDark.value)
  localStorage.setItem('theme', isDark.value ? 'dark' : 'light')
}

function closeMobile() {
  appStore.setMobileOpen(false)
}

const savedTheme = localStorage.getItem('theme')
if (savedTheme === 'dark' || (!savedTheme && window.matchMedia('(prefers-color-scheme: dark)').matches)) {
  isDark.value = true
  document.documentElement.classList.add('dark')
}

watch(
  () => appStore.publicSettingsLoaded,
  (loaded) => {
    if (loaded) void adminSettingsStore.fetch()
  },
  { immediate: true }
)

onMounted(() => {
  void adminSettingsStore.fetch()
  if (appStore.sidebarScrollTop > 0 && sidebarNavRef.value) {
    void nextTick(() => {
      if (sidebarNavRef.value) {
        sidebarNavRef.value.scrollTop = appStore.sidebarScrollTop
      }
    })
  }
})

onBeforeUnmount(() => {
  if (sidebarNavRef.value) {
    appStore.sidebarScrollTop = sidebarNavRef.value.scrollTop
  }
})
</script>

<style scoped>
.sidebar-logo {
  flex: 0 0 2.25rem;
  min-width: 2.25rem;
}

.sidebar-header-collapsed {
  gap: 0;
  padding-left: 1.125rem;
  padding-right: 1.125rem;
}

.sidebar-brand {
  min-width: 0;
  flex: 1 1 auto;
  max-width: 12rem;
  white-space: nowrap;
  transition: max-width 0.22s ease, opacity 0.14s ease, transform 0.14s ease;
}

.sidebar-brand-collapsed {
  max-width: 0;
  overflow: hidden;
  opacity: 0;
  transform: translateX(-4px);
  pointer-events: none;
}

.sidebar-brand-title,
.sidebar-label {
  display: block;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.sidebar-link-collapsed {
  gap: 0;
  padding-left: 0.875rem;
  padding-right: 0.875rem;
}

.sidebar-label {
  max-width: 12rem;
  transition: max-width 0.2s ease, opacity 0.12s ease, transform 0.12s ease;
}

.sidebar-label-collapsed {
  max-width: 0;
  opacity: 0;
  transform: translateX(-4px);
  pointer-events: none;
}
</style>
