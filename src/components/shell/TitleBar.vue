<script setup lang="ts">
import { Menu as MenuIcon, Minus, Plus, ScrollText, Settings, ShieldCheck, Square, X, Zap } from '@lucide/vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'

import mark from '@/assets/img/mark.png'
import wordmark from '@/assets/img/wordmark.png'
import { Button, Menu, MenuItem, MenuSeparator } from '@/components/ui'
import { windowControls } from '@/platform'

// The wizard has no menu: it leads through its own steps.
defineProps<{ menu?: boolean; elevated?: boolean }>()
const emit = defineEmits<{ addSubscription: []; logs: [] }>()

const { t } = useI18n()
const router = useRouter()

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
      <MenuItem @select="router.push('/settings/presets')"><Settings />{{ t('menu.settings') }}</MenuItem>
      <MenuItem @select="emit('logs')"><ScrollText />{{ t('menu.logs') }}</MenuItem>
      <MenuSeparator />
      <MenuItem @select="windowControls.close()"><X />{{ t('menu.exit') }}</MenuItem>
    </Menu>
    <div data-tauri-drag-region class="ml-auto flex h-full items-center gap-1">
      <span
        v-if="elevated"
        class="mr-2 inline-flex h-[26px] items-center gap-1.5 rounded-full bg-s3 px-2.5 text-xs text-warn"
        data-testid="admin-chip"
      >
        <ShieldCheck class="size-3.5" />{{ t('window.administrator') }}
      </span>
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
