<script setup lang="ts">
import { ShieldCheck } from '@lucide/vue'
import { useI18n } from 'vue-i18n'

import { Button, Dialog } from '@/components/ui'
import { useConnect } from '@/composables/useConnect'

// Asked only when TUN is wanted and the core has no rights: the window itself never gets them.
const connect = useConnect()
const { t } = useI18n()
</script>

<template>
  <Dialog v-model:open="connect.adminOpen" :title="t('admin.title')">
    <template #icon><ShieldCheck /></template>
    <p class="text-t2">{{ t('admin.text') }}</p>
    <p class="mt-2.5 text-xs text-t3">
      {{ t('admin.fallback') }}
      <button type="button" class="cursor-pointer text-blue hover:underline" @click="connect.useSystemInstead()">
        {{ t('admin.useSystem') }}
      </button>
    </p>
    <template #footer>
      <Button variant="ghost" class="sp" @click="connect.adminOpen = false">{{ t('common.cancel') }}</Button>
      <Button @click="connect.tryWithoutRights()">{{ t('admin.tryWithout') }}</Button>
      <Button variant="primary" data-testid="admin-restart" @click="connect.elevateAndConnect()">
        <ShieldCheck />{{ t('admin.restart') }}
      </Button>
    </template>
  </Dialog>
</template>
