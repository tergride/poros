<script setup lang="ts">
import { computed, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'

import ProfileField from '@/components/adv/ProfileField.vue'
import SettingsCard from '@/components/settings/SettingsCard.vue'
import { useEndpointDraft } from '@/composables/useEndpointDraft'
import { useProfileForms } from '@/composables/useProfileForms'
import { fieldState, getAt, holds } from '@/lib/profileForm'
import type { Field } from '@/platform/profile-types'
import { TabAdvanced, TabCommon, TabSecurity } from '@/platform/profile-types'

// The connection of an endpoint: the form of its profile the core describes, as cards. The data for signing in has a
// card of its own, since a file without it leaves exactly that out; the key of tls-crypt-v2 is the client's own and
// goes there too.
const { entry } = useEndpointDraft()
const { forms, load, formOf } = useProfileForms()
const { t } = useI18n()
onMounted(() => void load().catch(() => undefined))

const profile = computed(() => entry.value?.profile as unknown as Record<string, unknown> | undefined)
const type = computed(() => entry.value?.profile.type ?? '')
const form = computed(() => (forms.value ? formOf(type.value) : undefined))

function signIn(field: Field): boolean {
  const own = ['username', 'password', 'otp', 'cert', 'key', 'key_password'].map((name) => `${type.value}.${name}`)
  if (own.includes(field.path)) return true
  return field.path === 'openvpn.control_wrap.key' && getAt(profile.value, 'openvpn.control_wrap.type') === 'tls_crypt_v2'
}

const shown = computed(() => {
  const p = profile.value
  if (!p || !form.value) return []
  return form.value.fields.filter((field) => field.path !== 'name' && fieldState(field, p) !== 'hidden')
})
const cards = computed(() => [
  { id: 'server', title: t('endpoints.server'), hint: t('endpoints.serverHint'), fields: shown.value.filter((f) => f.tab === TabCommon && !signIn(f)) },
  { id: 'signIn', title: t('endpoints.signIn'), hint: t('endpoints.signInHint'), fields: shown.value.filter(signIn) },
  { id: 'protection', title: t('endpoints.protection'), hint: '', fields: shown.value.filter((f) => f.tab === TabSecurity && !signIn(f)) },
  { id: 'more', title: t('endpoints.more'), hint: '', fields: shown.value.filter((f) => f.tab === TabAdvanced && !signIn(f)) },
])
// Why an inactive field does not apply, as the profile editor says it: the field its first failed condition looks at.
function reason(field: Field): string {
  const p = profile.value
  if (!p || fieldState(field, p) !== 'inactive') return ''
  const failed = (field.when ?? []).find((c) => !c.in?.length && !holds(c, p))
  if (!failed) return ''
  const other = form.value?.fields.find((f) => f.path === failed.path)
  return t(failed.not ? 'profileEditor.excludes' : 'profileEditor.needs', { name: other ? t(`profileEditor.field.${other.label}`) : failed.path })
}
</script>

<template>
  <template v-if="profile && form">
    <template v-for="card in cards" :key="card.id">
      <SettingsCard v-if="card.fields.length > 0" :title="card.title" :data-testid="`endpoint-card-${card.id}`">
        <p v-if="card.hint" class="mt-1.5 text-xs text-t3">{{ card.hint }}</p>
        <div class="grid grid-cols-2 items-start gap-x-4 gap-y-3 pt-3 pb-1">
          <ProfileField v-for="field in card.fields" :key="field.path" :field="field" :profile="profile" :reason="reason(field)" server-id="" />
        </div>
      </SettingsCard>
    </template>
  </template>
</template>
