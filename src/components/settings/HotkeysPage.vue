<script setup lang="ts">
import { Keyboard } from '@lucide/vue'
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'

import KeyRecorder from '@/components/settings/KeyRecorder.vue'
import MenuKeysDialog from '@/components/settings/MenuKeysDialog.vue'
import SettingsCard from '@/components/settings/SettingsCard.vue'
import { Button } from '@/components/ui'
import { useSetting } from '@/composables/useSetting'
import { hotkeys as shell } from '@/platform'
import type { Hotkeys } from '@/platform/core-types'
import { useCoreStore } from '@/stores/core'

// «Hotkeys» of the advanced mode: the shortcuts of Windows, which act in both modes with the window hidden too, and
// the dialog of the window's own. Changes wait for «Save»; a shortcut another program holds is named after it.
const core = useCoreStore()
const { t } = useI18n()
const hotkeys = useSetting('hotkeys')
const menuKeys = useSetting('menuKeys')
const dialog = ref(false)

const actions: (keyof Hotkeys)[] = ['show', 'presets', 'system', 'tun', 'start']

const failed = ref<string[]>([])
let stop: (() => void) | undefined
onMounted(async () => {
  failed.value = await shell.failed()
  stop = await shell.onFailed((next) => (failed.value = next))
})
onBeforeUnmount(() => stop?.())

function set(action: keyof Hotkeys, value: string): void {
  hotkeys.value = { ...hotkeys.value, [action]: value }
}

// What keeps a shortcut from working: no modifier, the same keys for another action, or another program.
function problem(action: keyof Hotkeys): string {
  const value = hotkeys.value[action]
  if (!value) return ''
  if (!value.includes('+')) return t('settings.hotkeys.needsModifier')
  const global = actions.find((other) => other !== action && hotkeys.value[other] === value)
  if (global) return t('settings.hotkeys.takenBy', { name: t(`settings.hotkeys.action.${global}`) })
  const menu = Object.entries(menuKeys.value).find(([, shortcut]) => shortcut === value)?.[0]
  if (menu) return t('settings.hotkeys.takenBy', { name: t(`settings.hotkeys.menu.${menu}`) })
  if (failed.value.includes(action) && core.settings?.hotkeys[action] === value) return t('settings.hotkeys.takenOutside', { keys: value })
  return ''
}
const hints = computed(() => Object.fromEntries(actions.map((a) => [a, problem(a)])))
</script>

<template>
  <h2 class="mb-4 font-disp text-[19px] font-normal">{{ t('settings.nav.hotkeys') }}</h2>
  <SettingsCard :title="t('settings.hotkeys.global')">
    <p class="pt-2 pb-1 text-xs text-t3">{{ t('settings.hotkeys.globalHint') }}</p>
    <div v-for="action in actions" :key="action" class="flex items-center gap-3.5 border-t border-line py-2.5 first-of-type:border-t-0" :data-testid="`hotkey-${action}`">
      <div class="min-w-0 flex-1">
        <strong class="block font-medium">{{ t(`settings.hotkeys.action.${action}`) }}</strong>
        <span v-if="hints[action]" class="text-xs text-warn" data-testid="hotkey-problem">{{ hints[action] }}</span>
        <span v-else-if="action === 'presets'" class="text-xs text-t3">{{ t('settings.hotkeys.presetsHint') }}</span>
      </div>
      <KeyRecorder
        :model-value="hotkeys[action]"
        :bad="!!hints[action]"
        :label="t(`settings.hotkeys.action.${action}`)"
        @update:model-value="(v: string) => set(action, v)"
      />
    </div>
  </SettingsCard>
  <SettingsCard :title="t('settings.hotkeys.menuTitle')">
    <div class="flex items-center gap-3.5 py-2.5">
      <div class="min-w-0 flex-1">
        <strong class="block font-medium">{{ t('settings.hotkeys.menuRow') }}</strong>
        <span class="text-xs text-t3">{{ t('settings.hotkeys.menuHint') }}</span>
      </div>
      <Button size="sm" data-testid="menu-keys-open" @click="dialog = true"><Keyboard />{{ t('settings.hotkeys.assign') }}</Button>
    </div>
  </SettingsCard>
  <MenuKeysDialog v-model:open="dialog" v-model:keys="menuKeys" :hotkeys="hotkeys" />
</template>
