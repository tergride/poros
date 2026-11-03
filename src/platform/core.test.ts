import { describe, expect, it, vi } from 'vitest'

import { createStubCore, isCoreError } from './core'
import {
  EventSubscriptionsChanged,
  MethodSettingsGet,
  MethodSettingsSet,
  MethodSubscriptionsAdd,
  MethodSubscriptionsList,
  MethodSubscriptionsRemove,
  ReasonDuplicate,
  ReasonInvalidURL,
  ReasonNotFound,
} from './core-types'

async function reasonOf(promise: Promise<unknown>): Promise<string | undefined> {
  try {
    await promise
  } catch (error) {
    return isCoreError(error) ? error.data?.reason : undefined
  }
  return undefined
}

describe('the stub core', () => {
  it('starts with TUN and System off, as the engine does', async () => {
    const settings = await createStubCore().call(MethodSettingsGet)
    expect(settings.tun).toBe(false)
    expect(settings.systemProxy).toBe(false)
  })

  it('keeps subscriptions by the rules of the engine and announces each change', async () => {
    const core = createStubCore()
    const changes = vi.fn()
    core.on(EventSubscriptionsChanged, changes)

    expect(await reasonOf(core.call(MethodSubscriptionsAdd, { url: 'vless://id@example.com:443', name: '' }))).toBe(ReasonInvalidURL)
    const sub = await core.call(MethodSubscriptionsAdd, { url: ' https://sub.example.com/u/1 ', name: '' })
    expect(sub.name).toBe('sub.example.com')
    expect(await reasonOf(core.call(MethodSubscriptionsAdd, { url: 'https://sub.example.com/u/1', name: '' }))).toBe(ReasonDuplicate)
    expect(await core.call(MethodSubscriptionsList)).toEqual([sub])

    expect(await reasonOf(core.call(MethodSubscriptionsRemove, { id: 'nope' }))).toBe(ReasonNotFound)
    await core.call(MethodSubscriptionsRemove, { id: sub.id })
    expect(changes).toHaveBeenCalledTimes(2)
    expect(changes).toHaveBeenLastCalledWith([])
  })

  it('stops delivering events to a handler once it unsubscribes', async () => {
    const core = createStubCore()
    const handler = vi.fn()
    const off = core.on('settings.changed', handler)
    const settings = await core.call(MethodSettingsGet)
    await core.call(MethodSettingsSet, { ...settings, tun: true })
    off()
    await core.call(MethodSettingsSet, { ...settings, tun: false })
    expect(handler).toHaveBeenCalledOnce()
  })
})
