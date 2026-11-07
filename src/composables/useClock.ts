import { onScopeDispose, ref } from 'vue'

/** The current time in milliseconds, ticking every interval while the component lives. */
export function useClock(interval: number) {
  const now = ref(Date.now())
  const timer = setInterval(() => (now.value = Date.now()), interval)
  onScopeDispose(() => clearInterval(timer))
  return now
}
