<script setup lang="ts">
import { Square, Zap } from '@lucide/vue'
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'

import { Button, Flag } from '@/components/ui'
import { useAttempt } from '@/composables/useAttempt'
import { serverTitle } from '@/lib/format'
import { EventServersSpeedTested, type ServerSpeed } from '@/platform/core-types'
import { useCoreStore } from '@/stores/core'

// «Tools → Other»: the speed of the chosen server, measured as «Speed test» of its menu, in a test core of its own: the
// connection keeps running. The mode and the address are those of «Settings → Tests».
const core = useCoreStore()
const attempt = useAttempt()
const { t, locale } = useI18n()

const server = computed(() => core.server)
const running = ref(false)
const result = ref<(ServerSpeed & { at: number }) | null>(null)
let measuring = ''
let off: (() => void) | undefined

onMounted(() => {
  off = core.core().on(EventServersSpeedTested, (report) => {
    if (!running.value) return
    const own = report.results.find((r) => r.serverId === measuring)
    if (own) result.value = { ...own, at: Date.now() }
    if (report.done) running.value = false
  })
})
onBeforeUnmount(() => off?.())

async function start(): Promise<void> {
  const s = server.value
  if (!s) return
  measuring = s.id
  result.value = null
  running.value = true
  await attempt(async () => {
    try {
      await core.speedTestServers('', [s.id])
    } catch (error) {
      running.value = false
      throw error
    }
  })
}

async function stop(): Promise<void> {
  await attempt(() => core.stopTests())
  running.value = false
}

const when = computed(() =>
  result.value ? new Date(result.value.at).toLocaleTimeString(locale.value, { hour: '2-digit', minute: '2-digit' }) : '',
)
const none = '—'
</script>

<template>
  <section class="rounded-xl border border-line bg-s2 px-4 py-3" data-testid="speed-card">
    <h3 class="text-[13px] font-semibold">{{ t('adv.tools.speedTitle') }}</h3>
    <div v-if="running" class="mt-2 mb-3 flex items-center gap-2.5 rounded-[10px] border border-line bg-s1 px-3.5 py-2.5 text-[13px]" data-testid="speed-running">
      <Zap class="size-4 flex-none animate-pulse text-blue" />{{ t('adv.tools.speedRunning', { name: server ? serverTitle(server.name, server.country) : '' }) }}
    </div>
    <template v-else-if="result">
      <dl class="mt-2 mb-3 grid grid-cols-[auto_1fr] gap-x-3.5 gap-y-1 text-[13px]" data-testid="speed-result">
        <dt class="text-t3">{{ t('adv.tools.speedServer') }}</dt>
        <dd class="flex items-center justify-end gap-1.5 truncate font-mono text-xs"><Flag v-if="server?.country" :code="server.country" />{{ server ? serverTitle(server.name, server.country) : none }}</dd>
        <template v-if="result.error">
          <dt class="text-t3">{{ t('adv.tools.speedError') }}</dt>
          <dd class="truncate text-right text-xs text-warn" data-testid="speed-error">{{ result.error }}</dd>
        </template>
        <template v-else>
          <dt class="text-t3">{{ t('adv.tools.speedDown') }}</dt>
          <dd class="text-right font-mono text-xs" data-testid="speed-down">{{ result.download || none }}</dd>
          <dt class="text-t3">{{ t('adv.tools.speedUp') }}</dt>
          <dd class="text-right font-mono text-xs" data-testid="speed-up">{{ result.upload || none }}</dd>
          <dt class="text-t3">{{ t('adv.tools.speedCountry') }}</dt>
          <dd class="flex justify-end"><Flag v-if="result.country" :code="result.country" /><template v-else>{{ none }}</template></dd>
        </template>
        <dt class="text-t3">{{ t('adv.tools.speedWhen') }}</dt>
        <dd class="text-right font-mono text-xs">{{ when }}</dd>
      </dl>
    </template>
    <p v-else class="pt-1.5 pb-2.5 text-xs text-t3">
      {{ server ? t('adv.tools.speedHint', { name: serverTitle(server.name, server.country) }) : t('adv.tools.speedNoServer') }}
    </p>
    <div class="pb-1">
      <Button v-if="running" data-testid="speed-stop" @click="stop()"><Square />{{ t('adv.tools.speedStop') }}</Button>
      <Button v-else :disabled="!server || !core.ready" data-testid="speed-start" @click="start()">
        <Zap />{{ t(result ? 'adv.tools.speedAgain' : 'adv.tools.speedStart') }}
      </Button>
    </div>
  </section>
</template>
