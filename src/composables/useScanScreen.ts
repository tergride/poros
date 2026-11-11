import { appControls } from '@/platform'
import { MethodQRScan } from '@/platform/core-types'
import { useCoreStore } from '@/stores/core'

/**
 * Reads the QR codes on the screen. The window keeps itself out of the snapshot meanwhile instead of hiding: it stays
 * where it is, and what it covers is read.
 */
export function useScanScreen() {
  const core = useCoreStore()
  return async (): Promise<string[]> => {
    await appControls.excludeFromCapture(true).catch(() => undefined)
    try {
      // The compositor takes the window out with its next frame.
      await new Promise((resolve) => setTimeout(resolve, 150))
      return (await core.core().call(MethodQRScan)).texts
    } finally {
      await appControls.excludeFromCapture(false).catch(() => undefined)
    }
  }
}
