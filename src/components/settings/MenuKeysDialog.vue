<script setup lang="ts">
import { Keyboard } from '@lucide/vue'
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'

import KeyRecorder from '@/components/settings/KeyRecorder.vue'
import { Button, Dialog } from '@/components/ui'
import { windowShortcut } from '@/lib/shortcuts'
import type { Hotkeys, MenuKeys } from '@/platform/core-types'

// The shortcuts of the window's own actions: they act while it has the focus and the cursor is in no field, and the
// menus show them. «Done» gives them to the page, which waits for «Save».
const open = defineModel<boolean>('open', { default: false })
const keys = defineModel<MenuKeys>('keys', { required: true })
const props = defineProps<{ hotkeys: Hotkeys }>()
const { t } = useI18n()

const actions: (keyof MenuKeys)[] = ['search', 'paste', 'newProfile', 'scanQr', 'updateAll', 'testGroup', 'startStop', 'reset', 'settings', 'logs', 'wizard']
// On «Profiles» alone.
const onProfiles = new Set<keyof MenuKeys>(['paste', 'scanQr', 'testGroup'])
const defaults: MenuKeys = { search: 'Ctrl+K', paste: 'Ctrl+V', newProfile: '', scanQr: '', updateAll: '', testGroup: '', startStop: '', reset: '', settings: '', logs: '', wizard: '' }

const draft = ref<MenuKeys>({ ...keys.value })
watch(open, (now) => {
  if (now) draft.value = { ...keys.value }
})

function problem(action: keyof MenuKeys): string {
  const value = draft.value[action]
  if (!value) return ''
  if (!windowShortcut(value)) return t('settings.hotkeys.needsCtrl')
  const other = actions.find((a) => a !== action && draft.value[a] === value)
  if (other) return t('settings.hotkeys.takenBy', { name: t(`settings.hotkeys.menu.${other}`) })
  const global = (Object.keys(props.hotkeys) as (keyof Hotkeys)[]).find((a) => props.hotkeys[a] === value)
  if (global) return t('settings.hotkeys.takenBy', { name: t(`settings.hotkeys.action.${global}`) })
  return ''
}
const problems = computed(() => Object.fromEntries(actions.map((a) => [a, problem(a)])))
const wrong = computed(() => Object.values(problems.value).some(Boolean))

function done(): void {
  keys.value = { ...draft.value }
  open.value = false
}
</script>

<template>
  <Dialog v-model:open="open" :title="t('settings.hotkeys.menuTitle')" class="h-[620px] w-[640px]">
    <template #icon><Keyboard /></template>
    <p class="mb-1.5 text-xs text-t3">{{ t('settings.hotkeys.menuDialogHint') }}</p>
    <div v-for="action in actions" :key="action" class="flex items-center gap-3.5 border-t border-line py-2.5 first-of-type:border-t-0" :data-testid="`menu-key-${action}`">
      <div class="min-w-0 flex-1">
        <strong class="block font-medium">{{ t(`settings.hotkeys.menu.${action}`) }}</strong>
        <span v-if="problems[action]" class="text-xs text-warn" data-testid="menu-key-problem">{{ problems[action] }}</span>
        <span v-else-if="onProfiles.has(action)" class="text-xs text-t3">{{ t('settings.hotkeys.onProfiles') }}</span>
      </div>
      <KeyRecorder v-model="draft[action]" :bad="!!problems[action]" :label="t(`settings.hotkeys.menu.${action}`)" />
    </div>
    <template #footer>
      <Button variant="ghost" class="sp" data-testid="menu-keys-defaults" @click="draft = { ...defaults }">{{ t('settings.hotkeys.defaults') }}</Button>
      <Button variant="ghost" @click="open = false">{{ t('common.cancel') }}</Button>
      <Button variant="primary" :disabled="wrong" data-testid="menu-keys-done" @click="done()">{{ t('common.done') }}</Button>
    </template>
  </Dialog>
</template>
