import { type ComputedRef, type InjectionKey, computed, inject, provide, ref } from 'vue'

import type { Settings } from '@/platform/core-types'
import { useCoreStore } from '@/stores/core'

/**
 * The changes of a settings page of the advanced mode, which waits for «Save» where the simple mode saves each
 * switch at once. A key whose writing does more than the core's settings, such as the kill switch asking for rights,
 * has an action of its own that runs on save.
 */
export interface SettingsDraft {
  dirty: ComputedRef<boolean>
  get<K extends keyof Settings>(key: K): Settings[K]
  set<K extends keyof Settings>(key: K, value: Settings[K]): void
  onSave<K extends keyof Settings>(key: K, apply: (value: Settings[K]) => Promise<unknown>): void
  save(): Promise<void>
  discard(): void
}

const draftKey: InjectionKey<SettingsDraft> = Symbol('settings draft')

/** Starts a draft for the pages below: their settings change here until save. */
export function provideSettingsDraft(): SettingsDraft {
  const core = useCoreStore()
  const changes = ref<Partial<Settings>>({})
  const actions = new Map<keyof Settings, (value: never) => Promise<unknown>>()

  const draft: SettingsDraft = {
    dirty: computed(() => Object.keys(changes.value).length > 0),
    get(key) {
      return (key in changes.value ? changes.value[key] : core.settings?.[key]) as Settings[typeof key]
    },
    set(key, value) {
      const next = { ...changes.value }
      // Back to what is saved is no change.
      if (core.settings?.[key] === value) delete next[key]
      else next[key] = value
      changes.value = next
    },
    onSave(key, apply) {
      actions.set(key, apply as (value: never) => Promise<unknown>)
    },
    async save() {
      const pending = { ...changes.value }
      const plain: Partial<Settings> = {}
      for (const key of Object.keys(pending) as (keyof Settings)[]) {
        if (!actions.has(key)) Object.assign(plain, { [key]: pending[key] })
      }
      if (Object.keys(plain).length > 0) await core.updateSettings(plain)
      changes.value = Object.fromEntries(Object.entries(changes.value).filter(([key]) => !(key in plain)))
      // Each action may ask for rights or be refused: what it did not take stays changed.
      for (const [key, apply] of actions) {
        if (!(key in pending)) continue
        await apply(pending[key] as never)
        const next = { ...changes.value }
        delete next[key]
        changes.value = next
      }
    },
    discard() {
      changes.value = {}
    },
  }
  provide(draftKey, draft)
  return draft
}

/** The draft of the page, or null where settings save at once. */
export function useSettingsDraft(): SettingsDraft | null {
  return inject(draftKey, null)
}
