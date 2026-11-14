<script setup lang="ts">
import { ShieldCheck } from '@lucide/vue'
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'

import { Button, Dialog } from '@/components/ui'
import { useAttempt } from '@/composables/useAttempt'
import { useConnect } from '@/composables/useConnect'

// Asked when TUN or the kill switch is wanted and the core has no rights: the window itself never gets them.
const connect = useConnect()
const attempt = useAttempt()
const { t } = useI18n()
const tun = computed(() => connect.adminFor === 'tun')
</script>

<template>
  <Dialog v-model:open="connect.adminOpen" :title="tun ? t('admin.title') : t('admin.killSwitchTitle')">
    <template #icon><ShieldCheck /></template>
    <p class="text-t2">{{ tun ? t('admin.text') : t('admin.killSwitchText') }}</p>
    <p v-if="tun" class="mt-2.5 text-xs text-t3">
      {{ t('admin.fallback') }}
      <button type="button" class="cursor-pointer text-blue hover:underline" @click="attempt(() => connect.useSystemInstead())">
        {{ t('admin.useSystem') }}
      </button>
    </p>
    <template #footer>
      <Button variant="ghost" class="sp" @click="connect.adminOpen = false">{{ t('common.cancel') }}</Button>
      <Button @click="attempt(() => connect.tryWithoutRights())">{{ t('admin.tryWithout') }}</Button>
      <Button variant="primary" data-testid="admin-restart" @click="attempt(() => connect.elevateFromDialog())">
        <ShieldCheck />{{ t('admin.restart') }}
      </Button>
    </template>
  </Dialog>
</template>
