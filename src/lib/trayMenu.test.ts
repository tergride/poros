import { describe, expect, it, vi } from 'vitest'

import type { ConnectionState, Preset, Server, Settings } from '@/platform/core-types'

import { type TrayActions, type TrayItem, type TrayState, type TrayWords, trayMenu } from './trayMenu'

const preset = (id: string, builtin: boolean): Preset => ({ id, name: id, builtin }) as Preset

const words: TrayWords = {
  connected: (s) => `on ${s}`,
  connecting: () => 'connecting',
  disconnected: () => 'off',
  connect: () => 'Connect',
  disconnect: () => 'Disconnect',
  route: (p) => `Route: ${p}`,
  presets: () => 'Presets…',
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
})
