<script setup lang="ts">
import { Copy, SquarePen } from '@lucide/vue'
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'

import ProfileField from '@/components/adv/ProfileField.vue'
import { Banner, Button, Dialog, Select, Tabs, TabsContent, TabsList, TabsTrigger, Textarea } from '@/components/ui'
import { useProfileActions } from '@/composables/useProfileActions'
import { useProfileForms } from '@/composables/useProfileForms'
import { useReason } from '@/composables/useReason'
import { useRestartNeeded } from '@/composables/useRestartNeeded'
import { protocolName } from '@/lib/format'
import { cleanProfile, fieldState, getAt, holds, jsonErrorLine, securityOf } from '@/lib/profileForm'
import type { ServerCheck } from '@/platform/core-types'
import { CoreXray, MethodGroupsAdd, MethodServersCheck, MethodServersSave } from '@/platform/core-types'
import type { Field, FieldCondition, Profile } from '@/platform/profile-types'
import { FieldSecurity, TabAdvanced, TabCommon, TabMux, TabSecurity, TabTransport } from '@/platform/profile-types'
import { useCoreStore } from '@/stores/core'
import { useUiStore } from '@/stores/ui'

// The editor of one profile: a form the core describes, tab by tab, and the profile as JSON. The core checks the
// profile as it is typed, tells which of its cores would run it and gives its link.
const ui = useUiStore()
const core = useCoreStore()
const restart = useRestartNeeded()
const actions = useProfileActions()
const reason = useReason()
const { forms, load, formOf } = useProfileForms()
const { t } = useI18n()

type Draft = Record<string, unknown>
const draft = ref<Draft | null>(null)
const tab = ref<string>(TabCommon)
const failure = ref('')
const saving = ref(false)
const check = ref<ServerCheck | null>(null)
const jsonText = ref('')
const jsonError = ref('')

const copyOf = (value: unknown): Draft => JSON.parse(JSON.stringify(value)) as Draft

watch(
  () => ui.profileEdit,
  async (edit) => {
    if (!edit) return
    draft.value = copyOf(edit.profile)
    tab.value = TabCommon
    failure.value = jsonError.value = ''
    check.value = null
    try {
      await load()
    } catch (error) {
      failure.value = reason(error)
    }
  },
  { immediate: true },
)

const type = computed(() => String(draft.value?.type ?? ''))
const form = computed(() => (forms.value ? formOf(type.value) : undefined))
const tabOrder = [TabCommon, TabTransport, TabSecurity, TabMux, TabAdvanced]
const tabs = computed(() => [...tabOrder.filter((id) => form.value?.fields.some((f) => f.tab === id)), 'json'])

// Why an inactive field does not apply: the first condition it fails, named by the field that condition looks at.
function nameOf(condition: FieldCondition): string {
  const fields = form.value?.fields ?? []
  const security = fields.find(
    (f) => f.kind === FieldSecurity && (condition.path === `${f.path}.enabled` || condition.path === `${f.path}.reality.enabled`),
  )
  if (security) return condition.path === `${security.path}.reality.enabled` ? 'REALITY' : 'TLS'
  const field = fields.find((f) => f.path === condition.path)
  return field ? t(`profileEditor.field.${field.label}`) : condition.path
}
function reasonOf(field: Field, profile: Draft): string {
  if (fieldState(field, profile) !== 'inactive') return ''
  const failed = (field.when ?? []).find((c) => !c.in?.length && !holds(c, profile))
  if (!failed) return ''
  return t(failed.not ? 'profileEditor.excludes' : 'profileEditor.needs', { name: nameOf(failed) })
}
const shown = computed(() => {
  const profile = draft.value
  if (!profile || !form.value) return []
  return form.value.fields
    .map((field, index) => ({ field, index, state: fieldState(field, profile), reason: reasonOf(field, profile) }))
    .filter((f) => f.state !== 'hidden')
})
const fieldsOf = (id: string) => shown.value.filter((f) => f.field.tab === id)

// The type can change: the profile starts again from what «New profile» gives, with its name and address.
const compound = ['chain', 'custom', 'direct']
const types = computed(() => (forms.value?.forms ?? []).map((f) => ({ value: f.type, label: protocolName(f.type, t) })))
const typeChoice = computed({
  get: () => type.value,
  set: (next: string) => {
    const old = draft.value
    if (!old || next === type.value) return
    const start = forms.value?.new.find((n) => n.profile.type === next)?.profile ?? { type: next, [next]: {} }
    const fresh = copyOf(start)
    if (old.name) fresh.name = old.name
    if (!compound.includes(next) && !compound.includes(type.value)) {
      fresh.server = old.server
      fresh.server_port = old.server_port
    }
    draft.value = fresh
  },
})

// The chips of the title: the protocol with its security, and the core.
const securityName = computed(() => {
  const profile = draft.value
  if (!profile) return ''
  const field = form.value?.fields.find((f) => f.kind === FieldSecurity)
  const choice = field ? securityOf(profile, field.path) : getAt(profile, `${type.value}.tls.enabled`) ? 'tls' : ''
  if (getAt(profile, 'shadowsocks.shadowtls.version')) return 'ShadowTLS'
  return { tls: 'TLS', reality: 'REALITY' }[choice] ?? ''
})
const chip = computed(() => [protocolName(type.value, t), securityName.value].filter(Boolean).join(' · '))
const coreName = computed(() => (check.value ? (check.value.core === CoreXray ? 'Xray' : 'sing-box') : ''))

const clean = (): Profile | null => (draft.value ? cleanProfile(form.value, draft.value as unknown as Profile) : null)

