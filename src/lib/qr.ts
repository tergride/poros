/**
 * The path of the dark modules of a QR code, a row of '1' and '0' each: one rectangle a run of modules in a row, in a
 * viewBox of one unit a module.
 */
export function qrPath(rows: string[]): string {
  let d = ''
  rows.forEach((row, y) => {
    for (let x = 0; x < row.length; ) {
      if (row[x] !== '1') {
        x++
        continue
      }
      let end = x
      while (row[end] === '1') end++
      d += `M${x} ${y}h${end - x}v1h-${end - x}z`
      x = end
    }
  })
  return d
}
