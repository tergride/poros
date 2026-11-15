<script setup lang="ts">
import { computed, nextTick, ref, useId, useTemplateRef } from 'vue'
import { useI18n } from 'vue-i18n'

import { Textarea } from '@/components/ui'
import { prefixAt, prefixesFor } from '@/lib/rules'

// The text of a tab of the lists window. While the line at the caret has no colon, a list under it offers the prefixes
// of the kinds and narrows to what is typed: the arrows choose, Enter or Tab put the prefix in, Esc hides the list.
defineOptions({ inheritAttrs: false })
const props = defineProps<{ kinds: string[] }>()
const model = defineModel<string>({ default: '' })
const { t } = useI18n()

const box = useTemplateRef<HTMLDivElement>('box')
const id = useId()
const shown = ref<string[]>([])
const current = ref(-1)
const place = ref<{ left: number; top?: number; bottom?: number; height: number }>({ left: 0, height: 0 })
// Where the prefix being typed starts; Esc hides the list until the next typing or click.
let from = -1
let hidden = false

const row = 30
const rows = 8
const width = 300
const gap = 3

const area = () => box.value?.querySelector('textarea') ?? null

function close(): void {
  shown.value = []
  current.value = -1
  from = -1
}

function refresh(): void {
  const el = area()
  const at = el && !hidden && document.activeElement === el && el.selectionStart === el.selectionEnd ? prefixAt(el.value, el.selectionStart) : null
  const fit = at ? prefixesFor(props.kinds, at.typed) : []
  const spot = el && at && fit.length > 0 ? spotOf(el, at.from, fit.length) : null
  if (!at || !spot) {
    close()
    return
  }
  const was = at.from === from ? shown.value[current.value] : undefined
  // Something typed picks the first prefix that fits; an empty line picks none, so that Enter still breaks the line.
  current.value = was && fit.includes(was) ? fit.indexOf(was) : at.typed ? 0 : -1
  shown.value = fit
  place.value = spot
  from = at.from
}

// Where the list goes for the prefix starting at a position: under its line, past the bottom of the field if need be.
// Over the line only when the window has no room for three rows under it. The list is placed in the box the field's
// wrapper is positioned against, the dialog, so that the scrolling body of the dialog does not cut it.
function spotOf(el: HTMLTextAreaElement, position: number, count: number): typeof place.value | null {
  const host = box.value?.offsetParent
  if (!(host instanceof HTMLElement)) return null
  const line = lineOf(el, position)
  if (line.top + line.height <= 0 || line.top >= el.clientHeight) return null
  const field = el.getBoundingClientRect()
  const frame = host.getBoundingClientRect()
  const x = field.left + el.clientLeft - frame.left - host.clientLeft
  const y = field.top + el.clientTop - frame.top - host.clientTop
  const lineTop = field.top + el.clientTop + line.top
  const full = Math.min(count, rows) * row + 10
  const below = window.innerHeight - lineTop - line.height - gap - 8
  const above = lineTop - gap - 8
  // The prefixes of the list stand under the typed text: 15px is the border and the padding of the list and its row.
  const left = x + Math.max(0, Math.min(line.left - 15, el.clientWidth - width))
  if (below >= Math.min(full, 3 * row + 10) || below >= above) {
    return { left, top: y + line.top + line.height + gap, height: Math.min(full, below) }
  }
  return { left, bottom: host.clientHeight - y - line.top + gap, height: Math.min(full, above) }
}

const copied = [
  'font-family',
  'font-size',
  'font-weight',
  'font-style',
  'font-feature-settings',
  'font-variant-ligatures',
  'letter-spacing',
  'word-spacing',
  'line-height',
  'tab-size',
  'text-indent',
  'text-transform',
  'padding-top',
  'padding-right',
  'padding-bottom',
  'padding-left',
  'border-top-width',
  'border-right-width',
  'border-bottom-width',
  'border-left-width',
]

