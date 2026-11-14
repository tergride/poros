import { mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it } from 'vitest'
import { defineComponent, h } from 'vue'

import { createAppI18n } from '@/i18n'
import { useCoreStore } from '@/stores/core'

import { useSetting, useSettingAction } from './useSetting'
import { type SettingsDraft, provideSettingsDraft } from './useSettingsDraft'

describe('a settings draft of the advanced mode', () => {
  beforeEach(() => setActivePinia(createPinia()))

  it('holds the changes until save, and applies the actions of their keys then', async () => {
    const core = useCoreStore()
    await core.start()
    const applied: boolean[] = []
    let draft!: SettingsDraft
    let allowLan!: ReturnType<typeof useSetting<'allowLan'>>
    let runAsAdmin!: ReturnType<typeof useSettingAction<'runAsAdmin'>>
    const Page = defineComponent({
      setup() {
        allowLan = useSetting('allowLan')
        runAsAdmin = useSettingAction('runAsAdmin', async (on) => {
          applied.push(on)
          await core.updateSettings({ runAsAdmin: on })
        })
        return () => h('div')
      },
    })
    mount(
      defineComponent({
        setup() {
          draft = provideSettingsDraft()
          return () => h(Page)
        },
      }),
      { global: { plugins: [createAppI18n(['en'])] } },
    )

    allowLan.value = true
    expect(allowLan.value).toBe(true)
    expect(core.settings!.allowLan).toBe(false)
    expect(draft.dirty.value).toBe(true)
    // Back to what is saved is no change.
    allowLan.value = false
    expect(draft.dirty.value).toBe(false)

    allowLan.value = true
    runAsAdmin.value = true
    expect(applied).toEqual([])
    await draft.save()
    expect(core.settings!.allowLan).toBe(true)
    expect(applied).toEqual([true])
    expect(core.settings!.runAsAdmin).toBe(true)
    expect(draft.dirty.value).toBe(false)

    allowLan.value = false
    draft.discard()
    expect(allowLan.value).toBe(true)
  })

  it('leaves settings saving at once outside a draft', async () => {
    const core = useCoreStore()
    await core.start()
    let checkIp!: ReturnType<typeof useSetting<'checkIp'>>
    mount(
      defineComponent({
        setup() {
          checkIp = useSetting('checkIp')
          return () => h('div')
        },
      }),
      { global: { plugins: [createAppI18n(['en'])] } },
    )
    checkIp.value = false
    await new Promise((resolve) => setTimeout(resolve, 20))
    expect(core.settings!.checkIp).toBe(false)
  })
})
