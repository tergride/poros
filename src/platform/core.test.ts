import { describe, expect, it, vi } from 'vitest'

import { isCoreError } from './core'
import { createStubCore } from './stubCore'
import {
  ConnectionIdle,
  ConnectionRunning,
  ConnectionStarting,
  CoreSingBox,
  EventConnectionChanged,
  EventKillSwitchChanged,
  EventNetworkChanged,
  EventServersChanged,
  EventServersSpeedTested,
  EventGroupsChanged,
  EventServersTested,
  EventUpdatesChanged,
  GroupManual,
  KillSwitchAlways,
  KillSwitchConnected,
  KillSwitchOff,
  LinkPreset,
  LinkServer,
  LinkSubscription,
  MethodConnectionStart,
  MethodConnectionState,
  MethodConnectionStop,
  MethodDNSCheck,
  MethodGroupsAdd,
  MethodGroupsConfigure,
  MethodGroupsList,
  MethodGroupsRemove,
  MethodKillSwitchState,
  MethodLinksParse,
  MethodNetworkCheck,
  MethodPresetsCheck,
  MethodPresetsExport,
  MethodPresetsGet,
  MethodPresetsImport,
  MethodPresetsList,
  MethodPresetsReadLists,
  MethodPresetsRemove,
  MethodPresetsSave,
  MethodPresetsWriteLists,
  MethodServersAdd,
  MethodServersClone,
  MethodServersDeepLinks,
  MethodServersExport,
  MethodServersLinks,
  MethodServersGet,
  MethodServersList,
  MethodServersMove,
  MethodServersRemove,
  MethodServersSave,
  MethodServersClean,
  MethodServersSpeedTest,
  MethodServersTest,
  MethodSettingsGet,
  MethodSettingsSet,
  MethodSubscriptionsAdd,
  MethodUpdatesCheck,
  MethodUpdatesInstall,
  MethodUpdatesState,
  ReasonAdvancedOnly,
  ReasonCheckOff,
  ReasonDuplicate,
  ReasonInvalidEntry,
  ReasonInvalidLink,
  ReasonInvalidValue,
  ReasonKillSwitchNeedsElevation,
  ReasonNeedsElevation,
  ReasonNoPrefix,
  ReasonInvalidURL,
  ReasonNoServer,
  ReasonNoServers,
  ReasonNotFound,
  ReasonNotManual,
  ReasonPresetNameTaken,
  ReasonSingleServer,
  ReasonSubscriptionURL,
  ReasonUpdateNotReady,
  UpdateChecking,
  UpdateReady,
} from './core-types'
import type { PresetCopy, PresetRule } from './core-types'

