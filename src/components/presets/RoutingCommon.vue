<script setup lang="ts">
import { Check } from '@lucide/vue'
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { onBeforeRouteLeave, onBeforeRouteUpdate } from 'vue-router'

import SettingsCard from '@/components/settings/SettingsCard.vue'
import { Button, Dialog, Select } from '@/components/ui'
import { useAttempt } from '@/composables/useAttempt'
import { provideSettingsDraft } from '@/composables/useSettingsDraft'
import { AddressFirstIPv4, AddressFirstIPv6 } from '@/platform/core-types'
import { useCoreStore } from '@/stores/core'

// «General» of the routing: which address of a site with both to take first, and the preset in use, the same choice
// as in the connection bar. Changes wait for «Save», and leaving with one asks first, as on the settings pages.
const core = useCoreStore()
const attempt = useAttempt()
const { t, te } = useI18n()
const draft = provideSettingsDraft()

const addressFirst = computed({ get: () => draft.get('addressFirst'), set: (value: string) => draft.set('addressFirst', value) })
const preset = computed({ get: () => draft.get('preset'), set: (value: string) => draft.set('preset', value) })
const families = computed(() => [
  { value: '', label: t('routing.common.auto') },
  { value: AddressFirstIPv4, label: 'IPv4' },
  { value: AddressFirstIPv6, label: 'IPv6' },
])
const presets = computed(() =>
  core.presets.map((p) => ({ value: p.id, label: p.builtin && te(`presets.name.${p.id}`) ? t(`presets.name.${p.id}`) : p.name })),
)

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
  <div class="flex min-h-0 flex-1 flex-col">
    <div class="min-h-0 flex-1 overflow-auto px-7 pt-4 pb-7">
      <SettingsCard :title="t('routing.common.title')">
        <div class="grid grid-cols-2 items-start gap-x-4 gap-y-3 pt-2.5 pb-1">
          <div class="grid gap-[5px]">
            <span class="text-[12.5px] text-t2">{{ t('routing.common.addressFirst') }}</span>
            <Select
              v-model="addressFirst"
              :options="families"
              :disabled="!core.settings?.ipv6"
              class="w-full"
              :aria-label="t('routing.common.addressFirst')"
              data-testid="address-first"
            />
            <span class="text-xs text-t3">{{ t('routing.common.addressFirstHint') }}</span>
          </div>
          <div class="grid gap-[5px]">
            <span class="text-[12.5px] text-t2">{{ t('routing.common.preset') }}</span>
            <Select v-model="preset" :options="presets" class="w-full" :aria-label="t('routing.common.preset')" data-testid="common-preset" />
            <span class="text-xs text-t3">{{ t('routing.common.presetHint') }}</span>
          </div>
        </div>
      </SettingsCard>
    </div>
    <div v-if="draft.dirty.value" class="flex flex-none items-center gap-3 border-t border-line bg-s2 px-7 py-2.5" role="status" data-testid="savebar">
      <span class="flex-1 text-t2">{{ t('adv.settings.unsaved') }}</span>
      <Button variant="primary" size="sm" :disabled="saving" data-testid="common-save" @click="save()"><Check />{{ t('common.save') }}</Button>
    </div>
    <Dialog :open="asking !== null" :title="t('adv.unsaved.title')" @update:open="(o) => !o && answer('stay')">
      <p class="text-t2">{{ t('adv.unsaved.text') }}</p>
      <template #footer>
        <Button variant="ghost" class="sp" data-testid="unsaved-leave" @click="answer('leave')">{{ t('adv.unsaved.leave') }}</Button>
        <Button variant="primary" data-testid="unsaved-save" @click="answer('save')"><Check />{{ t('common.save') }}</Button>
      </template>
    </Dialog>
  </div>
</template>
