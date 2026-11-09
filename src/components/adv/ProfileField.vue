<script setup lang="ts">
import { ArrowDown, ArrowUp, Eye, EyeOff, X } from '@lucide/vue'
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'

import { Button, Input, Select, Switch, Textarea } from '@/components/ui'
import { serverTitle } from '@/lib/format'
import { getAt, headersText, parseBytes, parseHeaders, parseList, securityOf, setAt, setSecurity } from '@/lib/profileForm'
import type { Field } from '@/platform/profile-types'
import { FieldServers } from '@/platform/profile-types'
import { useCoreStore } from '@/stores/core'

// One field of the profile editor, drawn by its kind; it writes straight into the profile. An inactive field shows
// its value greyed out with why it does not apply.
const props = defineProps<{ field: Field; profile: Record<string, unknown>; reason: string; serverId: string }>()

const core = useCoreStore()
const { t, te } = useI18n()

const kind = computed(() => props.field.kind)
const inactive = computed(() => props.reason !== '')
const label = computed(() => t(`profileEditor.field.${props.field.label}`))
const hint = computed(() => {
  const key = `profileEditor.hint.${props.field.label}`
  return te(key) ? t(key) : ''
})
const value = computed(() => getAt(props.profile, props.field.path))
const put = (next: unknown) => setAt(props.profile, props.field.path, next)

// A select, a uTLS fingerprint or the choice of TLS: "" is none or the core's default.
function optionLabel(option: string): string {
  if (option === '') {
    const key = `profileEditor.empty.${props.field.label}`
    return te(key) ? t(key) : t('profileEditor.none')
  }
  if (!/^[\w-]+$/.test(option)) return option
  const own = `profileEditor.choice.${props.field.label}.${option}`
  const common = `profileEditor.option.${option}`
  return te(own) ? t(own) : te(common) ? t(common) : option
}
const options = computed(() =>
  (props.field.options ?? []).map((option) => ({ value: props.field.numeric ? Number(option) : option, label: optionLabel(option) })),
)
const choice = computed<string | number>({
  get: () => {
    if (kind.value === 'security') return securityOf(props.profile, props.field.path)
    if (kind.value === 'utls') return String(getAt(props.profile, `${props.field.path}.fingerprint`) ?? '')
    return props.field.numeric ? Number(value.value ?? 0) : String(value.value ?? '')
  },
  set: (next) => {
    if (kind.value === 'security') setSecurity(props.profile, props.field.path, String(next))
    else if (kind.value === 'utls') put(next ? { enabled: true, fingerprint: next } : undefined)
    else put(next)
  },
})

const on = computed({ get: () => value.value === true, set: (next: boolean) => put(next) })

// Fields typed as text keep what is typed, even while it is not a value yet, as "h2," on the way to two items.
function format(v: unknown): string {
  switch (kind.value) {
    case 'list':
    case 'bytes':
      return Array.isArray(v) ? v.join(', ') : ''
    case 'headers':
      return headersText(v)
    case 'json':
      return v === undefined ? '' : JSON.stringify(v, null, 2)
    case 'number':
      return typeof v === 'number' && v !== 0 ? String(v) : ''
    default:
      return typeof v === 'string' ? v : ''
  }
}
function parse(input: string): { ok: boolean; value: unknown } {
  const empty = input.trim() === ''
  switch (kind.value) {
    case 'list':
      return { ok: true, value: parseList(input) }
    case 'headers':
      return { ok: true, value: parseHeaders(input) }
    case 'bytes': {
      const bytes = parseBytes(input)
      return { ok: bytes !== undefined, value: bytes }
    }
    case 'number': {
      const n = Number(input)
      return empty ? { ok: true, value: undefined } : { ok: Number.isInteger(n) && n >= 0, value: n }
    }
    case 'json':
      if (empty) return { ok: true, value: undefined }
      try {
        return { ok: true, value: JSON.parse(input) as unknown }
      } catch {
        return { ok: false, value: undefined }
      }
    default:
      return { ok: true, value: input }
  }
}
const same = (a: unknown, b: unknown) => JSON.stringify(a ?? null) === JSON.stringify(b ?? null)
const text = ref(format(value.value))
const invalid = ref(false)
watch(value, (next) => {
  const typed = parse(text.value)
  if (!typed.ok || !same(typed.value, next)) {
    text.value = format(next)
    invalid.value = false
  }
})
function onText(input: string): void {
  text.value = input
  const typed = parse(input)
  invalid.value = !typed.ok
  if (typed.ok) put(typed.value)
}
const shown = ref(false)

