<script setup lang="ts">
import { ArrowLeft, Globe, Info, Layers, Link, Palette, Power, RefreshCw } from '@lucide/vue'
import { type Component, computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute } from 'vue-router'

import ConnPage from '@/components/settings/ConnPage.vue'
import HelpPage from '@/components/settings/HelpPage.vue'
import LookPage from '@/components/settings/LookPage.vue'
import PresetsPage from '@/components/settings/PresetsPage.vue'
import StartPage from '@/components/settings/StartPage.vue'
import SubsPage from '@/components/settings/SubsPage.vue'
import { Banner, Button } from '@/components/ui'
import { useRestartNeeded } from '@/composables/useRestartNeeded'
import { useCoreStore } from '@/stores/core'

const core = useCoreStore()
const restart = useRestartNeeded()
const route = useRoute()
const { t } = useI18n()

const pages: { id: string; icon: Component; page: Component }[] = [
  { id: 'presets', icon: Layers, page: PresetsPage },
  { id: 'conn', icon: Globe, page: ConnPage },
  { id: 'subs', icon: Link, page: SubsPage },
  { id: 'start', icon: Power, page: StartPage },
  { id: 'look', icon: Palette, page: LookPage },
  { id: 'help', icon: Info, page: HelpPage },
]
const current = computed(() => pages.find((p) => p.id === route.params.page) ?? pages[0]!)
</script>

<template>
  <div class="flex min-h-0 flex-1">
    <nav class="flex w-[232px] flex-none flex-col gap-0.5 overflow-auto border-r border-line bg-s0 p-3">
      <RouterLink to="/main" class="flex min-h-9 items-center gap-2.5 rounded-lg px-2.5 text-[13.5px] text-t2 hover:bg-s3 hover:text-t1">
        <ArrowLeft class="size-4" />{{ t('settings.back') }}
      </RouterLink>
      <div class="my-[5px] h-px bg-line" />
      <RouterLink
        v-for="page in pages"
        :key="page.id"
        :to="`/settings/${page.id}`"
        :class="[
          'flex min-h-9 items-center gap-2.5 rounded-lg px-2.5 text-[13.5px]',
          page.id === current.id ? 'bg-s4 text-white' : 'text-t2 hover:bg-s3 hover:text-t1',
        ]"
      >
        <component :is="page.icon" class="size-4" />{{ t(`settings.nav.${page.id}`) }}
      </RouterLink>
    </nav>
    <div class="min-w-0 flex-1 overflow-auto px-7 pt-[22px] pb-7">
      <Banner v-if="restart.needed" tone="info" class="mb-3" data-testid="restart-banner">
        {{ t('restartBanner.text') }}
        <template #actions>
          <Button size="sm" variant="primary" @click="restart.apply()"><RefreshCw />{{ t('restartBanner.apply') }}</Button>
          <Button size="sm" variant="ghost" @click="restart.later()">{{ t('common.later') }}</Button>
        </template>
      </Banner>
      <component :is="current.page" v-if="core.settings" />
    </div>
  </div>
</template>
