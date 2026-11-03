<script setup lang="ts">
import { onMounted, onUnmounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'

import { type AppInfo, type CoreStatus, appInfo, useCore } from '@/platform'
import type { AppInfo as CoreInfo } from '@/platform/core-types'
import { MethodAppInfo } from '@/platform/core-types'

const { t } = useI18n()
const info = ref<AppInfo | null>(null)
const status = ref<CoreStatus>({ state: 'starting' })
const engine = ref<CoreInfo | null>(null)
let unsubscribe = () => {}

async function readEngine(): Promise<void> {
  const core = await useCore()
  engine.value = status.value.state === 'ready' ? await core.call(MethodAppInfo) : null
}

onMounted(async () => {
  info.value = await appInfo()
  const core = await useCore()
  unsubscribe = core.onStatus((next) => {
    status.value = next
    void readEngine()
  })
  status.value = await core.status()
  await readEngine()
})

onUnmounted(() => unsubscribe())
</script>

<template>
  <section class="grid h-full place-items-center">
    <div class="text-center">
      <h1 class="font-disp text-2xl font-normal tracking-wide">Poros</h1>
      <p class="mt-3 text-t2">{{ t('skeleton.note') }}</p>
      <p v-if="info" class="mt-6 font-mono text-xs text-t3" data-testid="app-info">
        {{ t('skeleton.version', { version: info.version }) }} · {{ info.dataDir }}
      </p>
      <p class="mt-2 font-mono text-xs text-t3" data-testid="core-info">
        <template v-if="engine">{{ t('skeleton.core', { singBox: engine.singBox, xray: engine.xray }) }}</template>
        <template v-else>{{ t(`skeleton.coreState.${status.state}`) }}</template>
      </p>
      <p v-if="status.state === 'down' && status.error" class="mt-2 max-w-xl text-xs text-bad">{{ status.error }}</p>
    </div>
  </section>
</template>
