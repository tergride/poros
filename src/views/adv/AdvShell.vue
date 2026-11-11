<script setup lang="ts">
import { Activity, ChartColumn, Route, ScrollText, Search, Server, Settings, Wrench } from '@lucide/vue'
import { useEventListener } from '@vueuse/core'
import { onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute } from 'vue-router'

import CommandPalette from '@/components/adv/CommandPalette.vue'
import ConnBar from '@/components/adv/ConnBar.vue'
import LinkServerDialog from '@/components/adv/LinkServerDialog.vue'
import ProfileEditor from '@/components/adv/ProfileEditor.vue'
import ProfilePicker from '@/components/adv/ProfilePicker.vue'
import AdminDialog from '@/components/main/AdminDialog.vue'
import IpDialog from '@/components/main/IpDialog.vue'
import ModesDialog from '@/components/main/ModesDialog.vue'
import StatusBanners from '@/components/main/StatusBanners.vue'
import { MethodNetworkCheck } from '@/platform/core-types'
import { useCoreStore } from '@/stores/core'

// The advanced mode: the sections on a rail, the connection bar and the lines of the state over each of them.
const core = useCoreStore()
const route = useRoute()
const { t } = useI18n()
const ipOpen = ref(false)
const paletteOpen = ref(false)

// Ctrl+K opens the search anywhere in the advanced mode, unless a dialog holds the window.
useEventListener(window, 'keydown', (event: KeyboardEvent) => {
  if (!event.ctrlKey || event.altKey || event.shiftKey || event.code !== 'KeyK') return
  if (!paletteOpen.value && document.querySelector('[role="dialog"]')) return
  event.preventDefault()
  paletteOpen.value = !paletteOpen.value
})

const sections = [
  { id: 'profiles', icon: Server },
  { id: 'connections', icon: Activity },
  { id: 'routing', icon: Route },
  { id: 'logs', icon: ScrollText },
  { id: 'stats', icon: ChartColumn },
  { id: 'tools', icon: Wrench },
  { id: 'settings', icon: Settings },
]
const active = (id: string) => route.path.startsWith(`/adv/${id}`)

// As on the main screen of the simple mode: the first look at the address is the window's to ask for.
onMounted(() => {
  if (core.settings?.checkIp && core.network.checkedAt === 0 && !core.network.checking && !core.killSwitch.on && core.ready) {
    void core.core().call(MethodNetworkCheck).catch(() => undefined)
  }
})
</script>

<template>
  <div class="flex min-h-0 min-w-0 flex-1">
    <nav class="flex w-[172px] flex-none flex-col gap-0.5 border-r border-line bg-s0 p-2.5" :aria-label="t('adv.sections')">
      <RouterLink
        v-for="section in sections"
        :key="section.id"
        :to="`/adv/${section.id}`"
        :aria-current="active(section.id) ? 'page' : undefined"
        :class="[
          'flex min-h-9 items-center gap-2.5 rounded-lg px-2.5 text-[13.5px]',
          active(section.id) ? 'bg-s4 text-white' : 'text-t2 hover:bg-s3 hover:text-t1',
        ]"
        :data-testid="`rail-${section.id}`"
      >
        <component :is="section.icon" class="size-4" />{{ t(`adv.nav.${section.id}`) }}
      </RouterLink>
      <button
        type="button"
        class="mt-auto flex min-h-9 cursor-pointer items-center gap-2.5 rounded-lg border border-line2 px-2.5 text-[13px] text-t2 hover:bg-s3 hover:text-t1"
        data-testid="rail-search"
        @click="paletteOpen = true"
      >
        <Search class="size-4" />{{ t('adv.palette.button') }}<kbd class="ml-auto font-mono text-[10.5px] font-medium">Ctrl K</kbd>
      </button>
    </nav>
    <div class="flex min-h-0 min-w-0 flex-1 flex-col bg-s1">
      <ConnBar @ip="ipOpen = true" />
      <div class="flex flex-col gap-2.5 px-3.5 pt-2.5 empty:hidden"><StatusBanners /></div>
      <div class="flex min-h-0 min-w-0 flex-1"><RouterView /></div>
    </div>
    <ModesDialog />
    <AdminDialog />
    <IpDialog v-model:open="ipOpen" />
    <LinkServerDialog />
    <ProfilePicker />
    <ProfileEditor />
    <CommandPalette v-model:open="paletteOpen" />
  </div>
</template>
