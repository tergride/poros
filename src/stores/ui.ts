import { defineStore } from 'pinia'
import { ref } from 'vue'

/** The dialogs any screen can open, from the app menu or a button: they live once, in the app shell. */
export const useUiStore = defineStore('ui', () => {
  const addSubscription = ref(false)
  const logs = ref(false)
  return { addSubscription, logs }
})
