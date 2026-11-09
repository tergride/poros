<script setup lang="ts">
import { Activity, ChartColumn, Route, ScrollText, Server, Settings, Wrench } from '@lucide/vue'
import { onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute } from 'vue-router'

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
  </div>
</template>
