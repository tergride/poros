import { defineStore } from 'pinia'
import { ref } from 'vue'

import type { IncomingPreset } from '@/composables/usePresetImport'
import type { Server } from '@/platform/core-types'
import type { Profile } from '@/platform/profile-types'

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

/**
 * Where a new profile goes: a group of the user's, or a new one of that name, made when the profile is saved.
 */
export interface ProfileTarget {
  groupId: string
  newGroup: string
}

/** A profile open in the editor: a server by its ID, or a new one with its target. */
export interface ProfileEdit {
  id: string
  profile: Profile
  target: ProfileTarget
}

/** The dialogs any screen can open, from the app menu, a button or a link: they live once, in the app shell. */
export const useUiStore = defineStore('ui', () => {
  const addSubscription = ref(false)
  const addPrefill = ref<AddPrefill | null>(null)
  const linkPreset = ref<IncomingPreset | null>(null)
  // The preset an import has just replaced: an editor that has it open shows the new one.
  const replacedPreset = ref<{ id: string } | null>(null)
  // The ID of a subscription a link brought again: the dialog offers to update it or to show it.
  const knownSubscription = ref('')
  // A group to open and bring into sight in the list of the mode.
  const focusGroup = ref('')
  const logs = ref(false)
  const notice = ref<Notice | null>(null)
  // The dialog that moves to the other mode of the interface, opened from the chip of the title bar.
  const modeDialog = ref(false)
  // A link to one server the advanced mode offers to add.
  const linkServer = ref<{ link: string; server: Server; profile: Profile | null } | null>(null)
  // «New profile» and the profile editor of the advanced mode.
  const profilePicker = ref<ProfileTarget | null>(null)
  const profileEdit = ref<ProfileEdit | null>(null)
  // A profile the search picked: «Profiles» shows its group and selects it.
  const focusServer = ref('')
  return {
    addSubscription,
    addPrefill,
    linkPreset,
    replacedPreset,
    knownSubscription,
    focusGroup,
    logs,
    notice,
    modeDialog,
    linkServer,
    profilePicker,
    profileEdit,
    focusServer,
  }
})
