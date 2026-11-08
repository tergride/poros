import { defineStore } from 'pinia'
import { ref } from 'vue'

import type { PresetCopy } from '@/platform/core-types'

/** A link for the add dialog; check tries it at once, to say why it is no subscription. */
export interface AddPrefill {
  url: string
  name: string
  check: boolean
}

/** A line over the main list or the settings: what an action did, or why it did not. */
export interface Notice {
  tone: 'info' | 'warn'
  text: string
}

/** The dialogs any screen can open, from the app menu, a button or a link: they live once, in the app shell. */
export const useUiStore = defineStore('ui', () => {
  const addSubscription = ref(false)
  const addPrefill = ref<AddPrefill | null>(null)
  const linkPreset = ref<{ link: string; preset: PresetCopy } | null>(null)
  const logs = ref(false)
  const notice = ref<Notice | null>(null)
  // The dialog that moves to the other mode of the interface, opened from the chip of the title bar.
  const modeDialog = ref(false)
  return { addSubscription, addPrefill, linkPreset, logs, notice, modeDialog }
})
