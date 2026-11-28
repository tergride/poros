import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it } from 'vitest'
import { defineComponent, h } from 'vue'

import { createAppI18n } from '@/i18n'
import { useCoreStore } from '@/stores/core'
import { useUiStore } from '@/stores/ui'

import { useApplying, useSetting } from './useSetting'

function mountSetting<K extends 'allowLan' | 'adminAutostart'>(key: K) {
  let setting!: ReturnType<typeof useSetting<K>>
  let applying!: ReturnType<typeof useApplying>
  mount(
    defineComponent({
      setup() {
        setting = useSetting(key)
        applying = useApplying(key)
        return () => h('div')
      },
    }),
    { global: { plugins: [createAppI18n(['en'])] } },
  )
  return { setting, applying }
}

describe('a setting applied at once', () => {
  beforeEach(() => setActivePinia(createPinia()))

  it('shows the new value while the core applies it and takes no other until then', async () => {
    const core = useCoreStore()
    await core.start()
    const { setting, applying } = mountSetting('allowLan')
    setting.value = true
    expect(setting.value).toBe(true)
    expect(applying.value).toBe(true)
    setting.value = false
    expect(setting.value).toBe(true)
    await flushPromises()
    expect(applying.value).toBe(false)
    expect(core.settings!.allowLan).toBe(true)
    expect(setting.value).toBe(true)
  })

  it('goes back to the value of the core when the core refuses it', async () => {
    const core = useCoreStore()
    await core.start()
    // Rights at sign-in take a core with administrator rights.
    const { setting } = mountSetting('adminAutostart')
    setting.value = true
    expect(setting.value).toBe(true)
    await flushPromises()
    expect(core.settings!.adminAutostart).toBe(false)
    expect(setting.value).toBe(false)
    expect(useUiStore().notice?.tone).toBe('warn')
  })
})
