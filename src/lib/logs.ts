/** How much a line of the log weighs: sing-box writes ERROR or WARN, Xray [Error] or [Warning], Poros neither. */
export type LogLevel = 'error' | 'warn' | 'info' | 'debug'

export function logLevel(line: string): LogLevel {
  if (/\b(ERROR|FATAL|PANIC|panic)\b|\[Error\]/.test(line)) return 'error'
  if (/\bWARN(ING)?\b|\[Warning\]/.test(line)) return 'warn'
  if (/\b(DEBUG|TRACE)\b|\[Debug\]/.test(line)) return 'debug'
  return 'info'
}

/** The level a filter asks for and the levels it lets through: each shows the heavier ones too, «All» debug as well. */
const shown: Record<string, LogLevel[]> = {
  all: ['error', 'warn', 'info', 'debug'],
  error: ['error'],
  warn: ['error', 'warn'],
  info: ['error', 'warn', 'info'],
}

export interface LogFilter {
  text: string
  matchCase: boolean
  regex: boolean
  level: string
}

/**
 * The lines a filter lets through. A regular expression that does not compile filters nothing out and says so, so
 * that the screen does not go blank while it is being typed.
 */
export function filterLog(lines: string[], filter: LogFilter): { lines: string[]; invalid: boolean } {
  const levels = shown[filter.level] ?? shown.all!
  let test: (line: string) => boolean = () => true
  let invalid = false
  const text = filter.text
  if (text) {
    if (filter.regex) {
      try {
        const pattern = new RegExp(text, filter.matchCase ? '' : 'i')
        test = (line) => pattern.test(line)
      } catch {
        invalid = true
      }
    } else if (filter.matchCase) {
      test = (line) => line.includes(text)
    } else {
      const lower = text.toLowerCase()
      test = (line) => line.toLowerCase().includes(lower)
    }
  }
  return { lines: lines.filter((line) => levels.includes(logLevel(line)) && test(line)), invalid }
}
