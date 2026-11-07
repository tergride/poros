import { describe, expect, it, vi } from 'vitest'

import { isCoreError } from './core'
import { createStubCore } from './stubCore'
import {
  ConnectionIdle,
  ConnectionRunning,
  ConnectionStarting,
  EventConnectionChanged,
  EventNetworkChanged,
  EventServersChanged,
  EventServersTested,
  EventSubscriptionsChanged,
  LinkPreset,
  LinkSubscription,
  MethodConnectionStart,
  MethodConnectionStop,
  MethodLinksParse,
  MethodNetworkCheck,
  MethodPresetsExport,
  MethodPresetsGet,
  MethodPresetsImport,
  MethodPresetsList,
  MethodPresetsRemove,
  MethodPresetsSave,
  MethodServersList,
  MethodServersTest,
  MethodSettingsGet,
  MethodSettingsSet,
  MethodSubscriptionsAdd,
  MethodSubscriptionsList,
  MethodSubscriptionsRemove,
  ReasonCheckOff,
  ReasonDuplicate,
  ReasonInvalidEntry,
  ReasonInvalidLink,
  ReasonNeedsElevation,
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
    // TUN needs a core with administrator rights.
    expect(states).toHaveBeenLastCalledWith(expect.objectContaining({ state: ConnectionIdle, error: ReasonNeedsElevation }))
    await core.elevate()
    await settle()
    expect(await core.status()).toEqual({ state: 'ready', elevated: true })
    await core.call(MethodConnectionStart)
    await settle()
    expect(states).toHaveBeenLastCalledWith(
      expect.objectContaining({ state: ConnectionRunning, serverId: server!.id, tun: true, error: '' }),
    )

    await core.call(MethodConnectionStop)
    await settle()
    expect(states).toHaveBeenLastCalledWith(expect.objectContaining({ state: ConnectionIdle, error: '' }))
  })

  it('checks the addresses only while the user keeps the check on', async () => {
    const core = createStubCore()
    const checks = vi.fn()
    core.on(EventNetworkChanged, checks)
    const settings = await core.call(MethodSettingsGet)
    await core.call(MethodSettingsSet, { ...settings, checkIp: false })
    expect(await reasonOf(core.call(MethodNetworkCheck))).toBe(ReasonCheckOff)

    await core.call(MethodSettingsSet, { ...settings, checkIp: true })
    expect((await core.call(MethodNetworkCheck)).checking).toBe(true)
    await new Promise((resolve) => setTimeout(resolve, 0))
    // Without a connection there is neither an address through the VPN nor a leak test.
    expect(checks).toHaveBeenLastCalledWith(
      expect.objectContaining({ checking: false, proxy: expect.objectContaining({ ip: '' }), leak: '' }),
    )
  })

  it('measures the servers and remembers the delay', async () => {
    const core = createStubCore()
    const tested = vi.fn()
    core.on(EventServersTested, tested)
    await core.call(MethodSubscriptionsAdd, { url: 'https://sub.example.com/u/1', name: '' })
    await core.call(MethodServersTest, { subscriptionId: '' })
    await new Promise((resolve) => setTimeout(resolve, 0))
    expect(tested).toHaveBeenLastCalledWith(expect.objectContaining({ done: true }))
    const latencies = (await core.call(MethodServersList, { subscriptionId: '' })).map((s) => s.latency)
    expect(latencies).toEqual([42, 48, -1])
  })

  it('keeps preset copies by the rules of the engine', async () => {
    const core = createStubCore()
    const draft = await core.call(MethodPresetsGet, { id: 'all' })
    expect(draft).toMatchObject({ id: '', basedOn: 'all', inherited: [{ action: 'direct' }] })
    expect(
      await reasonOf(core.call(MethodPresetsSave, { ...draft, name: 'Home', sites: [{ value: 'not a site', action: 'proxy' }] })),
    ).toBe(ReasonInvalidEntry)
    const saved = await core.call(MethodPresetsSave, {
      ...draft,
      name: 'Home',
      sites: [{ value: 'example.org', action: 'block' }],
      apps: [{ value: 'steam.exe', action: 'direct' }],
    })
    expect(saved).toMatchObject({ builtin: false, basedOn: 'all', sites: 1, apps: 1 })

    const settings = await core.call(MethodSettingsGet)
    await core.call(MethodSettingsSet, { ...settings, preset: saved.id })
    const { link } = await core.call(MethodPresetsExport, { id: saved.id })
    const imported = await core.call(MethodPresetsImport, { link })
    expect(imported).toMatchObject({ name: 'Home', sites: 1, apps: 1 })

    await core.call(MethodPresetsRemove, { id: saved.id })
    expect((await core.call(MethodSettingsGet)).preset).toBe('all')
    expect((await core.call(MethodPresetsList)).map((p) => p.id)).toEqual(['all', 'hardened', imported.id])
  })

  it('makes a preset from scratch that has no base and falls back to the standard one', async () => {
    const core = createStubCore()
    const draft = await core.call(MethodPresetsGet, { id: '' })
    expect(draft).toMatchObject({ basedOn: '', name: '', inherited: [{ action: 'direct' }] })
    const saved = await core.call(MethodPresetsSave, { ...draft, name: 'Work' })
    expect(saved).toMatchObject({ builtin: false, basedOn: '' })
    const settings = await core.call(MethodSettingsGet)
    await core.call(MethodSettingsSet, { ...settings, preset: saved.id })
    await core.call(MethodPresetsRemove, { id: saved.id })
    expect((await core.call(MethodSettingsGet)).preset).toBe('all')
  })

  it('reads poros:// links as the engine does and adds nothing', async () => {
    const core = createStubCore()
    const encode = (text: string) => btoa(text).replaceAll('+', '-').replaceAll('/', '_').replace(/=+$/, '')
    expect(await core.call(MethodLinksParse, { link: `poros://AddSub/${encode('https://sub.example.com/u/8f3a#My%20VPN')}` })).toEqual({
      kind: LinkSubscription,
      url: 'https://sub.example.com/u/8f3a',
      name: 'My VPN',
    })

    const draft = await core.call(MethodPresetsGet, { id: 'all' })
    const saved = await core.call(MethodPresetsSave, { ...draft, name: 'Home', dns: 'tls://1.1.1.1' })
    const { link } = await core.call(MethodPresetsExport, { id: saved.id })
    const info = await core.call(MethodLinksParse, { link })
    expect(info).toMatchObject({ kind: LinkPreset, preset: { name: 'Home', basedOn: 'all', dns: 'tls://1.1.1.1' } })
    expect(await core.call(MethodPresetsList)).toHaveLength(3)

    expect(await reasonOf(core.call(MethodLinksParse, { link: `poros://add/${encode('{}')}` }))).toBe(ReasonSingleServer)
    expect(await reasonOf(core.call(MethodLinksParse, { link: `throne://route/${encode('{}')}` }))).toBe(ReasonInvalidLink)
    expect(await reasonOf(core.call(MethodLinksParse, { link: 'https://sub.example.com/u/8f3a' }))).toBe(ReasonInvalidLink)
  })
})
