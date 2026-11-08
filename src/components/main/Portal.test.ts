import { mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { createAppI18n } from '@/i18n'
import type { Settings } from '@/platform/core-types'
import { ConnectionIdle, ConnectionRunning, ConnectionStarting, KillSwitchAlways, KillSwitchConnected, KillSwitchOff } from '@/platform/core-types'
import { useCoreStore } from '@/stores/core'

import Portal from './Portal.vue'

const settings = { tun: false, systemProxy: true, server: 's1', preset: 'all' } as Settings
const server = { id: 's1', groupId: '', name: 'Germany', protocol: 'vless', host: '', port: 443, country: 'DE', security: '', upload: 0, download: 0, speedDown: '', speedUp: '', exitIp: '', exitCountry: '', latency: 0 }
const idle = { state: ConnectionIdle, serverId: '', tun: false, systemProxy: false, since: 0, error: '', errorText: '' }
const blocked = { on: true, full: false, error: '', errorText: '' }

function mountPortal() {
  return mount(Portal, { global: { plugins: [createAppI18n(['ru-RU'])], stubs: { RouterLink: true } } })
}

describe('Portal and the kill switch', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    const core = useCoreStore()
    core.servers = [server]
  })

  it('keeps Stop after a lost connection while the kill switch holds', async () => {
    const core = useCoreStore()
    core.settings = { ...settings, killSwitch: KillSwitchConnected }
    core.connection = { ...idle }
    core.killSwitch = { ...blocked }
    const wrapper = mountPortal()
    expect(wrapper.get('[data-testid="portal-state"]').text()).toBe('Соединение прервано — трафик VPN закрыт')
    expect(wrapper.find('[data-testid="start"]').exists()).toBe(false)
    expect(wrapper.get('[data-testid="stop"]').find('[data-testid="kill-switch-lock"]').exists()).toBe(true)

    // The core came back without rights and its TUN is down: Windows blocks everything.
    core.killSwitch = { ...blocked, full: true }
    await wrapper.vm.$nextTick()
    expect(wrapper.get('[data-testid="portal-state"]').text()).toBe('Соединение прервано — интернет закрыт')

    core.killSwitch = { on: false, full: false, error: '', errorText: '' }
    await wrapper.vm.$nextTick()
    expect(wrapper.get('[data-testid="portal-state"]').text()).toBe('Не подключено')
    expect(wrapper.find('[data-testid="kill-switch-lock"]').exists()).toBe(false)
  })

  it('shows the lock on Start and on Stop while the block is on', async () => {
    const core = useCoreStore()
    core.settings = { ...settings, killSwitch: KillSwitchAlways }
    core.connection = { ...idle }
    core.killSwitch = { ...blocked }
    const wrapper = mountPortal()
    expect(wrapper.get('[data-testid="start"]').find('[data-testid="kill-switch-lock"]').exists()).toBe(true)
    // Without a connection VPN traffic is closed, and the window says so.
    expect(wrapper.get('[data-testid="portal-state"]').text()).toBe('Трафик VPN закрыт до подключения')

    core.connection = { ...idle, state: ConnectionRunning, since: Date.now() }
    await wrapper.vm.$nextTick()
    expect(wrapper.get('[data-testid="stop"]').find('[data-testid="kill-switch-lock"]').exists()).toBe(true)
  })
})

describe('Portal while it connects', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    setActivePinia(createPinia())
    const core = useCoreStore()
    core.servers = [server]
    core.settings = { ...settings, killSwitch: KillSwitchOff }
    core.status = { state: 'ready', elevated: false }
  })
  afterEach(() => vi.useRealTimers())

  it('offers Cancel once a start takes longer than 3 seconds', async () => {
    const core = useCoreStore()
    core.connection = { ...idle, state: ConnectionStarting }
    const wrapper = mountPortal()
    expect(wrapper.get('[data-testid="start"]').attributes('disabled')).toBeDefined()
    expect(wrapper.find('[data-testid="cancel"]').exists()).toBe(false)

    await vi.advanceTimersByTimeAsync(3100)
    expect(wrapper.find('[data-testid="start"]').exists()).toBe(false)
    expect(wrapper.get('[data-testid="cancel"]').text()).toBe('Отменить')

    core.connection = { ...idle, state: ConnectionRunning, since: Date.now() }
    await wrapper.vm.$nextTick()
    expect(wrapper.find('[data-testid="cancel"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="stop"]').exists()).toBe(true)
  })

  it('shows the exit country as a flag named in its hint', async () => {
    const core = useCoreStore()
    core.connection = { ...idle, state: ConnectionRunning, since: Date.now() }
    core.network = {
      ...core.network,
      proxy: { ip: '198.51.100.24', ipv6: '2001:db8::24', countryCode: 'DE', city: '', isp: '', asn: 0 },
    }
    const wrapper = mountPortal()
    // IPv4 first: it is the shorter one.
    expect(wrapper.get('[data-testid="portal-ip"]').text()).toBe('IP 198.51.100.24')
    const flags = wrapper.findAll('[data-testid="flag"]')
    expect(flags.length).toBe(2)
    expect(flags[0]!.text()).toBe('🇩🇪')
    expect(flags[0]!.attributes('title')).toBe('DE — Германия')
  })
})
