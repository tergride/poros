<script setup lang="ts">
import { computed, onMounted, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'

import AddSubscriptionDialog from '@/components/main/AddSubscriptionDialog.vue'
import LinkPresetDialog from '@/components/main/LinkPresetDialog.vue'
import LogsDialog from '@/components/shell/LogsDialog.vue'
import TitleBar from '@/components/shell/TitleBar.vue'
import { useAttempt } from '@/composables/useAttempt'
import { useConnect } from '@/composables/useConnect'
import { useLinks } from '@/composables/useLinks'
import { useTray } from '@/composables/useTray'
import { useUpdates } from '@/composables/useUpdates'
import { resolveLocale } from '@/i18n/locale'
import { useCoreStore } from '@/stores/core'
import { useUiStore } from '@/stores/ui'

const core = useCoreStore()
const ui = useUiStore()
const route = useRoute()
const router = useRouter()
const attempt = useAttempt()
const { locale } = useI18n()

onMounted(() => void core.start())
useTray()
useLinks()

// A notice belongs to the screen it came from.
watch(
  () => route.path,
  () => (ui.notice = null),
)

// Once per launch: a core restarted later for rights must not connect on its own.
const connect = useConnect()
const stopAutoStart = watch(
  () => core.loaded,
  (loaded) => {
    if (!loaded) return
    stopAutoStart()
    void attempt(() => connect.autoStart())
  },
)

// The language of the settings, or the system's while none is chosen.
watch(
  () => core.settings?.language,
  (language) => {
    locale.value = language || resolveLocale(navigator.languages)
    document.documentElement.lang = locale.value
  },
  { immediate: true },
)

watch(
  () => core.settings?.textSize,
  (size) => document.documentElement.classList.toggle('text-large', size === 'large'),
  { immediate: true },
)

const withMenu = computed(() => route.path.startsWith('/main') || route.path.startsWith('/settings') || route.path.startsWith('/preset'))

// The menu's check: on the help page, where its result shows.
const updates = useUpdates()
async function checkUpdates(): Promise<void> {
  await router.push('/settings/help')
  if (core.updates.available) await attempt(() => updates.check())
}

async function addSubscription(): Promise<void> {
  if (!route.path.startsWith('/main')) await router.push('/main')
  ui.addSubscription = true
}
</script>

<template>
  <div class="flex h-full flex-col">
    <TitleBar
      :menu="withMenu"
      :elevated="core.status.elevated"
      @add-subscription="addSubscription"
      @logs="ui.logs = true"
      @check-updates="checkUpdates"
    />
    <div class="flex min-h-0 flex-1">
      <RouterView />
    </div>
    <LogsDialog v-model:open="ui.logs" />
    <AddSubscriptionDialog v-model:open="ui.addSubscription" />
    <LinkPresetDialog />
  </div>
</template>
