import { mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it } from 'vitest'
import { defineComponent, h } from 'vue'

import { createAppI18n } from '@/i18n'
import { useCoreStore } from '@/stores/core'

import { type SettingsBlock, type SettingsPage, provideSettingsPage, useSettingsBlock } from './useSettingsBlock'

describe('the blocks of a settings page', () => {
  beforeEach(() => setActivePinia(createPinia()))

  it('hold their changes until Apply and send them together', async () => {
    const core = useCoreStore()
    await core.start()
    let block!: SettingsBlock
    mount(
      defineComponent({
        setup() {
          block = useSettingsBlock()
          return () => h('div')
        },
      }),
      { global: { plugins: [createAppI18n(['en'])] } },
    )
    const auth = block.setting('proxyAuth')
    const username = block.setting('proxyUsername')
    const password = block.setting('proxyPassword')

    auth.value = true
    expect(auth.value).toBe(true)
    expect(core.settings!.proxyAuth).toBe(false)
    expect(block.dirty).toBe(true)
    // Back to what is saved is no change.
    auth.value = false
    expect(block.dirty).toBe(false)

    auth.value = true
    username.value = 'me'
    password.value = 'secret'
    expect(await block.apply()).toBe(true)
    expect(core.settings).toMatchObject({ proxyAuth: true, proxyUsername: 'me', proxyPassword: 'secret' })
    expect(block.dirty).toBe(false)
  })

  it('keep refused values with the reason until they are fixed or discarded', async () => {
    const core = useCoreStore()
    await core.start()
    let block!: SettingsBlock
    mount(
      defineComponent({
        setup() {
          block = useSettingsBlock()
          return () => h('div')
        },
      }),
      { global: { plugins: [createAppI18n(['en'])] } },
    )
    const url = block.setting('latencyUrl')
    const lines = block.setting('logLines')
    const saved = { latencyUrl: core.settings!.latencyUrl, logLines: core.settings!.logLines }

    // Too few lines are refused, and nothing of the block is saved.
    url.value = 'http://example.com/test'
    lines.value = 50
    expect(await block.apply()).toBe(false)
    expect(block.problem).toContain('Invalid value')
    expect(lines.value).toBe(50)
    expect(core.settings).toMatchObject(saved)
    // A change clears the reason; Discard brings back what is saved.
    lines.value = 60
    expect(block.problem).toBe('')
    block.discard()
    expect(block.dirty).toBe(false)
    expect(url.value).toBe(saved.latencyUrl)
    expect(lines.value).toBe(saved.logLines)
  })

  it('make the page ask before leaving, and apply or discard all of them', async () => {
    const core = useCoreStore()
    await core.start()
    let page!: SettingsPage
    let lines!: SettingsBlock
    let tun!: SettingsBlock
    const Page = defineComponent({
      setup() {
        lines = useSettingsBlock()
        tun = useSettingsBlock()
        return () => h('div')
      },
    })
    const wrapper = mount(
      defineComponent({
        setup() {
          page = provideSettingsPage()
          return () => h(Page)
        },
      }),
      { global: { plugins: [createAppI18n(['en'])] } },
    )

    lines.setting('logLines').value = 900
    tun.setting('tunIpv4').value = '172.20.1.1/24'
    expect(page.dirty).toBe(true)
    page.discard()
    expect(page.dirty).toBe(false)

    // One refused block keeps the page; the other is applied.
    lines.setting('logLines').value = 50
    tun.setting('tunIpv4').value = '172.20.1.1/24'
    expect(await page.apply()).toBe(false)
    expect(core.settings!.tunIpv4).toBe('172.20.1.1/24')
    expect(lines.problem).not.toBe('')
    expect(page.dirty).toBe(true)
    lines.setting('logLines').value = 900
    expect(await page.apply()).toBe(true)
    expect(core.settings!.logLines).toBe(900)
    expect(page.dirty).toBe(false)

    // A page that is gone leaves its blocks behind.
    wrapper.unmount()
    expect(page.dirty).toBe(false)
  })
})
