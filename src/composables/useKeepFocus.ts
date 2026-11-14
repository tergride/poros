import { useEventListener } from '@vueuse/core'
import { type Ref, type WatchSource, nextTick, watch } from 'vue'

/**
 * Keeps the focus of the keyboard in a box whose button changes with a state, as Start turns into Cancel and Stop: the
 * old button leaves or turns disabled, and the focus would fall back to the page. Once the box had the focus and nothing
 * else took it since, the focus comes back to the first enabled button of the box.
 */
export function useKeepFocus(box: Ref<HTMLElement | null>, source: WatchSource): void {
  let inside = false
  useEventListener(document, 'focusin', (event) => {
    inside = !!box.value?.contains(event.target as Node)
  })
  watch(source, async () => {
    await nextTick()
    const focused = document.activeElement
    if (!inside || (focused && focused !== document.body)) return
    box.value?.querySelector<HTMLButtonElement>('button:not(:disabled)')?.focus()
  })
}
