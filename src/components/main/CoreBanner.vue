<script setup lang="ts">
import { RefreshCw } from '@lucide/vue'
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'

import { Banner, Button } from '@/components/ui'
import { useAttempt } from '@/composables/useAttempt'
import { StatusNotAnswering } from '@/platform'
import { useCoreStore } from '@/stores/core'

// The core after the first start, over the main list and the settings: stopped, starting again, or silent. While
// it is so, the buttons that need it wait, and this line says why.
const core = useCoreStore()
const attempt = useAttempt()
const { t } = useI18n()
const restarting = ref(false)

const text = computed(() => {
  const status = core.status
  if (status.state === 'ready') return status.reason === StatusNotAnswering ? t('boot.notAnswering') : ''
  if (status.state === 'starting') return t('boot.restarting')
  return `${t('boot.down')} ${t('boot.downHint')}`
})

async function restart(): Promise<void> {
  restarting.value = true
  await attempt(() => core.core().restart())
  restarting.value = false
}
</script>

<template>
  <Banner v-if="text" data-testid="core-banner" :title="core.status.error">
    {{ text }}
    <template v-if="core.status.state === 'down'" #actions>
      <Button size="sm" :disabled="restarting" @click="restart()"><RefreshCw />{{ t('boot.restart') }}</Button>
    </template>
  </Banner>
</template>
