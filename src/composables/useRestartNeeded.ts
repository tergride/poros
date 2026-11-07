import { defineStore } from 'pinia'
import { computed, ref, watch } from 'vue'

import type { Settings } from '@/platform/core-types'
import { ConnectionRunning } from '@/platform/core-types'
import { useConnect } from '@/composables/useConnect'
import { useCoreStore } from '@/stores/core'

// The settings a running connection was built with; System applies on the fly and the server restarts by itself.
const builtWith = (s: Settings) => JSON.stringify([s.tun, s.preset, s.allowLan, s.ipv6, s.realityOnXray, s.killSwitch])

/**
 * Note 6: after a setting of the running connection changes, the window offers to apply it. Nothing restarts on its
 * own; "Later" hides the offer until the next change.
 */
export const useRestartNeeded = defineStore('restartNeeded', () => {
  const core = useCoreStore()
  const connect = useConnect()
  const applied = ref('')
  const dismissed = ref('')
  // A preset copy saved while it runs changes the connection without changing the settings.
  const presetEdited = ref(false)

  watch(
    () => core.connection.state === ConnectionRunning,
    (running) => {
      if (running && core.settings) {
        applied.value = builtWith(core.settings)
        presetEdited.value = false
      }
    },
    { immediate: true },
  )

  const needed = computed(() => {
    if (!core.running || !core.settings) return false
    const current = builtWith(core.settings)
    return (current !== applied.value || presetEdited.value) && current + presetEdited.value !== dismissed.value
  })

  function later(): void {
    if (core.settings) dismissed.value = builtWith(core.settings) + presetEdited.value
  }

  async function apply(): Promise<void> {
    await connect.connect()
  }

  return { needed, presetEdited, later, apply }
})
