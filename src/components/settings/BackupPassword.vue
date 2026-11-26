<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'

import OptionRow from '@/components/settings/OptionRow.vue'
import { Input, Switch } from '@/components/ui'

// The password of a new backup: typed twice, so that a slip does not lock the file for good.
const on = defineModel<boolean>('on', { default: false })
const password = defineModel<string>('password', { default: '' })
const repeat = defineModel<string>('repeat', { default: '' })
const { t } = useI18n()
const mismatch = computed(() => repeat.value !== '' && password.value !== repeat.value)
</script>

<template>
  <OptionRow :title="t('settings.backup.encrypt')" :hint="t('settings.backup.encryptHint')" :note="t('settings.backup.encryptNote')">
    <Switch v-model="on" data-testid="backup-encrypt" />
  </OptionRow>
  <div v-if="on" class="grid grid-cols-2 gap-x-4 gap-y-2 pt-1 pb-3">
    <label class="grid gap-[5px]">
      <span class="text-[12.5px] text-t2">{{ t('settings.backup.password') }}</span>
      <Input v-model="password" type="password" autocomplete="new-password" data-testid="backup-password" />
    </label>
    <label class="grid gap-[5px]">
      <span class="text-[12.5px] text-t2">{{ t('settings.backup.repeat') }}</span>
      <Input v-model="repeat" type="password" autocomplete="new-password" data-testid="backup-repeat" />
    </label>
    <p v-if="mismatch" class="col-span-2 text-xs text-warn" data-testid="backup-mismatch">{{ t('settings.backup.mismatch') }}</p>
  </div>
</template>
