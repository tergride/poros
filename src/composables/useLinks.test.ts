import { mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { describe, expect, it } from 'vitest'
import { defineComponent, h } from 'vue'
import { createMemoryHistory, createRouter } from 'vue-router'

import { createAppI18n } from '@/i18n'
import { MethodSubscriptionsAdd } from '@/platform/core-types'
import { useCoreStore } from '@/stores/core'
import { useUiStore } from '@/stores/ui'

import { useLinks } from './useLinks'

const settle = () => new Promise((resolve) => setTimeout(resolve, 20))
const encode = (text: string) => btoa(text).replace(/=+$/, '').replace(/\+/g, '-').replace(/\//g, '_')
// A later start of Poros hands its links over; outside Tauri the event stands in for it.
const send = (link: string) => window.dispatchEvent(new CustomEvent('poros-links', { detail: [link] }))

describe('poros://addsub/ links', () => {
  it('offer an added subscription to update or show, and wait for that dialog before the next link', async () => {
    setActivePinia(createPinia())
    const core = useCoreStore()
    const ui = useUiStore()
    await core.start()
    const added = await core.core().call(MethodSubscriptionsAdd, { url: 'https://sub.example.com/u/1', name: '' })
    await core.load()
    const router = createRouter({ history: createMemoryHistory(), routes: [{ path: '/:rest(.*)*', component: { render: () => null } }] })
    await router.push('/main')
    mount(
      defineComponent({
        setup() {
          useLinks()
          return () => h('div')
        },
      }),
      { global: { plugins: [router, createAppI18n(['ru-RU'])] } },
    )

    send(`poros://addsub/${encode('https://sub.example.com/u/1#Other name')}`)
    send('poros://addsub/https://sub.example.com/u/2?token=a%2Fb&flag=1#My%20VPN')
    await settle()
    expect(ui.knownSubscription).toBe(added.group.id)
    expect(ui.addSubscription).toBe(false)

    ui.knownSubscription = ''
    await settle()
    expect(ui.addSubscription).toBe(true)
    expect(ui.addPrefill).toEqual({ url: 'https://sub.example.com/u/2?token=a%2Fb&flag=1', name: 'My VPN', check: false })
  })
})
