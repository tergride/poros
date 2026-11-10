import { invoke } from '@tauri-apps/api/core'
import { listen } from '@tauri-apps/api/event'

import type {
  AddGroup,
  AddServers,
  AddSubscription,
  AppInfo,
  ByID,
  CleanServers,
  ConfigureGroup,
  ConnectionState,
  DNSObject,
  Group,
  KillSwitchState,
  LinkInfo,
  NetworkState,
  OutsideLink,
  Preset,
  PresetCopy,
  PresetLink,
  PresetLists,
  PresetListsInput,
  PresetListsRead,
  PresetRules,
  RuleProblem,
  MoveServers,
  RenameGroup,
  SaveServer,
  Server,
  ServerCheck,
  ServerConfig,
  ServerForms,
  ServerIDs,
  ServerLinks,
  ServerProfile,
  ServersAdded,
  ServersChanged,
  ServersQuery,
  ServersIPTested,
  ServersResolved,
  ServersSpeedTested,
  ServersTested,
  Settings,
  SubscriptionUpdate,
  Traffic,
  UpdateInstall,
  UpdateState,
  WARPRegister,
} from './core-types'
import {
  EventConnectionChanged,
  EventConnectionTraffic,
  EventKillSwitchChanged,
  EventNetworkChanged,
  EventPresetsChanged,
  EventServersChanged,
  EventServersIPTested,
  EventServersSpeedTested,
  EventServersTested,
  EventGroupsChanged,
  EventSettingsChanged,
  EventUpdatesChanged,
  MethodAppInfo,
  MethodConnectionStart,
  MethodConnectionState,
  MethodConnectionStop,
  MethodGroupsAdd,
  MethodGroupsConfigure,
  MethodGroupsList,
  MethodGroupsRemove,
  MethodGroupsRename,
  MethodKillSwitchState,
  MethodLinksParse,
  MethodLogsGet,
  MethodNetworkCheck,
  MethodNetworkState,
  MethodDNSCheck,
  MethodPresetsCheck,
  MethodWARPRegister,
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
  MethodServersCheck,
  MethodServersExport,
  MethodServersForms,
  MethodServersGet,
  MethodServersLinks,
  MethodServersList,
  MethodServersMove,
  MethodServersRemove,
  MethodServersSave,
  MethodServersClean,
  MethodServersClearResults,
  MethodServersIPTest,
  MethodServersResetTraffic,
  MethodServersResolve,
  MethodServersSpeedTest,
  MethodServersStopTests,
  MethodServersTest,
  MethodSettingsGet,
  MethodSettingsSet,
  MethodSubscriptionsAdd,
  MethodSubscriptionsUpdate,
  MethodUpdatesCheck,
  MethodUpdatesInstall,
  MethodUpdatesState,
} from './core-types'

