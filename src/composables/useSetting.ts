import { type WritableComputedRef, computed } from 'vue'

import type { Settings } from '@/platform/core-types'
import { useCoreStore } from '@/stores/core'

/** One setting as a v-model: reading gives the core's value, writing saves it there at once. */
export function useSetting<K extends keyof Settings>(key: K): WritableComputedRef<Settings[K]> {
  const core = useCoreStore()
  return computed({
    get: () => core.settings![key],
    set: (value) => void core.updateSettings({ [key]: value } as Partial<Settings>),
  })
}
