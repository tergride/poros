import { describe, expect, it, vi } from 'vitest'

import { isCoreError } from './core'
import { createStubCore } from './stubCore'
import {
  ConnectionIdle,
  ConnectionRunning,
  ConnectionStarting,
  EventConnectionChanged,
  EventKillSwitchChanged,
  EventNetworkChanged,
  EventServersChanged,
  EventServersTested,
  EventSubscriptionsChanged,
  EventUpdatesChanged,
  KillSwitchAlways,
  KillSwitchConnected,
  KillSwitchOff,
  LinkPreset,
  LinkSubscription,
  MethodConnectionStart,
  MethodConnectionState,
  MethodConnectionStop,
  MethodKillSwitchState,
  MethodLinksParse,
  MethodNetworkCheck,
  MethodPresetsExport,
  MethodPresetsGet,
  MethodPresetsImport,
  MethodPresetsList,
  MethodPresetsReadLists,
  MethodPresetsRemove,
  MethodPresetsSave,
  MethodPresetsWriteLists,
  MethodServersList,
  MethodServersTest,
  MethodSettingsGet,
  MethodSettingsSet,
  MethodSubscriptionsAdd,
  MethodSubscriptionsList,
  MethodSubscriptionsRemove,
  MethodUpdatesCheck,
  MethodUpdatesInstall,
  MethodUpdatesState,
  ReasonCheckOff,
  ReasonDuplicate,
  ReasonInvalidEntry,
  ReasonInvalidLink,
  ReasonInvalidValue,
  ReasonKillSwitchNeedsElevation,
  ReasonNeedsElevation,
  ReasonInvalidURL,
  ReasonNoServer,
  ReasonNotFound,
  ReasonPresetNameTaken,
  ReasonSingleServer,
  ReasonUpdateNotReady,
  UpdateChecking,
  UpdateReady,
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
    // The chosen server goes with its subscription, and a server that is not there cannot be chosen.
    const settings = await core.call(MethodSettingsGet)
    expect(await reasonOf(core.call(MethodSettingsSet, { ...settings, server: 'nope' }))).toBe(ReasonNotFound)
    await core.call(MethodSettingsSet, { ...settings, server: servers[0]!.id })
    await core.call(MethodSubscriptionsRemove, { id: sub.id })
    expect((await core.call(MethodSettingsGet)).server).toBe('')
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

  it('checks for an update and downloads it in the background, as the engine does', async () => {
    const core = createStubCore()
    const states = vi.fn()
    core.on(EventUpdatesChanged, states)
    expect(await core.call(MethodUpdatesState)).toEqual(expect.objectContaining({ available: true, current: '0.1.0' }))
    expect(await reasonOf(core.call(MethodUpdatesInstall))).toBe(ReasonUpdateNotReady)
    expect((await core.call(MethodUpdatesCheck)).state).toBe(UpdateChecking)
    await new Promise((resolve) => setTimeout(resolve, 80))
    expect(states).toHaveBeenLastCalledWith(expect.objectContaining({ state: UpdateReady, version: '0.1.1' }))
    expect((await core.call(MethodUpdatesInstall)).archive).toContain('Poros-0.1.1-windows-amd64.zip')
  })

  it('keeps the kill switch block as the engine does', async () => {
    const core = createStubCore()
    const blocks = vi.fn()
    core.on(EventKillSwitchChanged, blocks)
    const settle = () => new Promise((resolve) => setTimeout(resolve, 0))
    await core.call(MethodSubscriptionsAdd, { url: 'https://sub.example.com/u/1', name: '' })
    const [server] = await core.call(MethodServersList, { subscriptionId: '' })
    let settings = await core.call(MethodSettingsGet)
    expect(settings.killSwitch).toBe(KillSwitchOff)
    expect(await reasonOf(core.call(MethodSettingsSet, { ...settings, killSwitch: 'on' }))).toBe(ReasonInvalidValue)

    // Connected: a Start without rights does not connect, and says why.
    settings = await core.call(MethodSettingsSet, { ...settings, server: server!.id, systemProxy: true, killSwitch: KillSwitchConnected })
    await core.call(MethodConnectionStart)
    await settle()
    expect((await core.call(MethodConnectionState)).error).toBe(ReasonKillSwitchNeedsElevation)
    expect(await core.call(MethodKillSwitchState)).toEqual({ on: false, error: ReasonKillSwitchNeedsElevation, errorText: expect.any(String) })

    // With rights the Start blocks; the block outlives the core, and Stop from a core without rights lifts it.
    await core.elevate()
    await settle()
    await core.call(MethodConnectionStart)
    await settle()
    expect((await core.call(MethodKillSwitchState)).on).toBe(true)
    await core.restart()
    await settle()
    expect((await core.call(MethodKillSwitchState)).on).toBe(true)
    await core.call(MethodConnectionStop)
    await settle()
    expect(await core.call(MethodKillSwitchState)).toEqual({ on: false, error: '', errorText: '' })

    // Always: on at once with rights, and Stop keeps it.
    await core.elevate()
    await settle()
    await core.call(MethodSettingsSet, { ...settings, killSwitch: KillSwitchAlways })
    await settle()
    expect((await core.call(MethodKillSwitchState)).on).toBe(true)
    await core.call(MethodConnectionStop)
    await settle()
    expect((await core.call(MethodKillSwitchState)).on).toBe(true)
    await core.call(MethodSettingsSet, { ...settings, killSwitch: KillSwitchOff })
    await settle()
    expect(blocks).toHaveBeenLastCalledWith({ on: false, error: '', errorText: '' })
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
    // As in the engine, a name is taken once: the imported copy gets a number, a second save is refused.
    expect(imported).toMatchObject({ name: 'Home 2', sites: 1, apps: 1 })
    expect(await reasonOf(core.call(MethodPresetsSave, { ...draft, name: ' home ' }))).toBe(ReasonPresetNameTaken)
    expect(await reasonOf(core.call(MethodPresetsSave, { ...draft, name: 'All traffic' }))).toBe(ReasonPresetNameTaken)

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

  it('reads the lists window as the engine does', async () => {
    const core = createStubCore()
    const read = await core.call(MethodPresetsReadLists, {
      lists: {
        proxy: 'video.example.com\r\n# work\n\nhttps://Mail.example.org/inbox\nnot a site\nDOMAIN:media.example.net\nvideo.example.com',
        direct: '192.168.50.0/24\nip:203.0.113.7\nsteam.exe\nC:\\Games\\game.exe\nprocessName:steam\n*.video.example.com',
        block: '*.ads.example\ngeosite:category-ads\n*.\nSTEAM.EXE',
      },
    })
    expect(read.sites.map((e) => `${e.action} ${e.value}`)).toEqual([
      'proxy video.example.com',
      'proxy https://Mail.example.org/inbox',
      'proxy media.example.net',
      'direct 192.168.50.0/24',
      'direct 203.0.113.7',
      'block *.ads.example',
    ])
    expect(read.apps.map((e) => `${e.action} ${e.value}`)).toEqual(['direct steam.exe', 'direct C:\\Games\\game.exe', 'direct steam'])
    expect(read.problems).toEqual([
      { action: 'proxy', line: 5, text: 'not a site', reason: ReasonInvalidEntry, other: '' },
      { action: 'direct', line: 6, text: '*.video.example.com', reason: ReasonDuplicate, other: 'proxy' },
      { action: 'block', line: 2, text: 'geosite:category-ads', reason: ReasonInvalidEntry, other: '' },
      { action: 'block', line: 3, text: '*.', reason: ReasonInvalidEntry, other: '' },
      { action: 'block', line: 4, text: 'STEAM.EXE', reason: ReasonDuplicate, other: 'direct' },
    ])
    expect(await reasonOf(core.call(MethodPresetsReadLists, { lists: { warp: '' } }))).toBe(ReasonInvalidValue)

    const entries = {
      sites: [
        { value: 'example.org', action: 'block' },
        { value: 'setup.exe', action: 'proxy' },
      ],
      apps: [
        { value: 'steam', action: 'direct' },
        { value: 'steam.exe', action: 'direct' },
      ],
    }
    const lists = await core.call(MethodPresetsWriteLists, entries)
    expect(lists.lists).toEqual({ proxy: 'domain:setup.exe', direct: 'processName:steam\nsteam.exe', block: 'example.org' })
    const back = await core.call(MethodPresetsReadLists, lists)
    expect(back).toEqual({ sites: [entries.sites[1], entries.sites[0]], apps: entries.apps, problems: [] })
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
