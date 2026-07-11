import { nextTick } from 'vue'
import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import GroupSelector from '../GroupSelector.vue'
import type { AdminGroup, GroupPlatform } from '@/types'

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string, params?: Record<string, unknown>) =>
        params?.count != null ? `${key}:${String(params.count)}` : key
    })
  }
})

const buildGroup = (id: number, platform: GroupPlatform, name = `${platform}-${id}`): AdminGroup => ({
  id,
  name,
  description: null,
  platform,
  rate_multiplier: 1,
  is_exclusive: false,
  status: 'active',
  subscription_type: 'standard',
  daily_limit_usd: null,
  weekly_limit_usd: null,
  monthly_limit_usd: null,
  allow_image_generation: false,
  allow_batch_image_generation: false,
  image_rate_independent: false,
  image_rate_multiplier: 1,
  batch_image_discount_multiplier: 1,
  batch_image_hold_multiplier: 1,
  image_price_1k: null,
  image_price_2k: null,
  image_price_4k: null,
  peak_rate_enabled: false,
  peak_start: '',
  peak_end: '',
  peak_rate_multiplier: 1,
  claude_code_only: false,
  fallback_group_id: null,
  fallback_group_id_on_invalid_request: null,
  require_oauth_only: false,
  require_privacy_set: false,
  created_at: '',
  updated_at: '',
  model_routing: null,
  model_routing_enabled: false,
  mcp_xml_inject: false,
  sort_order: id
})

const mountSelector = (props: {
  modelValue: number[]
  groups: AdminGroup[]
  platform?: GroupPlatform
  searchable?: boolean | 'auto'
}) =>
  mount(GroupSelector, {
    props,
    global: {
      stubs: {
        GroupBadge: {
          props: ['name'],
          template: '<span>{{ name }}</span>'
        },
        Icon: true
      }
    }
  })

describe('GroupSelector', () => {
  it('prunes selected groups that are hidden by platform filtering', async () => {
    const wrapper = mountSelector({
      modelValue: [1, 2],
      platform: 'openai',
      groups: [
        buildGroup(1, 'anthropic'),
        buildGroup(2, 'openai')
      ]
    })

    await nextTick()

    expect(wrapper.emitted('update:modelValue')?.[0]).toEqual([[2]])
  })

  it('does not prune selected groups when search filtering hides them', async () => {
    const wrapper = mountSelector({
      modelValue: [1, 2],
      platform: 'openai',
      searchable: true,
      groups: [
        buildGroup(1, 'openai', 'alpha'),
        buildGroup(2, 'openai', 'beta')
      ]
    })

    await nextTick()
    await wrapper.get('input').setValue('missing')
    await nextTick()

    expect(wrapper.emitted('update:modelValue')).toBeUndefined()
  })
})
