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

  it('lets the window be dragged by the bar itself', () => {
    const wrapper = mount(TitleBar, { global: { plugins: [createAppI18n(['en-US'])] } })
    expect(wrapper.get('header').attributes()).toHaveProperty('data-tauri-drag-region')
  })
})