// The core reads the profile once typing pauses; an answer to an older profile is dropped.
let asked = 0
let timer: ReturnType<typeof setTimeout> | undefined
watch(
  [draft, form],
  () => {
    clearTimeout(timer)
    if (!draft.value || !form.value) return
    timer = setTimeout(async () => {
      const profile = clean()
      if (!profile) return
      const n = ++asked
      try {
        const answer = await core.core().call(MethodServersCheck, { id: ui.profileEdit?.id ?? '', groupId: '', profile })
        if (n === asked) check.value = answer
      } catch {
        if (n === asked) check.value = null
      }
    }, 300)
  },
  { deep: true },
)

// The JSON tab shows the profile as the core stores it; what is typed there replaces it once it reads.
watch(tab, (next) => {
  if (next === 'json' && draft.value) {
    jsonText.value = JSON.stringify(draft.value, null, 2)
    jsonError.value = ''
  }
})
function onJson(text: string): void {
  jsonText.value = text
  let parsed: unknown
  try {
    parsed = JSON.parse(text)
  } catch (error) {
    const line = jsonErrorLine(text, error)
    jsonError.value = line ? t('profileEditor.jsonLine', { line }) : t('profileEditor.notJson')
    return
  }
  if (typeof parsed !== 'object' || parsed === null || Array.isArray(parsed) || typeof (parsed as Draft).type !== 'string') {
    jsonError.value = t('profileEditor.jsonNoType')
    return
  }
  jsonError.value = ''
  draft.value = parsed as Draft
}

async function save(): Promise<void> {
  const edit = ui.profileEdit
  const profile = clean()
  if (!edit || !profile) return
  saving.value = true
  failure.value = ''
  try {
    // A new profile for a group that does not exist yet makes the group first, once.
    if (!edit.id && !edit.target.groupId) {
      const group = await core.core().call(MethodGroupsAdd, { name: edit.target.newGroup.trim() || t('profiles.myServers') })
      edit.target.groupId = group.id
    }
    await core.core().call(MethodServersSave, { id: edit.id, groupId: edit.target.groupId, profile })
    if (edit.id) restart.serverSaved(edit.id)
    ui.profileEdit = null
  } catch (error) {
    failure.value = reason(error)
  } finally {
    saving.value = false
  }
}
</script>

<template>
  <Tabs v-model="tab">
    <Dialog
      :open="ui.profileEdit !== null"
      :title="t(ui.profileEdit?.id ? 'profileEditor.title' : 'profileEditor.newProfile')"
      class="h-[680px] w-[900px]"
      @update:open="(open) => !open && (ui.profileEdit = null)"
    >
      <template #icon><SquarePen /></template>
      <template #actions>
        <span class="inline-flex h-[22px] items-center rounded-md bg-s4 px-2 text-[11.5px] whitespace-nowrap text-t2" data-testid="profile-chip">{{ chip }}</span>
        <span
          v-if="coreName"
          class="inline-flex h-[22px] items-center rounded-md bg-s4 px-2 text-[11.5px] whitespace-nowrap text-t2"
          :title="t('profileEditor.coreHint', { core: coreName })"
          data-testid="profile-core"
        >
          {{ coreName }}
        </span>
      </template>
      <template #tabs>
        <!-- Only once the form is known: the first tab takes the focus of the dialog, and a focused tab opens. -->
        <TabsList v-if="form" class="px-3">
          <TabsTrigger v-for="id in tabs" :key="id" :value="id">{{ t(`profileEditor.tab.${id}`) }}</TabsTrigger>
        </TabsList>
      </template>
      <template v-if="draft && form">
        <TabsContent v-for="id in tabs.filter((x) => x !== 'json')" :key="id" :value="id" class="grid grid-cols-2 items-start gap-x-4 gap-y-3 pt-3">
          <template v-for="(f, i) in fieldsOf(id)" :key="`${type}-${f.index}`">
            <ProfileField :field="f.field" :profile="draft" :reason="f.reason" :server-id="ui.profileEdit?.id ?? ''" />
            <label v-if="id === 'common' && i === 0" class="grid min-w-0 content-start gap-1.5">
              <span class="text-xs text-t3">{{ t('profileEditor.type') }}</span>
              <Select v-model="typeChoice" :options="types" class="w-full" :aria-label="t('profileEditor.type')" data-testid="profile-type" />
            </label>
          </template>
        </TabsContent>
        <TabsContent value="json" class="grid gap-2 pt-3">
          <Textarea
            :model-value="jsonText"
            mono
            class="h-[440px] resize-none"
            spellcheck="false"
            :aria-label="t('profileEditor.tab.json')"
            data-testid="profile-json"
            @update:model-value="onJson"
          />
          <span v-if="jsonError" class="text-xs text-warn" data-testid="profile-json-error">{{ jsonError }}</span>
          <span v-else class="text-xs text-t3">{{ t('profileEditor.jsonHint') }}</span>
        </TabsContent>
      </template>
      <p v-if="check?.problem" class="mt-3 text-xs text-warn" data-testid="profile-problem">{{ t('profileEditor.problem', { problem: check.problem }) }}</p>
      <Banner v-if="failure" class="mt-3" data-testid="profile-failure">{{ failure }}</Banner>
      <template #footer>
        <Button variant="ghost" class="sp" :disabled="!check?.link" data-testid="profile-copy-link" @click="actions.copyText(check!.link)">
          <Copy />{{ t('profileEditor.copyLink') }}
        </Button>
        <Button variant="ghost" @click="ui.profileEdit = null">{{ t('common.cancel') }}</Button>
        <Button variant="primary" :disabled="saving || !draft || (tab === 'json' && !!jsonError)" data-testid="profile-save" @click="save()">
          {{ t('common.save') }}
        </Button>
      </template>
    </Dialog>
  </Tabs>
</template>
