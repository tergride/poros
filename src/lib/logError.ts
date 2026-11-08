import { appControls, isCoreError } from '@/platform'

/** An error as the log keeps it: the core's code, words and reason, or the message of a script error. */
export function errorText(error: unknown): string {
  if (isCoreError(error)) {
    const reason = error.data?.reason ? ` (${error.data.reason})` : ''
    return `error ${error.code}${reason}: ${error.message}`
  }
  if (error instanceof Error) return `${error.name}: ${error.message}`
  return String(error)
}

/** Writes an error the interface met to the log; `where` says what the user was doing. */
export function logError(where: string, error: unknown): void {
  appControls.log(`${where}: ${errorText(error)}`)
}
