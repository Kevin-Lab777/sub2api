import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'

import { useOpsCapabilitiesStore } from '../opsCapabilities'

const getCapabilities = vi.hoisted(() => vi.fn())

vi.mock('@/api/admin/ops', () => ({
  opsAPI: { getCapabilities },
}))

describe('Ops capabilities store', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    getCapabilities.mockReset()
  })

  it('shares one in-flight capability request', async () => {
    getCapabilities.mockResolvedValue({
      monitoring_enabled: true,
      realtime_monitoring_enabled: false,
      query_mode_default: 'raw',
    })
    const store = useOpsCapabilitiesStore()

    const [first, second] = await Promise.all([store.fetch(), store.fetch()])

    expect(getCapabilities).toHaveBeenCalledOnce()
    expect(first).toEqual(second)
    expect(store.monitoringEnabled).toBe(true)
    expect(store.realtimeMonitoringEnabled).toBe(false)
    expect(store.queryModeDefault).toBe('raw')
  })

  it('propagates failures without publishing synthetic capabilities', async () => {
    const error = new Error('capabilities unavailable')
    getCapabilities.mockRejectedValue(error)
    const store = useOpsCapabilitiesStore()

    await expect(store.fetch()).rejects.toBe(error)
    expect(store.capabilities).toBeNull()
  })
})
