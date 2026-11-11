import { describe, expect, it, vi } from 'vitest'

import type { ConnectionState, OTPCode, Preset, Server, Settings } from '@/platform/core-types'

import { type TrayActions, type TrayItem, type TrayState, type TrayWords, trayFallback, trayMenu } from './trayMenu'

const preset = (id: string, builtin: boolean): Preset => ({ id, name: id, builtin }) as Preset

const words: TrayWords = {
  connected: (s) => `on ${s}`,
  connecting: () => 'connecting',
  disconnected: () => 'off',
  blocked: (full) => (full ? 'all closed until Stop' : 'blocked until Stop'),
  blockedAlways: () => 'blocked',
  coreDown: () => 'core down',
  starting: () => 'starting',
  connect: () => 'Connect',
  disconnect: () => 'Disconnect',
  stop: () => 'Stop',
  route: (p) => `Route: ${p}`,
  presets: () => 'Presets…',
  otp: () => 'Codes',
  otpHint: () => 'A click copies',
  otpName: (c) => c.name,
  show: () => 'Show',
  exit: () => 'Exit',
  presetName: (p) => p.name,
}

function state(overrides: Partial<Settings> = {}, connection: Partial<ConnectionState> = {}): TrayState {
  return {
    connection: { state: 'idle', serverId: '', tun: false, systemProxy: false, since: 0, error: '', errorText: '', ...connection },
    settings: { server: 's2', preset: 'home', tun: true, systemProxy: false, ...overrides } as Settings,
    servers: [{ id: 's2', name: '🇩🇪 Frankfurt', country: 'DE' } as Server],
    presets: [preset('all', true), preset('hardened', true), preset('home', false)],
  }
}

function actions(): TrayActions {
  return {
    connect: vi.fn(),
    disconnect: vi.fn(),
    choosePreset: vi.fn(),
    showPresets: vi.fn(),
    copyOtp: vi.fn(),
    setTun: vi.fn(),
    setSystemProxy: vi.fn(),
    show: vi.fn(),
    exit: vi.fn(),
  }
}

const texts = (items: TrayItem[]) => items.map((item) => (item.kind === 'separator' ? '—' : item.text))

describe('the tray menu', () => {
  it('follows the mockup without a server list: state, start, route, modes, window', () => {
    const items = trayMenu(state(), actions(), words)
    expect(texts(items)).toEqual(['off', 'Connect', '—', 'Route: home', '—', 'TUN', 'System', '—', 'Show', 'Exit'])
    const route = items[3]
    if (route?.kind !== 'submenu') throw new Error('no route submenu')
    expect(texts(route.items)).toEqual(['all', 'hardened', '—', 'home', '—', 'Presets…'])
  })

  it('lists the codes of OTP while there are any, and a click copies one', () => {
    const codes = [{ id: 'o1', name: 'GitHub' }, { id: 'o2', name: 'Office' }] as OTPCode[]
    const act = actions()
    const items = trayMenu({ ...state(), otp: codes }, act, words)
    expect(texts(items)).toEqual(['off', 'Connect', '—', 'Route: home', 'Codes', '—', 'TUN', 'System', '—', 'Show', 'Exit'])
    const menu = items[4]
    if (menu?.kind !== 'submenu') throw new Error('no codes submenu')
    expect(texts(menu.items)).toEqual(['GitHub', 'Office', '—', 'A click copies'])
    const office = menu.items[1]
    if (office?.kind !== 'item') throw new Error('no item')
    office.action?.()
    expect(act.copyOtp).toHaveBeenCalledWith('o2')
    expect(menu.items[3]).toMatchObject({ enabled: false })
  })

  it('marks what is chosen and acts on it', () => {
    const act = actions()
    const items = trayMenu(state(), act, words)
    const tun = items.find((item) => item.kind === 'check' && item.text === 'TUN')
    if (tun?.kind !== 'check') throw new Error('no TUN item')
    expect(tun.checked).toBe(true)
    tun.action()
    expect(act.setTun).toHaveBeenCalledWith(false)
  })

  it('names the server while connected, offers Disconnect, and no Connect without a server', () => {
    const running = trayMenu(state({}, { state: 'running', serverId: 's2' }), actions(), words)
    expect(texts(running).slice(0, 2)).toEqual(['on Frankfurt', 'Disconnect'])
    const empty = trayMenu(state({ server: '' }), actions(), words)
    expect(empty[1]).toMatchObject({ text: 'Connect', enabled: false })
  })

  it('offers Stop while the kill switch holds the block after a lost connection', () => {
    const act = actions()
    const held = trayMenu({ ...state({ killSwitch: 'connected' }), blocked: true }, act, words)
    expect(texts(held).slice(0, 2)).toEqual(['blocked until Stop', 'Stop'])
    const stop = held[1]
    if (stop?.kind !== 'item') throw new Error('no Stop')
    stop.action?.()
    expect(act.disconnect).toHaveBeenCalled()
    // In Always the block stays without a connection, and Connect is what lifts it.
    const always = trayMenu({ ...state({ killSwitch: 'always' }), blocked: true }, actions(), words)
    expect(texts(always).slice(0, 2)).toEqual(['blocked', 'Connect'])
    // Without Poros's TUN Windows blocks everything, and the tray says so.
    const full = trayMenu({ ...state({ killSwitch: 'connected' }), blocked: true, full: true }, actions(), words)
    expect(texts(full)[0]).toBe('all closed until Stop')
  })

  it('says the core is down and offers nothing that needs it', () => {
    const items = trayMenu({ ...state(), coreDown: true }, actions(), words)
    expect(items[0]).toMatchObject({ text: 'core down' })
    expect(items[1]).toMatchObject({ text: 'Connect', enabled: false })
  })

  it('keeps the window and the way out before the core gives its data, or when it never does', () => {
    const act = actions()
    expect(texts(trayFallback(false, act, words))).toEqual(['starting', '—', 'Show', 'Exit'])
    const down = trayFallback(true, act, words)
    expect(texts(down)[0]).toBe('core down')
    const exit = down[3]
    if (exit?.kind !== 'item') throw new Error('no Exit')
    exit.action?.()
    expect(act.exit).toHaveBeenCalled()
  })
})
