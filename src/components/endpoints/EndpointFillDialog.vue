<script setup lang="ts">
import { KeyRound } from '@lucide/vue'
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'

import ProfileField from '@/components/adv/ProfileField.vue'
import { Banner, Button, Dialog } from '@/components/ui'
import { useEndpointDraft } from '@/composables/useEndpointDraft'
import { useProfileForms } from '@/composables/useProfileForms'
import { partOf } from '@/lib/profileForm'
import type { Field, Profile } from '@/platform/profile-types'
import { SignInCA, SignInCert, SignInKey, SignInKeyPassword, SignInOTP, SignInPassword, SignInUsername, SignInWrapKey } from '@/platform/core-types'

// The data for signing in an endpoint's file came without: only the fields of what it needs, written into the draft
// of the editor. Later puts them back as they were; Done saves the endpoint, which stays as on or off as it was.
const open = defineModel<boolean>('open', { default: false })
const { entry, failure, save } = useEndpointDraft()
const { forms, load, formOf } = useProfileForms()
const { t } = useI18n()
const busy = ref(false)
let before = ''

watch(open, (now) => {
  if (!now || !entry.value) return
  before = JSON.stringify(entry.value.profile)
  failure.value = ''
  void load().catch(() => undefined)
})

const keyOf: Record<string, string> = {
  [SignInUsername]: 'username',
  [SignInPassword]: 'password',
  [SignInCA]: 'ca',
  [SignInCert]: 'cert',
  [SignInKey]: 'key',
  [SignInKeyPassword]: 'key_password',
  [SignInWrapKey]: 'control_wrap.key',
  [SignInOTP]: 'otp',
}
const profile = computed(() => entry.value?.profile as unknown as Record<string, unknown> | undefined)
const fields = computed<Field[]>(() => {
  const current = entry.value
  const form = forms.value ? formOf(current?.profile.type ?? '') : undefined
  if (!current || !form) return []
  const part = partOf(current.profile.type)
  const paths = current.need.map((name) => keyOf[name] && `${part}.${keyOf[name]}`).filter(Boolean)
  return form.fields.filter((field) => paths.includes(field.path))
})

function later(): void {
  if (entry.value && before) entry.value.profile = JSON.parse(before) as Profile
  open.value = false
}

async function done(): Promise<void> {
  busy.value = true
  try {
    if (await save()) open.value = false
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <Dialog :open="open" :title="t('endpoints.fillTitle', { name: entry?.profile.name ?? '' })" class="w-[640px]" @update:open="(o) => !o && later()">
    <template #icon><KeyRound /></template>
    <div class="grid gap-3" data-testid="endpoint-fill">
      <p class="text-t2">{{ t('endpoints.fillText') }}</p>
      <div v-if="profile" class="grid grid-cols-2 items-start gap-x-4 gap-y-3">
        <ProfileField v-for="field in fields" :key="field.path" :field="field" :profile="profile" reason="" server-id="" />
      </div>
      <Banner v-if="failure">{{ failure }}</Banner>
    </div>
    <template #footer>
      <Button variant="ghost" class="sp" data-testid="endpoint-fill-later" @click="later">{{ t('endpoints.later') }}</Button>
      <Button variant="primary" :disabled="busy" data-testid="endpoint-fill-done" @click="done">{{ t('common.done') }}</Button>
    </template>
  </Dialog>
</template>
