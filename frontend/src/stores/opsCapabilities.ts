import { computed, ref } from 'vue'
import { defineStore } from 'pinia'
import { opsAPI, type OpsCapabilities } from '@/api/admin/ops'

export const useOpsCapabilitiesStore = defineStore('opsCapabilities', () => {
  const capabilities = ref<OpsCapabilities | null>(null)
  const request = ref<Promise<OpsCapabilities> | null>(null)

  const monitoringEnabled = computed(() => capabilities.value?.monitoring_enabled)
  const realtimeMonitoringEnabled = computed(() => capabilities.value?.realtime_monitoring_enabled)
  const queryModeDefault = computed(() => capabilities.value?.query_mode_default)

  function fetch(force = false): Promise<OpsCapabilities> {
    if (!force && capabilities.value) return Promise.resolve(capabilities.value)
    if (request.value) return request.value

    const pending = opsAPI.getCapabilities()
      .then((value) => {
        capabilities.value = value
        return value
      })
      .finally(() => {
        request.value = null
      })
    request.value = pending
    return pending
  }

  return {
    capabilities,
    monitoringEnabled,
    realtimeMonitoringEnabled,
    queryModeDefault,
    fetch,
  }
})
