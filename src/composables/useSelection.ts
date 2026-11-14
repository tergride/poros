import { type Ref, computed, ref, watch } from 'vue'

/** How a row was clicked: the keys that change what a click selects. */
export interface ClickKeys {
  ctrl?: boolean
  shift?: boolean
}

/**
 * Rows selected as in a list of Windows: a click selects one, Ctrl adds or takes away a row, Shift takes the range
 * from the last row clicked, Ctrl with Shift adds that range, Ctrl+A takes all. A right click outside the selection
 * selects that row; inside, it keeps the selection for the menu. The arrow keys move from the row last reached, Shift
 * with them takes the range from the last row clicked. ids are the rows shown, in their order; a row that leaves them
 * leaves the selection too.
 */
export function useSelection(ids: Ref<string[]>) {
  const selected = ref<string[]>([])
  const anchor = ref('')
  // The row the keys move from: the last one clicked or reached by a key.
  const lead = ref('')

  watch(ids, (shown) => {
    const present = new Set(shown)
    if (selected.value.some((id) => !present.has(id))) selected.value = selected.value.filter((id) => present.has(id))
    if (!present.has(anchor.value)) anchor.value = ''
    if (!present.has(lead.value)) lead.value = ''
  })

  function range(to: string): string[] {
    const list = ids.value
    const from = list.indexOf(anchor.value)
    const end = list.indexOf(to)
    if (from < 0 || end < 0) return [to]
    return list.slice(Math.min(from, end), Math.max(from, end) + 1)
  }

  function click(id: string, keys: ClickKeys = {}): void {
    lead.value = id
    if (keys.shift && anchor.value) {
      const span = range(id)
      selected.value = keys.ctrl ? [...new Set([...selected.value, ...span])] : span
      return
    }
    if (keys.ctrl) {
      selected.value = selected.value.includes(id) ? selected.value.filter((s) => s !== id) : [...selected.value, id]
    } else selected.value = [id]
    anchor.value = id
  }

  function contextOn(id: string): void {
    if (!selected.value.includes(id)) {
      selected.value = [id]
      anchor.value = id
      lead.value = id
    }
  }

  /** Moves by delta rows from the lead, Infinity for the last row and -Infinity for the first; the row it reaches. */
  function step(delta: number, keys: Pick<ClickKeys, 'shift'> = {}): string {
    const list = ids.value
    if (list.length === 0) return ''
    const from = list.indexOf(lead.value)
    const index = from < 0 ? (delta > 0 ? 0 : list.length - 1) : Math.min(list.length - 1, Math.max(0, from + delta))
    const to = list[index]!
    lead.value = to
    if (keys.shift && anchor.value) selected.value = range(to)
    else {
      selected.value = [to]
      anchor.value = to
    }
    return to
  }

  function selectAll(): void {
    selected.value = [...ids.value]
  }

  function clear(): void {
    selected.value = []
    anchor.value = ''
    lead.value = ''
  }

  // In the order of the rows, whatever order they were picked in.
  const ordered = computed(() => {
    const picked = new Set(selected.value)
    return ids.value.filter((id) => picked.has(id))
  })

  return { selected: ordered, click, contextOn, step, selectAll, clear, isSelected: (id: string) => selected.value.includes(id) }
}
