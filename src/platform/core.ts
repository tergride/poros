import { invoke } from '@tauri-apps/api/core'
import { listen } from '@tauri-apps/api/event'

import type { AddSubscription, AppInfo, ByID, Preset, Settings, Subscription } from './core-types'
import {
  EventSettingsChanged,
  EventSubscriptionsChanged,
  MethodAppInfo,
  MethodPresetsList,
  MethodSettingsGet,
  MethodSettingsSet,
  MethodSubscriptionsAdd,
  MethodSubscriptionsList,
  MethodSubscriptionsRemove,
  ReasonDuplicate,
  ReasonInvalidURL,
  ReasonNotFound,
} from './core-types'

/** What each method of core/engine/api.go takes and returns. */
export interface CoreMethods {
  [MethodAppInfo]: { params: undefined; result: AppInfo }
  [MethodSettingsGet]: { params: undefined; result: Settings }
  [MethodSettingsSet]: { params: Settings; result: Settings }
  [MethodSubscriptionsList]: { params: undefined; result: Subscription[] }
  [MethodSubscriptionsAdd]: { params: AddSubscription; result: Subscription }
  [MethodSubscriptionsRemove]: { params: ByID; result: null }
  [MethodPresetsList]: { params: undefined; result: Preset[] }
}

export interface CoreEvents {
  [EventSettingsChanged]: Settings
  [EventSubscriptionsChanged]: Subscription[]
}

export type CoreMethod = keyof CoreMethods
export type CoreEvent = keyof CoreEvents

export interface CoreStatus {
  state: 'starting' | 'ready' | 'down'
  error?: string
}

/** A JSON-RPC error from the core; `data.reason` is one of the Reason constants. */
export interface CoreError {
  code: number
  message: string
  data?: { reason?: string }
}

type ParamsArg<M extends CoreMethod> = CoreMethods[M]['params'] extends undefined ? [] : [CoreMethods[M]['params']]

export interface CoreClient {
  call<M extends CoreMethod>(method: M, ...params: ParamsArg<M>): Promise<CoreMethods[M]['result']>
  on<E extends CoreEvent>(event: E, handler: (params: CoreEvents[E]) => void): () => void
  status(): Promise<CoreStatus>
  onStatus(handler: (status: CoreStatus) => void): () => void
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
  }
}

const defaultSettings: Settings = {
  uiMode: 'simple',
  language: '',
  onboardingDone: false,
  tun: false,
  systemProxy: false,
  preset: 'all',
  checkIp: true,
  allowLan: false,
}

const builtinPresets: Preset[] = [
  { id: 'all', name: 'All traffic', builtin: true },
  { id: 'except-ru', name: 'Except Russian sites', builtin: true },
  { id: 'blocked-only', name: 'Blocked sites only', builtin: true },
  { id: 'hardened', name: 'Hardened bypass', builtin: true },
]

function failed(reason: string, message: string): CoreError {
  return { code: 1, message, data: { reason } }
}

/**
 * A core in memory for the interface alone: Vite in a browser, Vitest and Playwright.
 * It follows the rules of the Go engine closely enough for screens and tests, not for real traffic.
 */
export function createStubCore(): CoreClient {
  let settings = { ...defaultSettings }
  let subscriptions: Subscription[] = []
  let nextId = 1
  const handlers = new Map<string, Set<(params: never) => void>>()
  const emit = <E extends CoreEvent>(event: E, params: CoreEvents[E]) =>
    handlers.get(event)?.forEach((handler) => handler(params as never))

  const methods: { [M in CoreMethod]: (params: CoreMethods[M]['params']) => CoreMethods[M]['result'] } = {
    [MethodAppInfo]: () => ({ singBox: 'stub', xray: 'stub', dataDir: 'browser' }),
    [MethodSettingsGet]: () => ({ ...settings }),
    [MethodSettingsSet]: (next) => {
      settings = { ...next }
      emit(EventSettingsChanged, { ...settings })
      return { ...settings }
    },
    [MethodSubscriptionsList]: () => [...subscriptions],
    [MethodSubscriptionsAdd]: ({ url, name }) => {
      const link = url.trim()
      let host: string
      try {
        const parsed = new URL(link)
        if (parsed.protocol !== 'http:' && parsed.protocol !== 'https:') throw new Error()
        host = parsed.hostname
      } catch {
        throw failed(ReasonInvalidURL, 'a subscription link starts with http:// or https://')
      }
      if (subscriptions.some((s) => s.url === link)) throw failed(ReasonDuplicate, 'this subscription is already added')
      const sub: Subscription = { id: String(nextId++), name: name.trim() || host, url: link, addedAt: Date.now(), updatedAt: 0 }
      subscriptions = [...subscriptions, sub]
      emit(EventSubscriptionsChanged, [...subscriptions])
      return sub
    },
    [MethodSubscriptionsRemove]: ({ id }) => {
      if (!subscriptions.some((s) => s.id === id)) throw failed(ReasonNotFound, `there is no subscription "${id}"`)
      subscriptions = subscriptions.filter((s) => s.id !== id)
      emit(EventSubscriptionsChanged, [...subscriptions])
      return null
    },
    [MethodPresetsList]: () => [...builtinPresets],
  }

  return {
    async call(method, ...params) {
      const run = methods[method] as (params: unknown) => CoreMethods[typeof method]['result']
      return run(params[0])
    },
    on(event, handler) {
      const set = handlers.get(event) ?? new Set()
      set.add(handler as (params: never) => void)
      handlers.set(event, set)
      return () => set.delete(handler as (params: never) => void)
    },
    status: async () => ({ state: 'ready' }),
    onStatus: () => () => {},
  }
}
