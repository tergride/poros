<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'

import { type AppInfo, appInfo } from '@/platform'

const { t } = useI18n()
const info = ref<AppInfo | null>(null)

onMounted(async () => {
  info.value = await appInfo()
})
</script>

<template>
  <section class="grid h-full place-items-center">
    <div class="text-center">
      <h1 class="font-disp text-2xl font-normal tracking-wide">Poros</h1>
      <p class="mt-3 text-t2">{{ t('skeleton.note') }}</p>
      <p v-if="info" class="mt-6 font-mono text-xs text-t3" data-testid="app-info">
        {{ t('skeleton.version', { version: info.version }) }} · {{ info.dataDir }}
      </p>
    </div>
  </section>
</template>
