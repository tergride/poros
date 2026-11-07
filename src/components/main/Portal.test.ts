import { mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it } from 'vitest'

import { createAppI18n } from '@/i18n'
import type { Settings } from '@/platform/core-types'
import { ConnectionIdle, ConnectionRunning, KillSwitchAlways, KillSwitchConnected } from '@/platform/core-types'
import { useCoreStore } from '@/stores/core'

import Portal from './Portal.vue'

const settings = { tun: false, systemProxy: true, server: 's1', preset: 'all' } as Settings
const server = { id: 's1', subscriptionId: '', name: 'Germany', protocol: 'vless', host: '', port: 443, country: 'DE', latency: 0 }
const idle = { state: ConnectionIdle, serverId: '', tun: false, systemProxy: false, since: 0, error: '', errorText: '' }
const blocked = { on: true, error: '', errorText: '' }

function mountPortal() {
  return mount(Portal, { global: { plugins: [createAppI18n(['ru-RU'])], stubs: { RouterLink: true } } })
}

describe('Portal and the kill switch', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    const core = useCoreStore()
    core.servers = [server]
  })

  it('keeps Stop after a lost connection while the block holds', async () => {
    const core = useCoreStore()
    core.settings = { ...settings, killSwitch: KillSwitchConnected }
    core.connection = { ...idle }
    core.killSwitch = { ...blocked }
    const wrapper = mountPortal()
    expect(wrapper.get('[data-testid="portal-state"]').text()).toBe('Интернет заблокирован до «Стопа»')
    expect(wrapper.find('[data-testid="start"]').exists()).toBe(false)
    expect(wrapper.get('[data-testid="stop"]').find('[data-testid="kill-switch-lock"]').exists()).toBe(true)

    core.killSwitch = { on: false, error: '', errorText: '' }
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
    expect(wrapper.get('[data-testid="portal-state"]').text()).toBe('Не подключено')

    core.connection = { ...idle, state: ConnectionRunning, since: Date.now() }
    await wrapper.vm.$nextTick()
    expect(wrapper.get('[data-testid="stop"]').find('[data-testid="kill-switch-lock"]').exists()).toBe(true)
  })
})