// The line of a position of the text, inside the padding box of the field as it is scrolled, by a hidden copy of the
// field that wraps the lines the same way.
function lineOf(el: HTMLTextAreaElement, position: number): { left: number; top: number; height: number } {
  const style = getComputedStyle(el)
  const copy = document.createElement('div')
  for (const name of copied) copy.style.setProperty(name, style.getPropertyValue(name))
  const border = (side: 'Top' | 'Right' | 'Bottom' | 'Left') => parseFloat(style[`border${side}Width`]) || 0
  Object.assign(copy.style, {
    position: 'absolute',
    top: '0',
    left: '0',
    height: '0',
    overflow: 'hidden',
    visibility: 'hidden',
    boxSizing: 'border-box',
    borderStyle: 'solid',
    whiteSpace: 'pre-wrap',
    overflowWrap: 'break-word',
    width: `${el.clientWidth + border('Left') + border('Right')}px`,
  })
  copy.textContent = el.value.slice(0, position)
  const mark = document.createElement('span')
  mark.textContent = '\u200b'
  copy.append(mark)
  box.value?.append(copy)
  const height = parseFloat(style.lineHeight) || mark.offsetHeight
  const line = {
    left: mark.offsetLeft - el.scrollLeft,
    top: mark.offsetTop - (height - mark.offsetHeight) / 2 - el.scrollTop,
    height,
  }
  copy.remove()
  return line
}

function accept(kind: string): void {
  const el = area()
  if (!el || from < 0) return
  const text = `${kind}:`
  el.setSelectionRange(from, el.selectionEnd)
  // insertText keeps the prefix in the undo history of the field.
  if (!document.execCommand('insertText', false, text)) {
    el.setRangeText(text, from, el.selectionEnd, 'end')
    el.dispatchEvent(new Event('input', { bubbles: true }))
  }
  close()
}

function move(by: 1 | -1): void {
  const count = shown.value.length
  current.value = current.value < 0 ? (by > 0 ? 0 : count - 1) : (current.value + by + count) % count
  void nextTick(() => document.getElementById(`${id}-${current.value}`)?.scrollIntoView({ block: 'nearest' }))
}

function onKeydown(event: KeyboardEvent): void {
  if (shown.value.length === 0 || event.isComposing) return
  const kind = shown.value[current.value]
  const plain = !event.shiftKey && !event.ctrlKey && !event.altKey && !event.metaKey
  if (event.key === 'ArrowDown' || event.key === 'ArrowUp') move(event.key === 'ArrowDown' ? 1 : -1)
  else if ((event.key === 'Enter' || event.key === 'Tab') && plain && kind) accept(kind)
  else if (event.key === 'Escape') {
    // The list closes, not the window around it.
    event.stopPropagation()
    hidden = true
    close()
  } else return
  event.preventDefault()
}

function again(): void {
  hidden = false
  refresh()
}

// A click puts the caret after focus comes, so the list waits for it.
function focused(): void {
  window.setTimeout(again, 0)
}

const listStyle = computed(() => ({
  left: `${place.value.left}px`,
  top: place.value.top === undefined ? undefined : `${place.value.top}px`,
  bottom: place.value.bottom === undefined ? undefined : `${place.value.bottom}px`,
  maxHeight: `${place.value.height}px`,
  width: `${width}px`,
}))
</script>

<template>
  <!-- Not positioned: the list is placed against the dialog, see spotOf. -->
  <div ref="box" class="flex min-h-0 flex-1 flex-col">
    <Textarea
      v-bind="$attrs"
      v-model="model"
      mono
      class="min-h-0 flex-1 resize-none"
      spellcheck="false"
      aria-autocomplete="list"
      :aria-controls="shown.length > 0 ? id : undefined"
      :aria-activedescendant="current >= 0 ? `${id}-${current}` : undefined"
      @keydown="onKeydown"
      @keyup="refresh"
      @input="again"
      @click="again"
      @focus="focused"
      @blur="close"
      @scroll="refresh"
    />
    <div
      v-if="shown.length > 0"
      :id="id"
      role="listbox"
      :aria-label="t('presetLists.prefixes')"
      class="absolute z-10 overflow-auto rounded-[10px] border border-line2 bg-s3 p-1 shadow-[0_18px_44px_rgba(0,0,0,.55)]"
      :style="listStyle"
      data-testid="lists-prefixes"
      @mousedown.prevent
    >
      <div
        v-for="(kind, i) in shown"
        :id="`${id}-${i}`"
        :key="kind"
        role="option"
        :aria-selected="i === current"
        :class="[
          'flex h-[30px] cursor-pointer items-center gap-3 rounded-md px-2.5',
          i === current ? 'bg-s5' : 'hover:bg-s4',
        ]"
        data-testid="lists-prefix"
        @mousemove="current = i"
        @click="accept(kind)"
      >
        <span class="font-mono text-[12.5px] text-t1">{{ kind }}:</span>
        <span class="ml-auto truncate text-[11.5px] text-t3">{{ t(`rules.kinds.${kind}`) }}</span>
      </div>
    </div>
  </div>
</template>
