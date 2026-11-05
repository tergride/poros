import { describe, expect, it, vi } from 'vitest'

import { createStubCore, isCoreError } from './core'
import {
  ConnectionIdle,
  ConnectionRunning,
  ConnectionStarting,
  EventConnectionChanged,
  EventServersChanged,
  EventSubscriptionsChanged,
  MethodConnectionStart,
  MethodConnectionStop,
  MethodServersList,
  MethodSettingsGet,
  MethodSettingsSet,
  MethodSubscriptionsAdd,
  MethodSubscriptionsList,
  MethodSubscriptionsRemove,
  ReasonDuplicate,
  ReasonInvalidURL,
  ReasonNoServer,
  ReasonNotFound,
  ReasonSingleServer,
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

  it('keeps subscriptions and their servers by the rules of the engine and announces each change', async () => {
    const core = createStubCore()
    const changes = vi.fn()
    const serverChanges = vi.fn()
    core.on(EventSubscriptionsChanged, changes)
    core.on(EventServersChanged, serverChanges)

    expect(await reasonOf(core.call(MethodSubscriptionsAdd, { url: 'sub.example.com/u/1', name: '' }))).toBe(ReasonInvalidURL)
    expect(await reasonOf(core.call(MethodSubscriptionsAdd, { url: 'vless://id@example.com:443', name: '' }))).toBe(ReasonSingleServer)
    const { subscription: sub, added } = await core.call(MethodSubscriptionsAdd, { url: ' https://sub.example.com/u/1 ', name: '' })
    expect(sub.name).toBe('sub.example.com')
    expect(await reasonOf(core.call(MethodSubscriptionsAdd, { url: 'https://sub.example.com/u/1', name: '' }))).toBe(ReasonDuplicate)
    expect(await core.call(MethodSubscriptionsList)).toEqual([sub])
    const servers = await core.call(MethodServersList, { subscriptionId: sub.id })
    expect(servers).toHaveLength(added)
    expect(sub.servers).toBe(added)

    expect(await reasonOf(core.call(MethodSubscriptionsRemove, { id: 'nope' }))).toBe(ReasonNotFound)
    await core.call(MethodSubscriptionsRemove, { id: sub.id })
    expect(changes).toHaveBeenCalledTimes(2)
    expect(changes).toHaveBeenLastCalledWith([])
    expect(serverChanges).toHaveBeenLastCalledWith({ subscriptionId: sub.id })
    expect(await core.call(MethodServersList, { subscriptionId: '' })).toEqual([])
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

  it('answers a start at once and settles it the way the engine does', async () => {
    const core = createStubCore()
    const states = vi.fn()
    core.on(EventConnectionChanged, states)
    const settle = () => new Promise((resolve) => setTimeout(resolve, 0))

    expect((await core.call(MethodConnectionStart)).state).toBe(ConnectionStarting)
    await settle()
    expect(states).toHaveBeenLastCalledWith(expect.objectContaining({ state: ConnectionIdle, error: ReasonNoServer }))

    await core.call(MethodSubscriptionsAdd, { url: 'https://sub.example.com/u/1', name: '' })
    const [server] = await core.call(MethodServersList, { subscriptionId: '' })
    const settings = await core.call(MethodSettingsGet)
    await core.call(MethodSettingsSet, { ...settings, server: server!.id, tun: true })
    await core.call(MethodConnectionStart)
    await settle()
    expect(states).toHaveBeenLastCalledWith(
      expect.objectContaining({ state: ConnectionRunning, serverId: server!.id, tun: true, error: '' }),
    )

    await core.call(MethodConnectionStop)
    await settle()
    expect(states).toHaveBeenLastCalledWith(expect.objectContaining({ state: ConnectionIdle, error: '' }))
  })
})
