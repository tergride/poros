<script setup lang="ts">
import { Minus, Square, X } from '@lucide/vue'
import { useI18n } from 'vue-i18n'

import mark from '@/assets/img/mark.png'
import wordmark from '@/assets/img/wordmark.png'
import { Button } from '@/components/ui/button'
import { windowControls } from '@/platform'

const { t } = useI18n()

const windowButton = 'h-11 w-[46px] rounded-none text-t2 hover:bg-s4 hover:text-t1 [&_svg]:stroke-[1.8]'
</script>

<template>
  <header data-tauri-drag-region class="flex h-11 flex-none items-center gap-2 border-b border-line bg-s0 pl-3">
    <div data-tauri-drag-region class="mr-1.5 flex items-center gap-[9px]">
      <img :src="mark" alt="" class="pointer-events-none h-[22px] w-auto" />
      <img :src="wordmark" alt="Poros" class="pointer-events-none h-2.5 w-auto" />
    </div>
    <div data-tauri-drag-region class="ml-auto flex h-full items-center">
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