// The servers of a chain, in order, picked from the other servers.
const chosen = computed(() => (Array.isArray(value.value) ? (value.value as string[]) : []))
const nameOf = (id: string) => {
  const s = core.servers.find((server) => server.id === id)
  if (!s) return t('profileEditor.missingServer')
  return `${serverTitle(s.name, s.country)} · ${core.groups.find((g) => g.id === s.groupId)?.name ?? ''}`
}
const candidates = computed(() => [
  { value: '', label: t('profileEditor.addServer') },
  ...core.servers
    .filter((s) => s.id !== props.serverId && s.protocol !== 'chain' && s.protocol !== 'direct' && !chosen.value.includes(s.id))
    .map((s) => ({ value: s.id, label: nameOf(s.id) })),
])
const adding = ref('')
watch(adding, (id) => {
  if (!id) return
  put([...chosen.value, id])
  adding.value = ''
})
function moveServer(index: number, by: number): void {
  const list = [...chosen.value]
  const [id] = list.splice(index, 1)
  list.splice(index + by, 0, id!)
  put(list)
}

const wide = computed(() => ['lines', 'json', 'headers', FieldServers].includes(kind.value))
</script>

<template>
  <label v-if="kind === 'bool'" :class="['flex min-h-9 cursor-pointer items-center gap-2.5', inactive && 'opacity-55']" :data-field="field.path">
    <Switch v-model="on" :disabled="inactive" :aria-label="label" />
    <span class="grid gap-0.5">
      <span>{{ label }}</span>
      <span v-if="reason || hint" class="text-xs text-t3">{{ reason || hint }}</span>
    </span>
  </label>
  <div v-else :class="['grid min-w-0 content-start gap-1.5', wide && 'col-span-2', inactive && 'opacity-55']" :data-field="field.path">
    <span class="text-xs text-t3">{{ label }}</span>
    <Select
      v-if="kind === 'select' || kind === 'utls' || kind === 'security'"
      v-model="choice"
      :options="options"
      :disabled="inactive"
      class="w-full"
      :aria-label="label"
    />
    <Textarea
      v-else-if="kind === 'lines' || kind === 'json' || kind === 'headers'"
      :model-value="kind === 'lines' ? String(value ?? '') : text"
      mono
      :class="kind === 'json' ? 'h-[180px]' : 'h-[96px]'"
      :disabled="inactive"
      :aria-label="label"
      spellcheck="false"
      @update:model-value="(v: string) => (kind === 'lines' ? put(v) : onText(v))"
    />
    <div v-else-if="kind === 'servers'" class="grid gap-1.5">
      <div v-for="(id, i) in chosen" :key="id" class="flex items-center gap-1 rounded-lg border border-line bg-s1 py-1 pr-1 pl-2.5" data-testid="chain-server">
        <span class="w-5 text-xs text-t3">{{ i + 1 }}</span>
        <span class="min-w-0 flex-1 truncate">{{ nameOf(id) }}</span>
        <Button variant="ghost" size="icon-sm" :disabled="i === 0" :aria-label="t('profileEditor.up')" @click="moveServer(i, -1)"><ArrowUp /></Button>
        <Button variant="ghost" size="icon-sm" :disabled="i === chosen.length - 1" :aria-label="t('profileEditor.down')" @click="moveServer(i, 1)"><ArrowDown /></Button>
        <Button variant="ghost" size="icon-sm" :aria-label="t('common.remove')" @click="put(chosen.filter((c) => c !== id))"><X /></Button>
      </div>
      <Select v-model="adding" :options="candidates" class="w-full" :aria-label="t('profileEditor.addServer')" data-testid="chain-add" />
    </div>
    <div v-else-if="kind === 'secret'" class="relative">
      <Input
        :model-value="text"
        :type="shown ? 'text' : 'password'"
        mono
        class="pr-9"
        :disabled="inactive"
        :aria-label="label"
        autocomplete="off"
        spellcheck="false"
        @update:model-value="onText"
      />
      <button
        type="button"
        class="absolute top-0 right-0 grid h-9 w-9 cursor-pointer place-items-center text-t3 hover:text-t1 [&_svg]:size-4"
        :aria-label="t(shown ? 'profileEditor.hide' : 'profileEditor.show')"
        @click="shown = !shown"
      >
        <EyeOff v-if="shown" /><Eye v-else />
      </button>
    </div>
    <Input
      v-else
      :model-value="text"
      :mono="field.path !== 'name'"
      :inputmode="kind === 'number' ? 'numeric' : undefined"
      :disabled="inactive"
      :aria-label="label"
      spellcheck="false"
      @update:model-value="onText"
    />
    <span v-if="invalid" class="text-xs text-warn">{{ t(kind === 'json' ? 'profileEditor.notJson' : 'profileEditor.notValue') }}</span>
    <span v-else-if="reason || hint" class="text-xs text-t3">{{ reason || hint }}</span>
  </div>
</template>
