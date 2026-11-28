import { type WritableComputedRef, computed } from 'vue'

import { useAttempt } from '@/composables/useAttempt'
import type { Settings } from '@/platform/core-types'
import { useCoreStore } from '@/stores/core'
import { useUiStore } from '@/stores/ui'

/**
 * One setting as a v-model that the core applies at once: the control shows the new value right away and takes no
 * other until the core answers. A refusal is told in the notice line, and the control goes back to the core's value.
 */
export function useSetting<K extends keyof Settings>(key: K): WritableComputedRef<Settings[K]> {
  return useSettingAction(key, null)
}

/** A setting whose writing does more than the core's settings, such as asking for rights first. */
export function useSettingAction<K extends keyof Settings>(
  key: K,
  apply: ((value: Settings[K]) => Promise<unknown>) | null,
): WritableComputedRef<Settings[K]> {
  const core = useCoreStore()
  const ui = useUiStore()
  const attempt = useAttempt()
  const write = apply ?? ((value: Settings[K]) => core.updateSettings({ [key]: value }))
  return computed({
    get: () => (key in ui.applying ? ui.applying[key] : core.settings![key]) as Settings[K],
    set: (value) => {
      if (key in ui.applying) return
      ui.applying = { ...ui.applying, [key]: value }
      void attempt(() => write(value)).finally(() => {
        const rest = { ...ui.applying }
        delete rest[key]
        ui.applying = rest
      })
    },
  })
}

/**
 * A typed setting saved at once, such as an address taken on Enter: it shows the core's value only, so that a field
 * keeps what was typed after a refusal, to be fixed.
 */
export function useTypedSetting<K extends keyof Settings>(key: K): WritableComputedRef<Settings[K]> {
  const core = useCoreStore()
  const attempt = useAttempt()
  return computed({
    get: () => core.settings![key],
    set: (value) => void attempt(() => core.updateSettings({ [key]: value })),
  })
}
