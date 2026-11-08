<script setup lang="ts">
import { Download, EyeOff, Lock, Menu as MenuIcon, Minus, Plus, RefreshCw, ScrollText, Settings, ShieldCheck, Square, X, Zap } from '@lucide/vue'
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'

import mark from '@/assets/img/mark.png'
import wordmark from '@/assets/img/wordmark.png'
import { Button, Menu, MenuItem, MenuSeparator, Segmented } from '@/components/ui'
import { appControls, windowControls } from '@/platform'
import { KillSwitchAlways, KillSwitchConnected } from '@/platform/core-types'

// The wizard has no menu: it leads through its own steps. Each chip tells what it means when the pointer rests on it.
// killSwitch is the mode of the setting: its chip shows while it is on, turns red while it blocks (killSwitchBlocking:
// held after a lost connection, or always without one; killSwitchFull: the whole internet) and opens its setting.
// restartNeeded: saved settings wait for a restart of the running connection, and their chip asks for it. mode is the
// mode of the interface: its chip offers the other one, the menu switches at once.
const props = defineProps<{
  menu?: boolean
  mode?: 'simple' | 'advanced'
  elevated?: boolean
  killSwitch?: string
  killSwitchBlocking?: '' | 'held' | 'always'
  killSwitchFull?: boolean
  restartNeeded?: boolean
}>()
const emit = defineEmits<{
  addSubscription: []
  logs: []
  checkUpdates: []
  restart: []
  killSwitch: []
  modeChip: []
  switchMode: [mode: 'simple' | 'advanced']
}>()

const { t } = useI18n()
const router = useRouter()

const killSwitchOn = computed(() => props.killSwitch === KillSwitchConnected || props.killSwitch === KillSwitchAlways)
const killSwitchHint = computed(() =>
  props.killSwitchBlocking
    ? t(`window.killSwitchBlocking.${props.killSwitchBlocking}${props.killSwitchFull ? 'Full' : ''}`)
    : t(`window.killSwitchHint.${props.killSwitch}`),
)
const modes = computed(() => [
  { value: 'simple', label: t('mode.simpleShort') },
  { value: 'advanced', label: t('mode.advancedShort') },
])
const modeModel = computed({
  get: () => props.mode ?? 'simple',
  set: (next: string) => emit('switchMode', next === 'advanced' ? 'advanced' : 'simple'),
})
const chip = 'inline-flex h-[26px] cursor-default items-center gap-1.5 rounded-full bg-s3 px-2.5 text-xs'

const windowButton = 'h-11 w-[46px] rounded-none border-0 text-t2 hover:bg-s4 hover:text-t1 [&_svg]:stroke-[1.8]'
</script>

<template>
  <header data-tauri-drag-region class="flex h-11 flex-none items-center gap-2 border-b border-line bg-s0 pl-3">
    <div data-tauri-drag-region class="mr-1.5 flex items-center gap-[9px]">
      <img :src="mark" alt="" class="pointer-events-none h-[22px] w-auto" />
      <img :src="wordmark" alt="Poros" class="pointer-events-none h-2.5 w-auto" />
    </div>
    <Menu v-if="menu">
      <template #trigger>
        <Button variant="ghost" size="sm" aria-haspopup="menu"><MenuIcon />{{ t('menu.open') }}</Button>
      </template>
      <MenuItem @select="emit('addSubscription')"><Plus />{{ t('menu.addSubscription') }}</MenuItem>
      <MenuSeparator />
      <MenuItem @select="router.push('/wizard/1')"><Zap />{{ t('menu.wizard') }}</MenuItem>
      <MenuSeparator />
      <div class="px-2 pt-1 pb-1.5 text-[11.5px] text-t3">{{ t('mode.menuLabel') }}</div>
      <div class="px-1.5 pb-1.5"><Segmented v-model="modeModel" :options="modes" :aria-label="t('mode.menuLabel')" /></div>
      <MenuSeparator />
      <!-- The advanced mode has its settings on the rail. -->
      <MenuItem v-if="mode !== 'advanced'" @select="router.push('/settings/presets')"><Settings />{{ t('menu.settings') }}</MenuItem>
      <MenuItem @select="emit('logs')"><ScrollText />{{ t('menu.logs') }}</MenuItem>
      <MenuItem @select="emit('checkUpdates')"><Download />{{ t('menu.checkUpdates') }}</MenuItem>
      <MenuSeparator />
      <MenuItem @select="appControls.hide()"><EyeOff />{{ t('menu.hide') }}</MenuItem>
      <MenuItem @select="appControls.exit()"><X />{{ t('menu.exit') }}</MenuItem>
    </Menu>
    <div data-tauri-drag-region class="ml-auto flex h-full items-center gap-1">
      <div v-if="menu || restartNeeded || killSwitchOn || elevated" data-tauri-drag-region class="mr-2 flex items-center gap-1.5">
        <button
          v-if="restartNeeded"
          type="button"
          :class="[chip, 'cursor-pointer text-blue hover:bg-s4']"
          :title="t('restart.chipHint')"
          data-testid="restart-chip"
          @click="emit('restart')"
        >
          <RefreshCw class="size-3.5" />{{ t('restart.chip') }}
        </button>
        <button
          v-if="killSwitchOn"
          type="button"
          :class="[chip, 'cursor-pointer', killSwitchBlocking ? 'bg-bad/15 text-bad hover:bg-bad/25' : 'text-t2 hover:bg-s4']"
          :title="killSwitchHint"
          :data-blocking="killSwitchBlocking || undefined"
          data-testid="kill-switch-chip"
          @click="emit('killSwitch')"
        >
          <Lock class="size-3.5" />{{ t('window.killSwitch') }}
        </button>
        <span v-if="elevated" :class="[chip, 'text-warn']" :title="t('window.administratorHint')" data-testid="admin-chip">
          <ShieldCheck class="size-3.5" />{{ t('window.administrator') }}
        </span>
        <button
          v-if="menu"
          type="button"
          :class="[chip, 'cursor-pointer text-t2 hover:bg-s4']"
          :title="t(mode === 'advanced' ? 'mode.advancedHint' : 'mode.simpleHint')"
          data-testid="mode-chip"
          @click="emit('modeChip')"
        >
          {{ t(mode === 'advanced' ? 'mode.advanced' : 'mode.simple') }}
        </button>
      </div>
      <Button variant="ghost" :class="windowButton" :aria-label="t('window.minimize')" @click="windowControls.minimize()">
        <Minus />
      </Button>
      <Button
        variant="ghost"
        :class="windowButton"
        :aria-label="t('window.maximize')"
        @click="windowControls.toggleMaximize()"
      >
        <Square class="size-3.5" />
      </Button>
      <Button
        variant="ghost"
        :class="[windowButton, 'hover:bg-close hover:text-white']"
        :aria-label="t('window.close')"
        @click="windowControls.close()"
      >
        <X />
      </Button>
    </div>
  </header>
</template>
