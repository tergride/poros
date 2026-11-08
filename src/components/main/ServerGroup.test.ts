import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'

import { createAppI18n } from '@/i18n'
import type { Group, Server } from '@/platform/core-types'
import { GroupManual, GroupSubscription } from '@/platform/core-types'

import ServerGroup from './ServerGroup.vue'

const group: Group = {
  id: 'g1',
  kind: GroupSubscription,
  name: 'Main',
  url: 'https://sub.example.com/u/1',
  addedAt: 1,
  updatedAt: 0,
  servers: 2,
  skipped: 0,
  upload: 1_000_000_000,
  download: 2_000_000_000,
  total: 10_000_000_000,
  expire: 0,
  webPageUrl: '',
  supportUrl: '',
  announce: '',
  error: '',
  errorText: '',
}
const servers: Server[] = [
  { id: 's1', groupId: 'g1', name: 'Germany', protocol: 'vless', host: 'de.example.com', port: 443, country: 'DE', latency: 42 },
  { id: 's2', groupId: 'g1', name: 'Finland', protocol: 'trojan', host: 'fi.example.com', port: 443, country: 'FI', latency: 0 },
]

function mountGroup(props: Partial<{ group: Group }>) {
  return mount(ServerGroup, {
    props: { group, servers, selected: 's1', open: true, updating: false, ...props },
    global: { plugins: [createAppI18n(['ru-RU'])] },
  })
}

describe('a group of servers', () => {
  it('shows the quota and the update of a subscription', () => {
    const wrapper = mountGroup({})
    expect(wrapper.find('[data-testid="quota"]').exists()).toBe(true)
    expect(wrapper.find('button[aria-label="Обновить подписку"]').exists()).toBe(true)
    expect(wrapper.find('button[aria-label="Действия с подпиской"]').exists()).toBe(true)
  })

  it('shows the country of a server as a flag named in the hint', () => {
    const flags = mountGroup({}).findAll('[data-testid="flag"]')
    expect(flags.map((f) => f.text())).toEqual(['🇩🇪', '🇫🇮'])
    expect(flags[0]!.attributes('title')).toBe('DE — Германия')
  })

  it('tells a group of the user apart and leaves out what only a subscription has', () => {
    const wrapper = mountGroup({ group: { ...group, kind: GroupManual, name: 'Mine', url: '', upload: 0, download: 0, total: 0 } })
    expect(wrapper.text()).toContain('2 сервера · своя группа')
    expect(wrapper.find('[data-testid="quota"]').exists()).toBe(false)
    expect(wrapper.find('button[aria-label="Обновить подписку"]').exists()).toBe(false)
    expect(wrapper.find('button[aria-label="Действия с группой"]').exists()).toBe(true)
    expect(wrapper.findAll('[data-testid="server"]')).toHaveLength(2)
  })
})
