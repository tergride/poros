import {
  type InjectionKey,
  type WritableComputedRef,
  computed,
  getCurrentInstance,
  inject,
  onBeforeUnmount,
  provide,
  reactive,
  ref,
  shallowRef,
} from 'vue'

import { useFailure } from '@/composables/useReason'
import { logError } from '@/lib/logError'
import type { Settings } from '@/platform/core-types'
import { useCoreStore } from '@/stores/core'

/**
 * A card of settings that are typed in or work only together, such as a port, or a password with the switch that
 * asks for it: they change here and go to the core as one on «Apply». A refusal is told in the card and leaves the
 * values to be fixed; «Discard» brings back what is saved.
 */
export interface SettingsBlock {
  readonly dirty: boolean
  readonly applying: boolean
  readonly problem: string
  setting<K extends keyof Settings>(key: K): WritableComputedRef<Settings[K]>
  /** False when the core refused the values. */
  apply(): Promise<boolean>
  discard(): void
}

/** The blocks of a settings page of the advanced mode, for the question before leaving it with changes. */
export interface SettingsPage {
  readonly dirty: boolean
  /** False when the core refused the values of a block, which then tells why. */
  apply(): Promise<boolean>
  discard(): void
}

interface PageContext {
  page: SettingsPage
  add(block: SettingsBlock): void
  remove(block: SettingsBlock): void
}

const pageKey: InjectionKey<PageContext> = Symbol('settings page')

// A part of the settings that is an object counts by its contents.
const same = (a: unknown, b: unknown) => JSON.stringify(a) === JSON.stringify(b)

/** Starts a settings page of the advanced mode: the blocks of the pages below join it. */
export function provideSettingsPage(): SettingsPage {
  const blocks = shallowRef<SettingsBlock[]>([])
  const page: SettingsPage = reactive({
    dirty: computed(() => blocks.value.some((block) => block.dirty)),
    async apply() {
      let applied = true
      for (const block of blocks.value) {
        if (block.dirty && !(await block.apply())) applied = false
      }
      return applied
    },
    discard() {
      for (const block of blocks.value) block.discard()
    },
  })
  provide(pageKey, {
    page,
    add: (block) => (blocks.value = [...blocks.value, block]),
    remove: (block) => (blocks.value = blocks.value.filter((other) => other !== block)),
  })
  return page
}

/** The settings page of the advanced mode, or null in the simple mode. */
export function useSettingsPage(): SettingsPage | null {
  return inject(pageKey, null)?.page ?? null
}

export function useSettingsBlock(): SettingsBlock {
  const core = useCoreStore()
  const failure = useFailure()
  const context = inject(pageKey, null)
  const changes = ref<Partial<Settings>>({})
  const applying = ref(false)
  const problem = ref('')

  // Back to what is saved is no change.
  function prune(): void {
    const kept = Object.entries(changes.value).filter(([key, value]) => !same(value, core.settings?.[key as keyof Settings]))
    if (kept.length < Object.keys(changes.value).length) changes.value = Object.fromEntries(kept)
  }

  const block: SettingsBlock = reactive({
    dirty: computed(() => Object.entries(changes.value).some(([key, value]) => !same(value, core.settings?.[key as keyof Settings]))),
    applying,
    problem,
    setting<K extends keyof Settings>(key: K): WritableComputedRef<Settings[K]> {
      return computed({
        get: () => (key in changes.value ? changes.value[key] : core.settings?.[key]) as Settings[K],
        set: (value: Settings[K]) => {
          changes.value = { ...changes.value, [key]: value }
          problem.value = ''
          prune()
        },
      })
    },
    async apply() {
      prune()
      const sent = { ...changes.value }
      if (Object.keys(sent).length === 0) return true
      applying.value = true
      problem.value = ''
      try {
        await core.updateSettings(sent)
        prune()
        return true
      } catch (error) {
        logError('settings refused', error)
        problem.value = failure(error)
        return false
      } finally {
        applying.value = false
      }
    },
    discard() {
      changes.value = {}
      problem.value = ''
    },
  })
  if (context) {
    context.add(block)
    if (getCurrentInstance()) onBeforeUnmount(() => context.remove(block))
  }
  return block
}
