import { type Directive, type DirectiveBinding, reactive } from 'vue'

// One tooltip serves the whole window (TooltipHost): an element only carries its text in data-tooltip, so a list of
// thousands of rows has no tooltip of its own in each. It shows while the pointer rests on the element, at once when
// the keyboard focuses it, and goes on a press, on Escape and when what holds the element scrolls.

export const tooltipId = 'poros-tooltip'
const delay = 300
const gap = 6
const margin = 12

/** The tooltip on screen: the element it explains and its text; no target while none is shown. */
export const shown = reactive<{ target: HTMLElement | null; text: string }>({ target: null, text: '' })

let pending: ReturnType<typeof setTimeout> | undefined
// The element whose tooltip waits for the pointer to rest.
let waiting: HTMLElement | null = null
let closedAt = 0
let described: string | null = null
// The element whose tooltip a press or Escape sent away: it shows nothing more until the pointer leaves it.
let dismissed: HTMLElement | null = null

const triggerOf = (node: EventTarget | null) => (node instanceof Element ? node.closest<HTMLElement>('[data-tooltip]') : null)

/** Whether the text of an element is cut: by «…» on its line or by a clamp of lines. */
const isCut = (el: HTMLElement) => el.scrollWidth > el.clientWidth + 1 || el.scrollHeight > el.clientHeight + 1

function cancel(): void {
  clearTimeout(pending)
  waiting = null
}

function show(el: HTMLElement): void {
  cancel()
  const text = el.dataset.tooltip ?? ''
  if (!text || !el.isConnected || (el.hasAttribute('data-tooltip-cut') && !isCut(el))) {
    hide()
    return
  }
  if (shown.target !== el) {
    hide()
    described = el.getAttribute('aria-describedby')
    el.setAttribute('aria-describedby', described ? `${described} ${tooltipId}` : tooltipId)
  }
  shown.target = el
  shown.text = text
}

function hide(): void {
  cancel()
  const el = shown.target
  if (!el) return
  if (described === null) el.removeAttribute('aria-describedby')
  else el.setAttribute('aria-describedby', described)
  shown.target = null
  shown.text = ''
  closedAt = Date.now()
}

function soon(el: HTMLElement): void {
  // The pointer moving between the parts of one element keeps its wait.
  if (el === shown.target || el === waiting) return
  cancel()
  // From one tooltip to the next, or right after one went, the next shows at once.
  if (shown.target || Date.now() - closedAt < delay) show(el)
  else {
    waiting = el
    pending = setTimeout(() => show(el), delay)
  }
}

/** Follows the pointer and the focus over the document; gives back what stops it. */
export function listen(doc: Document): () => void {
  const over = (event: PointerEvent) => {
    if (event.pointerType === 'touch') return
    const el = triggerOf(event.target)
    if (el !== dismissed) dismissed = null
    if (!el) hide()
    else if (el !== dismissed) soon(el)
  }
  const out = (event: PointerEvent) => {
    if (event.relatedTarget) return
    dismissed = null
    hide()
  }
  const down = (event: PointerEvent) => {
    dismissed = triggerOf(event.target)
    hide()
  }
  const focusIn = (event: FocusEvent) => {
    const el = triggerOf(event.target)
    if (el && event.target instanceof Element && event.target.matches(':focus-visible')) show(el)
  }
  const focusOut = (event: FocusEvent) => {
    const el = shown.target
    if (el && el.contains(event.target as Node) && !el.contains(event.relatedTarget as Node | null)) hide()
  }
  const key = (event: KeyboardEvent) => {
    if (event.key !== 'Escape') return
    dismissed = shown.target ?? waiting
    hide()
  }
  const scroll = (event: Event) => {
    if (shown.target && event.target instanceof Node && event.target.contains(shown.target)) hide()
  }
  doc.addEventListener('pointerover', over)
  doc.addEventListener('pointerout', out)
  doc.addEventListener('pointerdown', down, true)
  doc.addEventListener('focusin', focusIn)
  doc.addEventListener('focusout', focusOut)
  doc.addEventListener('keydown', key, true)
  doc.addEventListener('scroll', scroll, true)
  doc.defaultView?.addEventListener('blur', hide)
  return () => {
    doc.removeEventListener('pointerover', over)
    doc.removeEventListener('pointerout', out)
    doc.removeEventListener('pointerdown', down, true)
    doc.removeEventListener('focusin', focusIn)
    doc.removeEventListener('focusout', focusOut)
    doc.removeEventListener('keydown', key, true)
    doc.removeEventListener('scroll', scroll, true)
    doc.defaultView?.removeEventListener('blur', hide)
    hide()
    closedAt = 0
    dismissed = null
  }
}

/**
 * Where the tooltip goes: under the element, or over it when the window ends below; along its start, or its end with
 * end; inside the window by 12 px.
 */
export function position(
  target: Pick<DOMRect, 'left' | 'right' | 'top' | 'bottom'>,
  box: Pick<DOMRect, 'width' | 'height'>,
  end: boolean,
  view: { width: number; height: number },
): { left: number; top: number } {
  let top = target.bottom + gap
  if (top + box.height > view.height - margin && target.top - gap - box.height >= margin) top = target.top - gap - box.height
  const left = end ? target.right - box.width : target.left
  return { left: Math.max(margin, Math.min(left, view.width - margin - box.width)), top }
}

function write(el: HTMLElement, { value, modifiers }: DirectiveBinding<string | null | undefined, 'cut' | 'end'>): void {
  if (value) el.dataset.tooltip = value
  else delete el.dataset.tooltip
  el.toggleAttribute('data-tooltip-cut', !!modifiers.cut)
  el.toggleAttribute('data-tooltip-end', !!modifiers.end)
  if (el !== shown.target) return
  if (value) shown.text = value
  else hide()
}

/**
 * v-tooltip="text" explains an element in the tooltip of the window. .cut shows the whole text of an element only while
 * it is cut; .end lines the tooltip up with the element's end. An empty text shows nothing.
 */
export const vTooltip: Directive<HTMLElement, string | null | undefined, 'cut' | 'end'> = {
  mounted: write,
  updated: write,
  beforeUnmount(el) {
    if (el === shown.target || el === waiting) hide()
    if (el === dismissed) dismissed = null
  },
}
