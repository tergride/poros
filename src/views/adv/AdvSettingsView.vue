<script setup lang="ts">
import { ArrowRightLeft, Check, Gauge, Globe, Info, Link, Palette, Power, Search, Server, Signpost } from '@lucide/vue'
import { type Component, computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { onBeforeRouteLeave, onBeforeRouteUpdate, useRoute } from 'vue-router'

import ConnPage from '@/components/settings/ConnPage.vue'
import DnsPage from '@/components/settings/DnsPage.vue'
import HelpPage from '@/components/settings/HelpPage.vue'
import InboundPage from '@/components/settings/InboundPage.vue'
import LookPage from '@/components/settings/LookPage.vue'
import StartPage from '@/components/settings/StartPage.vue'
import SubsPage from '@/components/settings/SubsPage.vue'
import TestsPage from '@/components/settings/TestsPage.vue'
import TunPage from '@/components/settings/TunPage.vue'
import { Button, Dialog, Input } from '@/components/ui'
import { useAttempt } from '@/composables/useAttempt'
import { provideSettingsDraft } from '@/composables/useSettingsDraft'
import { useCoreStore } from '@/stores/core'

// The settings of the advanced mode: the pages of the simple mode first, as «Basic». A change waits for «Save»;
// leaving the page with one asks first, and Escape stays.
const core = useCoreStore()
const route = useRoute()
const attempt = useAttempt()
const { t } = useI18n()
const draft = provideSettingsDraft()

interface Page {
  id: string
  icon: Component
  page: Component
}
// «Basic» are the pages of the simple mode, with more of their settings; «More» the pages of the advanced mode alone.
const groups: { id: string; pages: Page[] }[] = [
  {
    id: 'basic',
    pages: [
      { id: 'conn', icon: Globe, page: ConnPage },
      { id: 'dns', icon: Signpost, page: DnsPage },
      { id: 'subs', icon: Link, page: SubsPage },
      { id: 'start', icon: Power, page: StartPage },
      { id: 'look', icon: Palette, page: LookPage },
      { id: 'help', icon: Info, page: HelpPage },
    ],
  },
  {
    id: 'more',
    pages: [
      { id: 'inbound', icon: Server, page: InboundPage },
      { id: 'tests', icon: Gauge, page: TestsPage },
      { id: 'tun', icon: ArrowRightLeft, page: TunPage },
    ],
  },
]
const pages = groups.flatMap((g) => g.pages)
const current = computed(() => pages.find((p) => p.id === route.params.page) ?? pages[0]!)

const query = ref('')
const shown = computed(() => {
  const needle = query.value.trim().toLowerCase()
  return groups
    .map((g) => ({ ...g, pages: g.pages.filter((p) => !needle || t(`settings.nav.${p.id}`).toLowerCase().includes(needle)) }))
    .filter((g) => g.pages.length > 0)
})

const saving = ref(false)
async function save(): Promise<boolean> {
  saving.value = true
  try {
    await draft.save()
    return true
  } catch (error) {
    await attempt(() => Promise.reject(error))
    return false
  } finally {
    saving.value = false
  }
}

// The question before leaving with changes: the answer resolves the navigation it holds.
const asking = ref<((go: boolean) => void) | null>(null)
function ask(): Promise<boolean> | boolean {
  if (!draft.dirty.value) return true
  return new Promise((resolve) => (asking.value = resolve))
}
function answer(choice: 'save' | 'leave' | 'stay'): void {
  const resolve = asking.value
  asking.value = null
  if (!resolve) return
  if (choice === 'leave') {
    draft.discard()
    resolve(true)
  } else if (choice === 'save') {
    void save().then(resolve)
  } else resolve(false)
}
onBeforeRouteLeave(ask)
onBeforeRouteUpdate(ask)
</script>

<template>
  <div class="flex min-h-0 min-w-0 flex-1">
    <nav class="flex w-[206px] flex-none flex-col gap-0.5 overflow-auto border-r border-line bg-s1 p-2.5">
      <div class="relative mb-1.5">
        <Search class="absolute top-2 left-2.5 size-4 text-t3" />
        <Input v-model="query" class="h-8 pl-8" :placeholder="t('adv.settings.search')" :aria-label="t('adv.settings.search')" />
      </div>
      <template v-for="group in shown" :key="group.id">
        <div class="px-2.5 pt-1.5 pb-1 text-[11.5px] font-medium tracking-wide text-t3 uppercase">{{ t(`adv.settings.${group.id}`) }}</div>
        <RouterLink
          v-for="page in group.pages"
          :key="page.id"
          :to="`/adv/settings/${page.id}`"
          :class="[
            'flex min-h-9 items-center gap-2.5 rounded-lg px-2.5 text-[13.5px]',
            page.id === current.id ? 'bg-s4 text-white' : 'text-t2 hover:bg-s3 hover:text-t1',
          ]"
        >
          <component :is="page.icon" class="size-4" />{{ t(`settings.nav.${page.id}`) }}
        </RouterLink>
      </template>
      <p v-if="shown.length === 0" class="px-2.5 py-2 text-xs text-t3">{{ t('adv.settings.nothingFound') }}</p>
    </nav>
    <div class="relative flex min-w-0 flex-1 flex-col">
      <div class="min-h-0 flex-1 overflow-auto px-7 pt-[22px] pb-7">
        <component :is="current.page" v-if="core.settings" />
      </div>
      <div
        v-if="draft.dirty.value"
        class="flex flex-none items-center gap-3 border-t border-line bg-s2 px-7 py-2.5"
        role="status"
        data-testid="savebar"
      >
        <span class="flex-1 text-t2">{{ t('adv.settings.unsaved') }}</span>
        <Button variant="primary" size="sm" :disabled="saving" data-testid="settings-save" @click="save()">
          <Check />{{ t('common.save') }}
        </Button>
      </div>
    </div>

    <Dialog :open="asking !== null" :title="t('adv.unsaved.title')" @update:open="(open) => !open && answer('stay')">
      <p class="text-t2">{{ t('adv.unsaved.text') }}</p>
      <template #footer>
        <Button variant="ghost" class="sp" data-testid="unsaved-leave" @click="answer('leave')">{{ t('adv.unsaved.leave') }}</Button>
        <Button variant="primary" data-testid="unsaved-save" @click="answer('save')"><Check />{{ t('common.save') }}</Button>
      </template>
    </Dialog>
  </div>
</template>