/** What each method of core/engine/api.go takes and returns. */
export interface CoreMethods {
  [MethodAppInfo]: { params: undefined; result: AppInfo }
  [MethodSettingsGet]: { params: undefined; result: Settings }
  [MethodSettingsSet]: { params: Settings; result: Settings }
  [MethodGroupsList]: { params: undefined; result: Group[] }
  [MethodGroupsAdd]: { params: AddGroup; result: Group }
  [MethodGroupsConfigure]: { params: ConfigureGroup; result: Group }
  [MethodGroupsRename]: { params: RenameGroup; result: Group }
  [MethodGroupsRemove]: { params: ByID; result: null }
  [MethodSubscriptionsAdd]: { params: AddSubscription; result: SubscriptionUpdate }
  [MethodSubscriptionsUpdate]: { params: ByID; result: SubscriptionUpdate }
  [MethodServersList]: { params: ServersQuery; result: Server[] }
  [MethodServersAdd]: { params: AddServers; result: ServersAdded }
  [MethodServersGet]: { params: ByID; result: ServerProfile }
  [MethodServersSave]: { params: SaveServer; result: Server }
  [MethodServersRemove]: { params: ServerIDs; result: null }
  [MethodServersClone]: { params: ServerIDs; result: ServerIDs }
  [MethodServersMove]: { params: MoveServers; result: ServerIDs }
  [MethodServersLinks]: { params: ServerIDs; result: ServerLinks }
  [MethodServersDeepLinks]: { params: ServerIDs; result: ServerLinks }
  [MethodServersExport]: { params: ByID; result: ServerConfig }
  [MethodServersForms]: { params: undefined; result: ServerForms }
  [MethodServersCheck]: { params: SaveServer; result: ServerCheck }
  [MethodServersTest]: { params: ServersQuery; result: null }
  [MethodServersSpeedTest]: { params: ServersQuery; result: null }
  [MethodServersIPTest]: { params: ServersQuery; result: null }
  [MethodServersResolve]: { params: ServersQuery; result: ServersResolved }
  [MethodServersClearResults]: { params: ServersQuery; result: null }
  [MethodServersStopTests]: { params: undefined; result: null }
  [MethodServersResetTraffic]: { params: ServersQuery; result: null }
  [MethodServersClean]: { params: CleanServers; result: ServerIDs }
  [MethodPresetsList]: { params: undefined; result: Preset[] }
  [MethodPresetsGet]: { params: ByID; result: PresetCopy }
  [MethodPresetsSave]: { params: PresetCopy; result: Preset }
  [MethodPresetsRemove]: { params: ByID; result: null }
  [MethodPresetsExport]: { params: ByID; result: PresetLink }
  [MethodPresetsImport]: { params: PresetLink; result: Preset }
  [MethodPresetsCheck]: { params: PresetRules; result: PresetRules }
  [MethodDNSCheck]: { params: DNSObject; result: null }
  [MethodWARPRegister]: { params: WARPRegister; result: Settings }
  [MethodPresetsWriteLists]: { params: PresetRules; result: PresetLists }
  [MethodPresetsReadLists]: { params: PresetListsInput; result: PresetListsRead }
  [MethodLinksParse]: { params: OutsideLink; result: LinkInfo }
  [MethodConnectionStart]: { params: undefined; result: ConnectionState }
  [MethodConnectionStop]: { params: undefined; result: ConnectionState }
  [MethodConnectionState]: { params: undefined; result: ConnectionState }
  [MethodNetworkCheck]: { params: undefined; result: NetworkState }
  [MethodNetworkState]: { params: undefined; result: NetworkState }
  [MethodLogsGet]: { params: undefined; result: string[] }
  [MethodKillSwitchState]: { params: undefined; result: KillSwitchState }
  [MethodUpdatesState]: { params: undefined; result: UpdateState }
  [MethodUpdatesCheck]: { params: undefined; result: UpdateState }
  [MethodUpdatesInstall]: { params: undefined; result: UpdateInstall }
}

export interface CoreEvents {
  [EventSettingsChanged]: Settings
  [EventGroupsChanged]: Group[]
  [EventServersChanged]: ServersChanged
  [EventServersTested]: ServersTested
  [EventServersSpeedTested]: ServersSpeedTested
  [EventServersIPTested]: ServersIPTested
  [EventPresetsChanged]: Preset[]
  [EventConnectionChanged]: ConnectionState
  [EventConnectionTraffic]: Traffic
  [EventNetworkChanged]: NetworkState
  [EventKillSwitchChanged]: KillSwitchState
  [EventUpdatesChanged]: UpdateState
}

export type CoreMethod = keyof CoreMethods
export type CoreEvent = keyof CoreEvents

export interface CoreStatus {
  state: 'starting' | 'ready' | 'down'
  /** The core runs with administrator rights, which TUN needs. */
  elevated: boolean
  error?: string
  /**
   * data_damaged: the core cannot read its data file and waits for the user; not_answering: a ready core stopped
   * answering, and the shell starts it again if it stays so.
   */
  reason?: typeof StatusDataDamaged | typeof StatusNotAnswering
}

export const StatusDataDamaged = 'data_damaged'
export const StatusNotAnswering = 'not_answering'

/** A JSON-RPC error from the core; `data.reason` is one of the Reason constants. */
export interface CoreError {
  code: number
  message: string
  /** A failure about a rule of a preset carries the whole RuleProblem. */
  data?: { reason?: string } & Partial<RuleProblem>
}

/** The local proxy of the core, HTTP and SOCKS5 on one port: DefaultMixedPort of core/engine/connection.go. */
export const localProxyHost = '127.0.0.1'
export const localProxyPort = 2090

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
  /** Puts a damaged data file aside and starts a core on clean data; the user asked for it. */
  resetData(): Promise<void>
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
    resetData: () => invoke('core_reset_data'),
  }
}
