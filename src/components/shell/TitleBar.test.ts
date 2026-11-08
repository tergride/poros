import { mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import { createAppI18n } from '@/i18n'
import { windowControls } from '@/platform'

import TitleBar from './TitleBar.vue'

vi.mock('@/platform', () => ({
  windowControls: {
    minimize: vi.fn(() => Promise.resolve()),
    toggleMaximize: vi.fn(() => Promise.resolve()),
    close: vi.fn(() => Promise.resolve()),
  },
}))

describe('TitleBar', () => {
  beforeEach(() => vi.clearAllMocks())

  it('drives the window from its buttons', async () => {
    const wrapper = mount(TitleBar, { global: { plugins: [createAppI18n(['ru-RU'])] } })

    await wrapper.get('button[aria-label="Свернуть"]').trigger('click')
    await wrapper.get('button[aria-label="Развернуть"]').trigger('click')
    await wrapper.get('button[aria-label="Закрыть"]').trigger('click')

    expect(windowControls.minimize).toHaveBeenCalledOnce()
    expect(windowControls.toggleMaximize).toHaveBeenCalledOnce()
    expect(windowControls.close).toHaveBeenCalledOnce()
  })

  it('shows a chip for the kill switch while it is on, and each chip tells what it means', () => {
    const mountBar = (props: { elevated?: boolean; killSwitch?: string }) =>
      mount(TitleBar, { props, global: { plugins: [createAppI18n(['ru-RU'])] } })

    expect(mountBar({ killSwitch: 'off' }).find('[data-testid="kill-switch-chip"]').exists()).toBe(false)
    const always = mountBar({ killSwitch: 'always', elevated: true })
    expect(always.get('[data-testid="kill-switch-chip"]').text()).toBe('Kill switch')
    expect(always.get('[data-testid="kill-switch-chip"]').attributes('title')).toMatch(/^Kill switch всегда: /)
    expect(always.get('[data-testid="admin-chip"]').attributes('title')).toMatch(/^Ядро Poros работает с правами администратора/)
    expect(mountBar({ killSwitch: 'connected' }).get('[data-testid="kill-switch-chip"]').attributes('title')).toMatch(
      /^Kill switch при подключении: /,
    )
  })

  it('turns the kill switch chip red while it blocks and opens the setting from it', async () => {
    const plugins = [createAppI18n(['ru-RU'])]
    const guarding = mount(TitleBar, { props: { killSwitch: 'connected' }, global: { plugins } })
    expect(guarding.get('[data-testid="kill-switch-chip"]').classes()).not.toContain('text-bad')
    await guarding.get('[data-testid="kill-switch-chip"]').trigger('click')
    expect(guarding.emitted('killSwitch')).toHaveLength(1)

    const held = mount(TitleBar, { props: { killSwitch: 'connected', killSwitchBlocking: 'held' }, global: { plugins } })
    const chip = held.get('[data-testid="kill-switch-chip"]')
    expect(chip.classes()).toContain('text-bad')
    expect(chip.attributes('title')).toMatch(/^Kill switch сейчас блокирует трафик VPN: соединение с сервером прервано/)
    const full = mount(TitleBar, { props: { killSwitch: 'always', killSwitchBlocking: 'always', killSwitchFull: true }, global: { plugins } })
    expect(full.get('[data-testid="kill-switch-chip"]').attributes('title')).toMatch(/^Kill switch сейчас блокирует весь интернет/)
  })

  it('offers the restart of saved settings with a chip that asks for it', async () => {
    const plugins = [createAppI18n(['ru-RU'])]
    expect(mount(TitleBar, { global: { plugins } }).find('[data-testid="restart-chip"]').exists()).toBe(false)
    const wrapper = mount(TitleBar, { props: { restartNeeded: true }, global: { plugins } })
    const chip = wrapper.get('[data-testid="restart-chip"]')
    expect(chip.text()).toBe('Нужен перезапуск')
    expect(chip.attributes('title')).toMatch(/^Настройки сохранены, но не применены/)
    await chip.trigger('click')
    expect(wrapper.emitted('restart')).toHaveLength(1)
  })

  it('lets the window be dragged by the bar itself', () => {
    const wrapper = mount(TitleBar, { global: { plugins: [createAppI18n(['en-US'])] } })
    expect(wrapper.get('header').attributes()).toHaveProperty('data-tauri-drag-region')
  })
})
