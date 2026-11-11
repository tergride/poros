<script setup lang="ts">
import { Download, Gauge, Plus, Radar, RefreshCw, Route, Search, Server, Settings, Zap } from '@lucide/vue'
import { DialogContent, DialogOverlay, DialogPortal, DialogRoot, DialogTitle } from 'reka-ui'
import { type Component, computed, nextTick, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'

import { useAppActions } from '@/composables/useAppActions'
import { serverTitle } from '@/lib/format'
import { score, settingLabels } from '@/lib/palette'
import { useCoreStore } from '@/stores/core'
import { useUiStore } from '@/stores/ui'

// Search, Ctrl+K: the pages and the settings on them, the actions of the menus, the profiles of every group and the
// presets. ↑ and ↓ move, Enter opens, Escape closes. Without a query it offers the actions and the pages.
const open = defineModel<boolean>('open', { default: false })
const core = useCoreStore()
const ui = useUiStore()
const router = useRouter()
const app = useAppActions()
const { t, te, locale, getLocaleMessage } = useI18n()

type GroupId = 'settings' | 'actions' | 'profiles' | 'presets'
interface Item {
  key: string
  group: GroupId
  icon: Component
  title: string
  hint: string
  // More words that find the item, as the address of a profile.
  words?: string
  run: () => unknown
}

// The pages of «Settings» of the advanced mode, in the order of their list.
const basicPages = ['conn', 'dns', 'subs', 'start', 'look', 'help']
const morePages = ['inbound', 'tests', 'tun', 'protocols', 'net', 'security', 'logging', 'hotkeys', 'backup']
// Names the pages show as they are, untranslated: the terms of sing-box.
const literals: Record<string, string[]> = {
  tun: ['MTU', 'Strict Route', 'TUN Routing', 'IPv4 CIDR', 'IPv6 CIDR'],
  protocols: ['Disable Path MTU Discovery'],
}

const query = ref('')
const current = ref(0)
const list = ref<HTMLElement | null>(null)
watch(open, (now) => {
  if (now) query.value = ''
})

// Actions close the palette first: what they open or say shows over the window.
function closed(action: () => unknown): () => void {
  return () => {
    open.value = false
    void action()
  }
}

function go(path: string): void {
  open.value = false
  void router.push(path)
}

const settingItems = computed<Item[]>(() => {
  const messages = getLocaleMessage(locale.value) as { settings?: Record<string, unknown> }
  const items: Item[] = []
  for (const [group, pages] of [['basic', basicPages], ['more', morePages]] as const) {
    for (const page of pages) {
      const title = t(`settings.nav.${page}`)
      items.push({ key: `page:${page}`, group: 'settings', icon: Settings, title, hint: t(`adv.settings.${group}`), run: () => go(`/adv/settings/${page}`) })
      for (const label of [...settingLabels(messages.settings?.[page]), ...(literals[page] ?? [])]) {
        if (label !== title) items.push({ key: `setting:${page}:${label}`, group: 'settings', icon: Settings, title: label, hint: title, run: () => go(`/adv/settings/${page}`) })
      }
    }
  }
  return items
})

// An action shows its shortcut of the window, when it has one.
const keys = computed(() => core.settings?.menuKeys)
const actionItems = computed<Item[]>(() => [
  { key: 'new-profile', group: 'actions', icon: Plus, title: t('profileEditor.newProfile'), hint: keys.value?.newProfile ?? '', run: closed(app.newProfile) },
  { key: 'update-all', group: 'actions', icon: Radar, title: t('profiles.updateAll'), hint: keys.value?.updateAll ?? '', run: closed(app.updateAll) },
  { key: 'test-all', group: 'actions', icon: Gauge, title: t('adv.palette.testAll'), hint: '', run: closed(app.testAll) },
  { key: 'wizard', group: 'actions', icon: Zap, title: t('menu.wizard'), hint: keys.value?.wizard ?? '', run: closed(app.wizard) },
  { key: 'reset', group: 'actions', icon: RefreshCw, title: t('settings.help.reset'), hint: keys.value?.reset ?? '', run: closed(app.reset) },
  { key: 'updates', group: 'actions', icon: Download, title: t('menu.checkUpdates'), hint: '', run: closed(app.checkUpdates) },
])

const profileItems = computed<Item[]>(() =>
  core.servers.map((s) => ({
    key: `profile:${s.id}`,
    group: 'profiles' as const,
    icon: Server,
    title: serverTitle(s.name, s.country),
    hint: core.groups.find((g) => g.id === s.groupId)?.name ?? '',
    words: s.host,
    run: () => {
      ui.focusServer = s.id
      go('/adv/profiles')
    },
  })),
)

const presetItems = computed<Item[]>(() =>
  core.presets.map((p) => ({
    key: `preset:${p.id}`,
    group: 'presets' as const,
    icon: Route,
    title: p.builtin && te(`presets.name.${p.id}`) ? t(`presets.name.${p.id}`) : p.name,
    hint: p.builtin ? t('adv.palette.builtin') : p.advanced ? t('adv.palette.advanced') : '',
    run: () => go(`/adv/routing/rules/${p.id}`),
  })),
)

const limits: Record<GroupId, number> = { settings: 6, actions: 6, profiles: 8, presets: 5 }
const order: GroupId[] = ['settings', 'actions', 'profiles', 'presets']

// The items that fit, best first in each group, the groups in the order of the mockup.
const shown = computed(() => {
  const q = query.value.trim()
  if (!q) {
    return [
      { group: 'actions' as GroupId, items: actionItems.value },
      { group: 'settings' as GroupId, items: settingItems.value.filter((i) => i.key.startsWith('page:')) },
    ]
  }
  const all = [...settingItems.value, ...actionItems.value, ...profileItems.value, ...presetItems.value]
  return order
    .map((group) => ({
      group,
      items: all
        .filter((i) => i.group === group)
        .map((i) => ({ i, s: score(q, i.title, `${i.hint} ${i.words ?? ''}`) }))
        .filter((x): x is { i: Item; s: number } => x.s !== null)
        .sort((a, b) => a.s - b.s)
        .slice(0, limits[group])
        .map((x) => x.i),
    }))
    .filter((g) => g.items.length > 0)
})
const flat = computed(() => shown.value.flatMap((g) => g.items))
watch(query, () => (current.value = 0))

function move(by: number): void {
  const n = flat.value.length
  if (n === 0) return
  current.value = (current.value + by + n) % n
  void nextTick(() => list.value?.querySelector('[data-current="true"]')?.scrollIntoView({ block: 'nearest' }))
}

function run(item: Item | undefined): void {
  if (item) void item.run()
}
</script>

<template>
  <DialogRoot v-model:open="open">
    <DialogPortal to="#app">
      <DialogOverlay class="fixed inset-0 z-40 bg-s0/60" />
      <DialogContent
        :aria-describedby="undefined"
        class="fixed top-[90px] left-1/2 z-50 w-[600px] max-w-[calc(100%-32px)] -translate-x-1/2 overflow-hidden rounded-[14px] border border-line2 bg-s2 shadow-[0_24px_60px_rgba(0,0,0,.55)] outline-none"
        data-testid="palette"
        @keydown.down.prevent="move(1)"
        @keydown.up.prevent="move(-1)"
        @keydown.enter.prevent="run(flat[current])"
      >
        <DialogTitle class="sr-only">{{ t('adv.palette.title') }}</DialogTitle>
        <div class="flex h-[50px] items-center gap-2.5 border-b border-line px-3.5">
          <Search class="size-5 flex-none text-t3" />
          <input
            v-model="query"
            class="h-full min-w-0 flex-1 border-0 bg-transparent text-[15px] text-t1 outline-none placeholder:text-t3"
            :placeholder="t('adv.palette.placeholder')"
            :aria-label="t('adv.palette.title')"
            autofocus
            data-testid="palette-input"
          />
          <kbd class="font-mono text-[11px] text-t3">Esc</kbd>
        </div>
        <div ref="list" class="max-h-[360px] overflow-auto p-1.5" role="listbox">
          <template v-for="g in shown" :key="g.group">
            <div class="px-2.5 pt-2 pb-1 text-[11.5px] text-t3">{{ t(`adv.palette.group.${g.group}`) }}</div>
            <button
              v-for="item in g.items"
              :key="item.key"
              type="button"
              role="option"
              :aria-selected="flat[current] === item"
              :data-current="flat[current] === item"
              :class="[
                'flex min-h-[34px] w-full cursor-pointer items-center gap-2.5 rounded-md px-2.5 text-left text-[13px] [&>svg]:size-4 [&>svg]:flex-none [&>svg]:text-t2',
                flat[current] === item ? 'bg-s5 text-white' : 'text-t1 hover:bg-s4',
              ]"
              data-testid="palette-item"
              @mousemove="current = flat.indexOf(item)"
              @click="run(item)"
            >
              <component :is="item.icon" />
              <span class="min-w-0 truncate">{{ item.title }}</span>
              <small v-if="item.hint" class="ml-auto flex-none truncate pl-3 text-[11.5px] text-t3">{{ item.hint }}</small>
            </button>
          </template>
          <p v-if="flat.length === 0" class="px-2.5 py-3 text-[13px] text-t3" data-testid="palette-empty">{{ t('adv.palette.nothing') }}</p>
        </div>
        <div class="flex gap-3.5 border-t border-line px-3.5 py-2 text-[11.5px] text-t3">
          <span><kbd class="font-mono">↑</kbd> <kbd class="font-mono">↓</kbd> {{ t('adv.palette.move') }}</span>
          <span><kbd class="font-mono">Enter</kbd> {{ t('adv.palette.open') }}</span>
          <span><kbd class="font-mono">Esc</kbd> {{ t('adv.palette.close') }}</span>
        </div>
      </DialogContent>
    </DialogPortal>
  </DialogRoot>
</template>
