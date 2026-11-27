/** The seconds the speed chart spans; the store keeps one sample more, for both ends of the span. */
export const speedSeconds = 60

// The least top of the speed axis: a trickle of a few kilobytes a second lies low and reads as quiet.
const lowestTop = 32 * 1024
const units = [1024, 1024 ** 2, 1024 ** 3]
const niceSteps = [1, 2, 2.5, 5]
const mostSteps = 4

function stepFor(value: number): number {
  for (let power = 10 ** (Math.floor(Math.log10(value)) - 1); ; power *= 10) {
    const step = niceSteps.map((s) => s * power).find((s) => value / s <= mostSteps)
    if (step) return step
  }
}

/**
 * The marks of the speed axis in bytes a second: from 0 to a round value at or above the peak, at most four steps.
 * The steps are round in the unit the labels show, so each label is exact: 250 KB, 768 KB, 1,5 MB.
 */
export function speedTicks(peak: number): number[] {
  const top = Math.max(peak, lowestTop)
  let index = 0
  while (index < units.length - 1 && top >= units[index + 1]!) index++
  const scale = (unit: number) => {
    const step = stepFor(top / unit)
    return { unit, step, count: Math.ceil(top / unit / step - 1e-9) }
  }
  let axis = scale(units[index]!)
  // A top of a thousand units and more is written in the next unit: 768 KB and 1 MB, not 1 000 KB and 1,5 MB.
  if (axis.step * axis.count >= 1000 && index < units.length - 1) axis = scale(units[index + 1]!)
  return Array.from({ length: axis.count + 1 }, (_, i) => Math.round(i * axis.step * axis.unit))
}

/** The points of one speed line in a box of width × height, the newest sample at the right edge, one slot a second. */
export function speedPoints(values: number[], top: number, width: number, height: number): [number, number][] {
  const recent = values.slice(-(speedSeconds + 1))
  const first = speedSeconds + 1 - recent.length
  return recent.map((value, i) => [((first + i) * width) / speedSeconds, height - (Math.min(value, top) * height) / top])
}