const rule = (kind: string, value: string, action: string): PresetRule => ({ name: '', conditions: [{ kind, values: [value] }], invert: false, off: false, action, endpoint: '' })
const emptyCopy: PresetCopy = { id: '', builtin: false, basedOn: '', name: '', advanced: false, rules: [], dns: '', directDns: '', default: '', inherited: [], endpoints: [] }

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
    core.on(EventGroupsChanged, changes)
    core.on(EventServersChanged, serverChanges)

    expect(await reasonOf(core.call(MethodSubscriptionsAdd, { url: 'sub.example.com/u/1', name: '' }))).toBe(ReasonInvalidURL)
    expect(await reasonOf(core.call(MethodSubscriptionsAdd, { url: 'vless://id@example.com:443', name: '' }))).toBe(ReasonSingleServer)
    const { group: sub, added } = await core.call(MethodSubscriptionsAdd, { url: ' https://sub.example.com/u/1 ', name: '' })
    expect(sub.name).toBe('sub.example.com')
    expect(await reasonOf(core.call(MethodSubscriptionsAdd, { url: 'https://sub.example.com/u/1', name: '' }))).toBe(ReasonDuplicate)
    expect(await core.call(MethodGroupsList)).toEqual([sub])
    const servers = await core.call(MethodServersList, { groupId: sub.id })
    expect(servers).toHaveLength(added)
    expect(sub.servers).toBe(added)

    expect(await reasonOf(core.call(MethodGroupsRemove, { id: 'nope' }))).toBe(ReasonNotFound)
    // The chosen server goes with its subscription, and a server that is not there cannot be chosen.
    const settings = await core.call(MethodSettingsGet)
    expect(await reasonOf(core.call(MethodSettingsSet, { ...settings, server: 'nope' }))).toBe(ReasonNotFound)
    await core.call(MethodSettingsSet, { ...settings, server: servers[0]!.id })
    await core.call(MethodGroupsRemove, { id: sub.id })
    expect((await core.call(MethodSettingsGet)).server).toBe('')
    expect(changes).toHaveBeenCalledTimes(2)
    expect(changes).toHaveBeenLastCalledWith([])
    expect(serverChanges).toHaveBeenLastCalledWith({ groupId: sub.id })
    expect(await core.call(MethodServersList, { groupId: '' })).toEqual([])
  })

  it('keeps groups of the user and their servers by the rules of the engine', async () => {
    const core = createStubCore()
    const vless = (host: string, name: string) => `vless://b831381d-6324-4d53-ad4f-8cda48b30811@${host}:443?security=tls#${encodeURIComponent(name)}`
    const mine = await core.call(MethodGroupsAdd, { name: ' Mine ' })
    expect(mine).toEqual(expect.objectContaining({ kind: GroupManual, name: 'Mine', servers: 0 }))
    const { group: sub } = await core.call(MethodSubscriptionsAdd, { url: 'https://sub.example.com/u/1', name: '' })
    expect((await core.call(MethodGroupsList)).map((g) => g.id)).toEqual([sub.id, mine.id])

    expect(await reasonOf(core.call(MethodServersAdd, { groupId: mine.id, newGroup: '', text: 'https://sub.example.com/u/2' }))).toBe(ReasonSubscriptionURL)
    expect(await reasonOf(core.call(MethodServersAdd, { groupId: mine.id, newGroup: '', text: 'a note' }))).toBe(ReasonNoServers)
    expect(await reasonOf(core.call(MethodServersAdd, { groupId: sub.id, newGroup: '', text: vless('de.example.com', 'DE') }))).toBe(ReasonNotManual)
    const added = await core.call(MethodServersAdd, { groupId: mine.id, newGroup: '', text: `${vless('fi.example.com', '🇫🇮 Finland')}
vless://broken
${vless('se.example.com', 'Sweden')}` })
    expect(added.ids).toHaveLength(2)
    expect(added.group.servers).toBe(2)
    expect(added.skips).toEqual([expect.objectContaining({ count: 1 })])
    const listed = await core.call(MethodServersList, { groupId: mine.id })
    expect(listed.map((s) => [s.name, s.country])).toEqual([['🇫🇮 Finland', 'FI'], ['Sweden', '']])

    const { profile } = await core.call(MethodServersGet, { id: added.ids[0]! })
    expect(profile.server).toBe('fi.example.com')
    expect((await core.call(MethodServersSave, { id: added.ids[0]!, groupId: '', profile: { ...profile, name: 'Finland 2' } })).name).toBe('Finland 2')
    expect(await reasonOf(core.call(MethodServersSave, { id: '', groupId: mine.id, profile: { ...profile, server: '' } }))).toBe(ReasonInvalidValue)

    const { ids: [copy] } = await core.call(MethodServersClone, { ids: [added.ids[0]!] })
    expect((await core.call(MethodServersList, { groupId: mine.id })).map((s) => s.id)).toEqual([added.ids[0], copy, added.ids[1]])
    const other = await core.call(MethodGroupsAdd, { name: 'Other' })
    const [fromSub] = await core.call(MethodServersList, { groupId: sub.id })
    const moved = await core.call(MethodServersMove, { ids: [added.ids[1]!, fromSub!.id], groupId: other.id })
    expect(moved.ids[0]).toBe(added.ids[1])
    expect(moved.ids[1]).not.toBe(fromSub!.id)
    expect(await core.call(MethodServersList, { groupId: sub.id })).toHaveLength(3)

    // A removed choice stays in its group.
    const settings = await core.call(MethodSettingsGet)
    await core.call(MethodSettingsSet, { ...settings, server: added.ids[0]! })
    await core.call(MethodServersRemove, { ids: [added.ids[0]!] })
    expect((await core.call(MethodSettingsGet)).server).toBe(copy)
  })

  it('tests and cleans servers by the rules of the engine', async () => {
    const core = createStubCore()
    const { group } = await core.call(MethodSubscriptionsAdd, { url: 'https://sub.example.com/u/1', name: '' })
    const [first] = await core.call(MethodServersList, { groupId: group.id })
    const speed = new Promise((resolve) => core.on(EventServersSpeedTested, resolve))
    await core.call(MethodServersSpeedTest, { groupId: '', ids: [first!.id] })
    expect(await speed).toMatchObject({ results: [{ serverId: first!.id }], done: true })
    const tested = new Promise((resolve) => core.on(EventServersTested, (e) => e.done && resolve(e)))
    await core.call(MethodServersTest, { groupId: group.id })
    await tested
    const { ids } = await core.call(MethodServersClean, { groupId: group.id, unavailable: true, duplicates: false, insecure: false })
    expect(ids).toHaveLength(1)
    expect(await core.call(MethodServersList, { groupId: group.id })).toHaveLength(2)
  })

  it('configures a group by the rules of the engine', async () => {
    const core = createStubCore()
    const { group } = await core.call(MethodSubscriptionsAdd, { url: 'https://sub.example.com/u/1', name: '' })
    const settings = { ...group.settings, serverAddress: '104.16.1.1', test: true, sortByLatency: true }
    const configured = await core.call(MethodGroupsConfigure, { id: group.id, url: '', settings })
    expect(configured.settings).toEqual(settings)
    expect(configured.url).toBe(group.url)
    const unsorted = { ...settings, test: false }
    expect(await reasonOf(core.call(MethodGroupsConfigure, { id: group.id, url: '', settings: unsorted }))).toBe(ReasonInvalidValue)
  })

  it('reads a poros://add/ link without adding the server', async () => {
    const core = createStubCore()
    const json = JSON.stringify({ type: 'trojan', name: '🇩🇪 Home', server: 'de.example.com', server_port: 443, trojan: { password: 'p' } })
    const link = 'poros://add/' + btoa(String.fromCharCode(...new TextEncoder().encode(json))).replaceAll('+', '-').replaceAll('/', '_')
    const info = await core.call(MethodLinksParse, { link })
    expect(info.kind).toBe(LinkServer)
    expect(info.server).toEqual(expect.objectContaining({ name: '🇩🇪 Home', protocol: 'trojan', host: 'de.example.com', country: 'DE' }))
    expect(await core.call(MethodGroupsList)).toEqual([])
    expect(await reasonOf(core.call(MethodLinksParse, { link: 'poros://add/%%%' }))).toBe(ReasonInvalidLink)

    const outbound = JSON.stringify({ type: 'trojan', tag: 'Throne', server: 'th.example.com', server_port: 8443, password: 'p' })
    const throne = 'throne://add/' + btoa(outbound).replaceAll('+', '-').replaceAll('/', '_')
    expect((await core.call(MethodLinksParse, { link: throne })).server).toEqual(
      expect.objectContaining({ name: 'Throne', protocol: 'trojan', host: 'th.example.com', port: 8443 }),
    )
  })

  it('gives servers back as links and as an outbound', async () => {
    const core = createStubCore()
    const added = await core.call(MethodServersAdd, { groupId: '', newGroup: 'Mine', text: 'trojan://p@tr.example.com:443#Home' })
    const id = added.ids[0]!
    const { links } = await core.call(MethodServersLinks, { ids: [id] })
    const { links: deep } = await core.call(MethodServersDeepLinks, { ids: [id] })
    expect(deep[0]).toMatch(/^poros:\/\/add\//)
    for (const link of [links[0]!, deep[0]!]) {
      const again = await core.call(MethodServersAdd, { groupId: added.group.id, newGroup: '', text: link })
      const { server } = await core.call(MethodServersGet, { id: again.ids[0]! })
      expect(server).toEqual(expect.objectContaining({ name: 'Home', host: 'tr.example.com', port: 443 }))
    }
    const exported = await core.call(MethodServersExport, { id })
    expect(exported.core).toBe(CoreSingBox)
    expect(JSON.parse(exported.json)).toEqual(expect.objectContaining({ type: 'trojan', server: 'tr.example.com' }))
    expect(await reasonOf(core.call(MethodServersLinks, { ids: ['missing'] }))).toBe(ReasonNotFound)
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
    const [server] = await core.call(MethodServersList, { groupId: '' })
    let settings = await core.call(MethodSettingsGet)
    expect(settings.killSwitch).toBe(KillSwitchOff)
    expect(await reasonOf(core.call(MethodSettingsSet, { ...settings, killSwitch: 'on' }))).toBe(ReasonInvalidValue)

    // Connected with System: no rights needed, the kill switch guards the connection, and Stop lets it go.
    settings = await core.call(MethodSettingsSet, { ...settings, server: server!.id, systemProxy: true, killSwitch: KillSwitchConnected })
    await core.call(MethodConnectionStart)
    await settle()
    expect((await core.call(MethodConnectionState)).error).toBe('')
    expect((await core.call(MethodKillSwitchState)).on).toBe(true)
    await core.call(MethodConnectionStop)
    await settle()
    expect(await core.call(MethodKillSwitchState)).toEqual({ on: false, full: false, error: '', errorText: '' })

    // Always: the refusal runs at once, and Stop keeps it.
    await core.call(MethodSettingsSet, { ...settings, killSwitch: KillSwitchAlways })
    await settle()
    expect((await core.call(MethodKillSwitchState)).on).toBe(true)
    await core.call(MethodConnectionStop)
    await settle()
    expect((await core.call(MethodKillSwitchState)).on).toBe(true)
    await core.call(MethodSettingsSet, { ...settings, killSwitch: KillSwitchOff })
    await settle()
    expect(blocks).toHaveBeenLastCalledWith({ on: false, full: false, error: '', errorText: '' })

    // Always with TUN asks for rights: without them the refusal runs without its TUN.
    settings = await core.call(MethodSettingsSet, { ...settings, tun: true, killSwitch: KillSwitchAlways })
    await settle()
    expect(await core.call(MethodKillSwitchState)).toEqual({ on: true, full: false, error: ReasonKillSwitchNeedsElevation, errorText: expect.any(String) })
    await core.elevate()
    await settle()
    expect(await core.call(MethodKillSwitchState)).toEqual({ on: true, full: false, error: '', errorText: '' })
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
    const [server] = await core.call(MethodServersList, { groupId: '' })
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
    await core.call(MethodServersTest, { groupId: '' })
    await new Promise((resolve) => setTimeout(resolve, 0))
    expect(tested).toHaveBeenLastCalledWith(expect.objectContaining({ done: true }))
    const latencies = (await core.call(MethodServersList, { groupId: '' })).map((s) => s.latency)
    expect(latencies).toEqual([42, 48, -1])
  })

  it('keeps preset copies by the rules of the engine', async () => {
    const core = createStubCore()
    const shown = await core.call(MethodPresetsGet, { id: 'all' })
    expect(shown).toMatchObject({ id: 'all', builtin: true, rules: [], inherited: [{ action: 'dns', first: true }, { action: 'direct' }] })
    expect(await reasonOf(core.call(MethodPresetsSave, { ...shown, name: 'Mine' }))).toBe(ReasonInvalidValue)
    const draft = { ...shown, id: '', builtin: false, basedOn: 'all', name: 'Home' }
    const bad = await core.call(MethodPresetsSave, { ...draft, rules: [rule('ip', '10.8.0.0/33', 'direct')] }).catch((error: unknown) => error)
    expect(isCoreError(bad) && bad.data).toMatchObject({ reason: ReasonInvalidEntry, rule: 0, kind: 'ip', line: 1, value: '10.8.0.0/33' })
    expect(await reasonOf(core.call(MethodPresetsSave, { ...draft, rules: [rule('port', '443', 'direct')] }))).toBe(ReasonAdvancedOnly)
    const saved = await core.call(MethodPresetsSave, {
      ...draft,
      rules: [rule('suffix', 'example.org', 'block'), rule('suffix', 'https://Ads.Example.org/x', 'block'), rule('processName', 'steam', 'direct')],
    })
    expect(saved).toMatchObject({ builtin: false, basedOn: 'all', advanced: false, rules: 3 })
    // As in the engine: values as the core reads them, and a narrower rule over a wider one.
    const stored = await core.call(MethodPresetsGet, { id: saved.id })
    expect(stored.rules.map((r) => r.conditions[0]!.values[0])).toEqual(['ads.example.org', 'example.org', 'steam.exe'])

    const settings = await core.call(MethodSettingsGet)
    await core.call(MethodSettingsSet, { ...settings, preset: saved.id })
    const { link } = await core.call(MethodPresetsExport, { id: saved.id })
    const imported = await core.call(MethodPresetsImport, { link })
    // As in the engine, a name is taken once: the imported copy gets a number, a second save is refused.
    expect(imported).toMatchObject({ name: 'Home 2', rules: 3 })
    expect(await reasonOf(core.call(MethodPresetsSave, { ...draft, name: ' home ' }))).toBe(ReasonPresetNameTaken)
    expect(await reasonOf(core.call(MethodPresetsSave, { ...draft, name: 'All traffic' }))).toBe(ReasonPresetNameTaken)

    // An advanced preset never becomes simple again.
    const advanced = await core.call(MethodPresetsSave, { ...stored, advanced: true })
    expect(advanced.advanced).toBe(true)
    expect(await reasonOf(core.call(MethodPresetsSave, { ...stored, advanced: false }))).toBe(ReasonInvalidValue)

    await core.call(MethodPresetsRemove, { id: saved.id })
    expect((await core.call(MethodSettingsGet)).preset).toBe('all')
    expect((await core.call(MethodPresetsList)).map((p) => p.id)).toEqual(['all', 'hardened', imported.id])
  })

  it('makes a preset from scratch that has no base and falls back to the standard one', async () => {
    const core = createStubCore()
    const saved = await core.call(MethodPresetsSave, { ...emptyCopy, name: 'Work' })
    expect(saved).toMatchObject({ builtin: false, basedOn: '' })
    const settings = await core.call(MethodSettingsGet)
    await core.call(MethodSettingsSet, { ...settings, preset: saved.id })
    await core.call(MethodPresetsRemove, { id: saved.id })
    expect((await core.call(MethodSettingsGet)).preset).toBe('all')
  })

  it('checks rules and reads the lists window as the engine does', async () => {
    const core = createStubCore()
    const checked = await core.call(MethodPresetsCheck, {
      advanced: true,
      rules: [rule('suffix', 'example.com', 'proxy'), rule('ip', '10.0.0.0/8', 'direct'), rule('ip', '10.1.2.3', 'block'), rule('port', '8000:9000', 'direct')],
    })
    expect(checked.rules.map((r) => r.conditions[0]!.values[0])).toEqual(['example.com', '10.1.2.3', '10.0.0.0/8', '8000-9000'])
    const repeated = await core.call(MethodPresetsCheck, { advanced: false, rules: [rule('suffix', 'example.com', 'proxy'), rule('suffix', 'Example.com', 'block')] }).catch((error: unknown) => error)
    expect(isCoreError(repeated) && repeated.data).toMatchObject({ reason: ReasonDuplicate, rule: 1, other: 0 })

    const read = await core.call(MethodPresetsReadLists, {
      advanced: false,
      rules: [],
      lists: {
        proxy: 'suffix:video.example.com\r\n# work\n\nsuffix:https://Mail.example.org/inbox\nmedia.example.net\ndomain:exact.example',
        direct: 'ip:192.168.50.0/24\nprocessName:steam\nprocessPath:C:\\Games\\game.exe\nsuffix:*.video.example.com',
        block: 'suffix:*.ads.example\ngeosite:category-ads\nsuffix:*.\nprocessName:STEAM.EXE',
      },
    })
    expect(read.rules.map((r) => `${r.action} ${r.conditions[0]!.kind}:${r.conditions[0]!.values[0]}`)).toEqual([
      'proxy suffix:video.example.com',
      'proxy suffix:mail.example.org',
      'direct ip:192.168.50.0/24',
      'direct processName:steam.exe',
      'direct processPath:C:\\Games\\game.exe',
      'block suffix:ads.example',
    ])
    expect(read.problems).toEqual([
      { action: 'proxy', line: 5, text: 'media.example.net', reason: ReasonNoPrefix, other: '' },
      { action: 'proxy', line: 6, text: 'domain:exact.example', reason: ReasonAdvancedOnly, other: '' },
      { action: 'direct', line: 4, text: 'suffix:*.video.example.com', reason: ReasonDuplicate, other: 'proxy' },
      { action: 'block', line: 2, text: 'geosite:category-ads', reason: ReasonNoPrefix, other: '' },
      { action: 'block', line: 3, text: 'suffix:*.', reason: ReasonInvalidEntry, other: '' },
      { action: 'block', line: 4, text: 'processName:STEAM.EXE', reason: ReasonDuplicate, other: 'direct' },
    ])
    expect(await reasonOf(core.call(MethodPresetsReadLists, { advanced: false, rules: [], lists: { warp: '' } }))).toBe(ReasonInvalidValue)

    // A rule with a line keeps its place; one of several conditions stays out of the window and in place.
    const games = { ...rule('processName', 'game.exe', 'direct'), name: 'Games' }
    games.conditions.push({ kind: 'network', values: ['udp'] })
    const rules = [rule('suffix', 'example.org', 'block'), games, rule('ip', '10.0.0.0/8', 'direct')]
    const lists = await core.call(MethodPresetsWriteLists, { advanced: true, rules })
    expect(lists.lists).toEqual({ proxy: '', direct: 'ip:10.0.0.0/8', block: 'suffix:example.org' })
    const back = await core.call(MethodPresetsReadLists, { advanced: true, rules, lists: { ...lists.lists, proxy: 'suffix:x.example.org' } })
    expect(back.rules.map((r) => r.conditions[0]!.values[0])).toEqual(['x.example.org', 'example.org', 'game.exe', '10.0.0.0/8'])
  })
  it('keeps the endpoints of an advanced preset as the engine does', async () => {
    const core = createStubCore()
    const encode = (text: string) => btoa(text).replaceAll('+', '-').replaceAll('/', '_').replace(/=+$/, '')
    const office = { type: 'openvpn', name: 'Office', server: 'vpn.example.com', server_port: 1194, openvpn: { username: 'u', password: 'pw' } }
    const added = await core.call(MethodServersAdd, {
      groupId: '',
      newGroup: 'Office',
      text: `poros://add/${encode(JSON.stringify(office))}
trojan://p@de.example.com:443#Germany`,
    })
    const [vpn, trojan] = added.ids as [string, string]
    const wiki = { ...rule('suffix', 'wiki.example', 'endpoint'), endpoint: vpn }
    const draft = { ...emptyCopy, basedOn: 'all', name: 'Office', advanced: true, endpoints: [vpn], rules: [wiki, rule('suffix', 'example.org', 'direct')] }
    const saved = await core.call(MethodPresetsSave, draft)
    expect(saved).toMatchObject({ advanced: true, rules: 2, endpoints: 1 })
    expect(await reasonOf(core.call(MethodPresetsSave, { ...draft, advanced: false, rules: [] }))).toBe(ReasonAdvancedOnly)
    expect(await reasonOf(core.call(MethodPresetsSave, { ...draft, name: 'No VPN', endpoints: [trojan], rules: [] }))).toBe(ReasonInvalidValue)
    expect(await reasonOf(core.call(MethodPresetsSave, { ...draft, name: 'Gone', endpoints: ['gone'], rules: [] }))).toBe(ReasonNotFound)
    const stray = await core.call(MethodPresetsCheck, { advanced: true, rules: [wiki] }).catch((error: unknown) => error)
    expect(isCoreError(stray) && stray.data).toMatchObject({ reason: ReasonInvalidValue, rule: 0 })

    // A link leaves the endpoints out with the rules to them; the lists window has no line for those rules.
    const { link } = await core.call(MethodPresetsExport, { id: saved.id })
    const imported = await core.call(MethodPresetsImport, { link })
    expect(await core.call(MethodPresetsGet, { id: imported.id })).toMatchObject({ endpoints: [], rules: [{ conditions: [{ values: ['example.org'] }] }] })
    const stored = await core.call(MethodPresetsGet, { id: saved.id })
    expect((await core.call(MethodPresetsWriteLists, { advanced: true, rules: stored.rules })).lists).toEqual({ proxy: '', direct: 'suffix:example.org', block: '' })
    const read = await core.call(MethodPresetsReadLists, { advanced: true, rules: stored.rules, lists: { proxy: 'suffix:Wiki.example', direct: 'suffix:example.org' } })
    expect(read.problems).toEqual([{ action: 'proxy', line: 1, text: 'suffix:Wiki.example', reason: ReasonDuplicate, other: 'endpoint' }])
    expect(read.rules).toEqual(stored.rules)
  })

  it('checks the DNS settings and an own dns object as the engine does', async () => {
    const core = createStubCore()
    const settings = await core.call(MethodSettingsGet)
    expect(settings).toMatchObject({ dnsRouting: 'auto', dnsHosts: true, dnsCache: true, dnsCacheSize: 4096, dnsTimeout: 10, fakeIp: false })
    expect(await reasonOf(core.call(MethodSettingsSet, { ...settings, dnsRemote: 'dns.example' }))).toBe(ReasonInvalidValue)
    expect(await reasonOf(core.call(MethodSettingsSet, { ...settings, dnsAnswers: 'nas.home' }))).toBe(ReasonInvalidValue)
    expect(await reasonOf(core.call(MethodSettingsSet, { ...settings, dnsIgnoreTtl: true, dnsOptimistic: true }))).toBe(ReasonInvalidValue)
    const saved = await core.call(MethodSettingsSet, { ...settings, dnsRemote: 'tls://1.1.1.1', dnsAnswers: 'nas.home 192.168.1.20', dnsRouting: 'proxy' })
    expect(saved).toMatchObject({ dnsRemote: 'tls://1.1.1.1', dnsRouting: 'proxy' })

    const object = (detour: string) => JSON.stringify({ servers: [{ type: 'https', tag: 'remote', server: '1.1.1.1', detour }], final: 'remote' })
    expect(await core.call(MethodDNSCheck, { object: object('proxy') })).toBeNull()
    const wrong = await core.call(MethodDNSCheck, { object: object('proxi') }).catch((error: unknown) => error)
    expect(isCoreError(wrong) && wrong.message).toContain('unknown outbound "proxi"')
    expect(await reasonOf(core.call(MethodDNSCheck, { object: '{"final":"remote"}' }))).toBe(ReasonInvalidValue)
  })

  it('reads poros:// links as the engine does and adds nothing', async () => {
    const core = createStubCore()
    const encode = (text: string) => btoa(text).replaceAll('+', '-').replaceAll('/', '_').replace(/=+$/, '')
    expect(await core.call(MethodLinksParse, { link: `poros://AddSub/${encode('https://sub.example.com/u/8f3a#My%20VPN')}` })).toEqual({
      kind: LinkSubscription,
      url: 'https://sub.example.com/u/8f3a',
      name: 'My VPN',
    })

    const saved = await core.call(MethodPresetsSave, { ...emptyCopy, basedOn: 'all', name: 'Home', dns: 'tls://1.1.1.1' })
    const { link } = await core.call(MethodPresetsExport, { id: saved.id })
    const info = await core.call(MethodLinksParse, { link })
    expect(info).toMatchObject({ kind: LinkPreset, preset: { name: 'Home', basedOn: 'all', dns: 'tls://1.1.1.1' } })
    expect(await core.call(MethodPresetsList)).toHaveLength(3)

    expect(await reasonOf(core.call(MethodLinksParse, { link: `poros://add/${encode('{}')}` }))).toBe(ReasonInvalidLink)
    expect(await reasonOf(core.call(MethodLinksParse, { link: `throne://add/${encode('{}')}` }))).toBe(ReasonInvalidLink)
    expect(await reasonOf(core.call(MethodLinksParse, { link: `throne://route/${encode('{}')}` }))).toBe(ReasonInvalidLink)
    expect(await reasonOf(core.call(MethodLinksParse, { link: 'https://sub.example.com/u/8f3a' }))).toBe(ReasonInvalidLink)
  })
})
