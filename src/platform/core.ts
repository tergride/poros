import { invoke } from '@tauri-apps/api/core'
import { listen } from '@tauri-apps/api/event'

import type {
  AddSubscription,
  AppInfo,
  ByID,
  ConnectionState,
  KillSwitchState,
  LinkInfo,
  NetworkState,
  OutsideLink,
  Preset,
  PresetCopy,
  PresetEntries,
  PresetLink,
  PresetLists,
  PresetListsRead,
  RenameSubscription,
  Server,
  ServersChanged,
  ServersQuery,
  ServersTested,
  Settings,
  Subscription,
  SubscriptionUpdate,
  Traffic,
} from './core-types'
import {
  EventConnectionChanged,
  EventConnectionTraffic,
  EventKillSwitchChanged,
  EventNetworkChanged,
  EventPresetsChanged,
  EventServersChanged,
  EventServersTested,
  EventSettingsChanged,
  EventSubscriptionsChanged,
  MethodAppInfo,
  MethodConnectionStart,
  MethodConnectionState,
  MethodConnectionStop,
  MethodKillSwitchState,
  MethodLinksParse,
  MethodLogsGet,
  MethodNetworkCheck,
  MethodNetworkState,
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
  MethodSubscriptionsRename,
  MethodSubscriptionsUpdate,
} from './core-types'

/** What each method of core/engine/api.go takes and returns. */
export interface CoreMethods {
  [MethodAppInfo]: { params: undefined; result: AppInfo }
  [MethodSettingsGet]: { params: undefined; result: Settings }
  [MethodSettingsSet]: { params: Settings; result: Settings }
  [MethodSubscriptionsList]: { params: undefined; result: Subscription[] }
  [MethodSubscriptionsAdd]: { params: AddSubscription; result: SubscriptionUpdate }
  [MethodSubscriptionsUpdate]: { params: ByID; result: SubscriptionUpdate }
  [MethodSubscriptionsRemove]: { params: ByID; result: null }
  [MethodSubscriptionsRename]: { params: RenameSubscription; result: Subscription }
  [MethodServersList]: { params: ServersQuery; result: Server[] }
  [MethodServersTest]: { params: ServersQuery; result: null }
  [MethodPresetsList]: { params: undefined; result: Preset[] }
  [MethodPresetsGet]: { params: ByID; result: PresetCopy }
  [MethodPresetsSave]: { params: PresetCopy; result: Preset }
  [MethodPresetsRemove]: { params: ByID; result: null }
  [MethodPresetsExport]: { params: ByID; result: PresetLink }
  [MethodPresetsImport]: { params: PresetLink; result: Preset }
  [MethodPresetsWriteLists]: { params: PresetEntries; result: PresetLists }
  [MethodPresetsReadLists]: { params: PresetLists; result: PresetListsRead }
  [MethodLinksParse]: { params: OutsideLink; result: LinkInfo }
  [MethodConnectionStart]: { params: undefined; result: ConnectionState }
  [MethodConnectionStop]: { params: undefined; result: ConnectionState }
  [MethodConnectionState]: { params: undefined; result: ConnectionState }
  [MethodNetworkCheck]: { params: undefined; result: NetworkState }
  [MethodNetworkState]: { params: undefined; result: NetworkState }
  [MethodLogsGet]: { params: undefined; result: string[] }
  [MethodKillSwitchState]: { params: undefined; result: KillSwitchState }
}

export interface CoreEvents {
  [EventSettingsChanged]: Settings
  [EventSubscriptionsChanged]: Subscription[]
  [EventServersChanged]: ServersChanged
  [EventServersTested]: ServersTested
  [EventPresetsChanged]: Preset[]
  [EventConnectionChanged]: ConnectionState
  [EventConnectionTraffic]: Traffic
  [EventNetworkChanged]: NetworkState
  [EventKillSwitchChanged]: KillSwitchState
}

export type CoreMethod = keyof CoreMethods
export type CoreEvent = keyof CoreEvents

export interface CoreStatus {
  state: 'starting' | 'ready' | 'down'
  /** The core runs with administrator rights, which TUN needs. */
  elevated: boolean
  error?: string
}

/** A JSON-RPC error from the core; `data.reason` is one of the Reason constants. */
export interface CoreError {
  code: number
  message: string
  data?: { reason?: string }
}

/** The shell's codes for a replaced core: the user declined the UAC prompt; the core could not start. */
export const CodeElevationCancelled = -33002
export const CodeStartFailed = -33003

type ParamsArg<M extends CoreMethod> = CoreMethods[M]['params'] extends undefined ? [] : [CoreMethods[M]['params']]

export interface CoreClient {
  call<M extends CoreMethod>(method: M, ...params: ParamsArg<M>): Promise<CoreMethods[M]['result']>
  on<E extends CoreEvent>(event: E, handler: (params: CoreEvents[E]) => void): () => void
  status(): Promise<CoreStatus>
  onStatus(handler: (status: CoreStatus) => void): () => void
  /** Replaces the core with one that has administrator rights, after the UAC prompt. */
  elevate(): Promise<void>
  /** Replaces the core with one without rights; it also brings back a core that gave up. */
  restart(): Promise<void>
}

export function isCoreError(value: unknown): value is CoreError {
  return typeof value === 'object' && value !== null && 'code' in value && 'message' in value
}

/** The client of the real core: calls and events go through the Tauri shell. */
export async function createTauriCore(): Promise<CoreClient> {
  const handlers = new Map<string, Set<(params: never) => void>>()
  const statusHandlers = new Set<(status: CoreStatus) => void>()

  await listen<{ method: string; params: never }>('core-event', ({ payload }) => {
    handlers.get(payload.method)?.forEach((handler) => handler(payload.params))
  })
  await listen<CoreStatus>('core-status', ({ payload }) => statusHandlers.forEach((handler) => handler(payload)))

  return {
    call: (method, ...params) => invoke('core_call', { method, params: params[0] ?? null }),
    on(event, handler) {
      const set = handlers.get(event) ?? new Set()
      set.add(handler as (params: never) => void)
      handlers.set(event, set)
      return () => set.delete(handler as (params: never) => void)
    },
    status: () => invoke<CoreStatus>('core_status'),
    onStatus(handler) {
      statusHandlers.add(handler)
      return () => statusHandlers.delete(handler)
    },
    elevate: () => invoke('core_elevate'),
    restart: () => invoke('core_restart'),
  }
}
