import { describe, expect, it } from 'vitest'
import { ref } from 'vue'

import { useSelection } from './useSelection'

describe('rows selected as in a list of Windows', () => {
  it('takes one row, adds with Ctrl, takes ranges with Shift and adds them with both', () => {
    const ids = ref(['a', 'b', 'c', 'd', 'e'])
    const s = useSelection(ids)
    s.click('b')
    expect(s.selected.value).toEqual(['b'])
    s.click('d', { ctrl: true })
    expect(s.selected.value).toEqual(['b', 'd'])
    s.click('b', { ctrl: true })
    expect(s.selected.value).toEqual(['d'])
    // From the last row clicked, b, even though Ctrl took it away.
    s.click('a', { shift: true })
    expect(s.selected.value).toEqual(['a', 'b'])
    s.click('e', { ctrl: true })
    s.click('c', { ctrl: true, shift: true })
    expect(s.selected.value).toEqual(['a', 'b', 'c', 'd', 'e'])
    s.selectAll()
    s.click('c')
    expect(s.selected.value).toEqual(['c'])
  })

  it('keeps the selection for a right click inside it and takes the row outside it', () => {
    const ids = ref(['a', 'b', 'c'])
    const s = useSelection(ids)
    s.click('a')
    s.click('b', { ctrl: true })
    s.contextOn('b')
    expect(s.selected.value).toEqual(['a', 'b'])
    s.contextOn('c')
    expect(s.selected.value).toEqual(['c'])
  })

  it('lets go of rows that are no longer shown', async () => {
    const ids = ref(['a', 'b', 'c'])
    const s = useSelection(ids)
    s.selectAll()
    ids.value = ['a', 'c']
    await Promise.resolve()
    expect(s.selected.value).toEqual(['a', 'c'])
  })
})
