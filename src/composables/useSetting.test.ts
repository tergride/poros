import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it } from 'vitest'
import { defineComponent, h } from 'vue'

import { createAppI18n } from '@/i18n'
import { useCoreStore } from '@/stores/core'
import { useUiStore } from '@/stores/ui'

import { useSetting } from './useSetting'

function mountSetting<K extends 'allowLan' | 'preset'>(key: K) {
  let setting!: ReturnType<typeof useSetting<K>>
  mount(
    defineComponent({
      setup() {
        setting = useSetting(key)
        return () => h('div')
      },
    }),
    { global: { plugins: [createAppI18n(['en'])] } },
  )
  return setting
}

describe('a setting applied at once', () => {
  beforeEach(() => setActivePinia(createPinia()))

  it('shows the new value while the core applies it and takes no other until then', async () => {
    const core = useCoreStore()
    await core.start()
    const setting = mountSetting('allowLan')
    const applying = () => 'allowLan' in useUiStore().applying
    setting.value = true
    expect(setting.value).toBe(true)
    expect(applying()).toBe(true)
    setting.value = false
    expect(setting.value).toBe(true)
    await flushPromises()
    expect(applying()).toBe(false)
    expect(core.settings!.allowLan).toBe(true)
    expect(setting.value).toBe(true)
  })

  it('goes back to the value of the core when the core refuses it', async () => {
    const core = useCoreStore()
    await core.start()
    const setting = mountSetting('preset')
    setting.value = 'no-such-preset'
    expect(setting.value).toBe('no-such-preset')
    await flushPromises()
    expect(core.settings!.preset).toBe('all')
    expect(setting.value).toBe('all')
    expect(useUiStore().notice?.tone).toBe('warn')
  })
})
