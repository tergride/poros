import { type Ref, computed, ref, watch } from 'vue'

/** How a row was clicked: the keys that change what a click selects. */
export interface ClickKeys {
  ctrl?: boolean
  shift?: boolean
}

/**
 * Rows selected as in a list of Windows: a click selects one, Ctrl adds or takes away a row, Shift takes the range
 * from the last row clicked, Ctrl with Shift adds that range, Ctrl+A takes all. A right click outside the selection
 * selects that row; inside, it keeps the selection for the menu. ids are the rows shown, in their order; a row that
 * leaves them leaves the selection too.
 */
export function useSelection(ids: Ref<string[]>) {
  const selected = ref<string[]>([])
  const anchor = ref('')

  watch(ids, (shown) => {
    const present = new Set(shown)
    if (selected.value.some((id) => !present.has(id))) selected.value = selected.value.filter((id) => present.has(id))
    if (!present.has(anchor.value)) anchor.value = ''
  })

  function range(to: string): string[] {
    const list = ids.value
    const from = list.indexOf(anchor.value)
    const end = list.indexOf(to)
    if (from < 0 || end < 0) return [to]
    return list.slice(Math.min(from, end), Math.max(from, end) + 1)
  }

  function click(id: string, keys: ClickKeys = {}): void {
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
    }
  }

  function selectAll(): void {
    selected.value = [...ids.value]
  }

  function clear(): void {
    selected.value = []
    anchor.value = ''
  }

  // In the order of the rows, whatever order they were picked in.
  const ordered = computed(() => {
    const picked = new Set(selected.value)
    return ids.value.filter((id) => picked.has(id))
  })

  return { selected: ordered, click, contextOn, selectAll, clear, isSelected: (id: string) => selected.value.includes(id) }
}
