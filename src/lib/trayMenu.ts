import { serverTitle } from '@/lib/format'
import type { ConnectionState, Preset, Server, Settings } from '@/platform/core-types'
import {
  ConnectionIdle,
  ConnectionRunning,
  ConnectionStarting,
  ConnectionStopping,
  KillSwitchConnected,
} from '@/platform/core-types'

/** The tray menu as plain data; the platform turns it into the system menu. */
export type TrayItem =
  | { kind: 'item'; text: string; enabled?: boolean; action?: () => void }
  | { kind: 'check'; text: string; checked: boolean; action: () => void }
  | { kind: 'submenu'; text: string; items: TrayItem[] }
  | { kind: 'separator' }

export interface TrayState {
  connection: ConnectionState
  settings: Settings
  servers: Server[]
  presets: Preset[]
  /** The kill switch blocks: after a lost connection until Stop, or in Always without one. */
  blocked?: boolean
  /** The core does not answer: nothing that needs it can be done. */
  coreDown?: boolean
}

export interface TrayActions {
  connect(): void
  disconnect(): void
  choosePreset(id: string): void
  showPresets(): void
  setTun(on: boolean): void
  setSystemProxy(on: boolean): void
  show(): void
  exit(): void
}

export interface TrayWords {
  connected(server: string): string
  connecting(): string
  disconnected(): string
  blocked(): string
  blockedAlways(): string
  coreDown(): string
  starting(): string
  connect(): string
  disconnect(): string
  stop(): string
  route(preset: string): string
  presets(): string
  show(): string
  exit(): string
  presetName(preset: Preset): string
}

const separator: TrayItem = { kind: 'separator' }

/** The kill switch holds the block after a lost connection: only Stop gives the internet back. */
function held(state: TrayState): boolean {
  return !!state.blocked && state.connection.state === ConnectionIdle && state.settings.killSwitch === KillSwitchConnected
}

export function statusText(state: TrayState, words: TrayWords): string {
  const { connection } = state
  if (state.coreDown) return words.coreDown()
  if (connection.state === ConnectionStarting || connection.state === ConnectionStopping) return words.connecting()
  if (held(state)) return words.blocked()
  if (connection.state !== ConnectionRunning) return state.blocked ? words.blockedAlways() : words.disconnected()
  const server = state.servers.find((s) => s.id === connection.serverId)
  return words.connected(server ? serverTitle(server.name, server.country) : '')
}

/** The menu while the core has not given its data, or never will: the window and the way out stay. */
export function trayFallback(down: boolean, actions: Pick<TrayActions, 'show' | 'exit'>, words: TrayWords): TrayItem[] {
  return [
    { kind: 'item', text: down ? words.coreDown() : words.starting(), enabled: false },
    separator,
    { kind: 'item', text: words.show(), action: actions.show },
    { kind: 'item', text: words.exit(), action: actions.exit },
  ]
}

/**
 * The menu of the mockup: the state, Start or Stop, the route, TUN and System. Servers are chosen in the window: a
 * subscription may hold hundreds of them, too many for a menu.
 */
export function trayMenu(state: TrayState, actions: TrayActions, words: TrayWords): TrayItem[] {
  const { connection, settings } = state
  const active = connection.state !== ConnectionIdle

  const builtin = state.presets.filter((p) => p.builtin)
  const mine = state.presets.filter((p) => !p.builtin)
  const presetItem = (p: Preset): TrayItem => ({
    kind: 'check',
    text: words.presetName(p),
    checked: p.id === settings.preset,
    action: () => actions.choosePreset(p.id),
  })
  const presetItems: TrayItem[] = [
    ...builtin.map(presetItem),
    ...(mine.length ? [separator, ...mine.map(presetItem)] : []),
    separator,
    { kind: 'item', text: words.presets(), action: actions.showPresets },
  ]
  const current = state.presets.find((p) => p.id === settings.preset)

  const main: TrayItem = held(state)
    ? { kind: 'item', text: words.stop(), enabled: !state.coreDown, action: actions.disconnect }
    : active
      ? { kind: 'item', text: words.disconnect(), enabled: !state.coreDown, action: actions.disconnect }
      : { kind: 'item', text: words.connect(), enabled: Boolean(settings.server) && !state.coreDown, action: actions.connect }

  return [
    { kind: 'item', text: statusText(state, words), enabled: false },
    main,
    separator,
    { kind: 'submenu', text: words.route(current ? words.presetName(current) : ''), items: presetItems },
    separator,
    { kind: 'check', text: 'TUN', checked: settings.tun, action: () => actions.setTun(!settings.tun) },
    { kind: 'check', text: 'System', checked: settings.systemProxy, action: () => actions.setSystemProxy(!settings.systemProxy) },
    separator,
    { kind: 'item', text: words.show(), action: actions.show },
    { kind: 'item', text: words.exit(), action: actions.exit },
  ]
}
