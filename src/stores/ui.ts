import { defineStore } from 'pinia'
import { ref } from 'vue'

import type { PresetCopy } from '@/platform/core-types'

/** A link for the add dialog; check tries it at once, to say why it is no subscription. */
export interface AddPrefill {
  url: string
  name: string
  check: boolean
}

/** The dialogs any screen can open, from the app menu, a button or a link: they live once, in the app shell. */
export const useUiStore = defineStore('ui', () => {
  const addSubscription = ref(false)
  const addPrefill = ref<AddPrefill | null>(null)
  const linkPreset = ref<{ link: string; preset: PresetCopy } | null>(null)
  const logs = ref(false)
  return { addSubscription, addPrefill, linkPreset, logs }
})
