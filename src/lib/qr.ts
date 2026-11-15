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

/**
 * The code as a PNG to send in a message: scale pixels a module and four modules of quiet zone, dark on white as
 * readers expect, whatever the theme.
 */
export function qrPng(rows: string[], scale = 10): Promise<Blob> {
  const size = (rows.length + 8) * scale
  const canvas = document.createElement('canvas')
  canvas.width = size
  canvas.height = size
  const draw = canvas.getContext('2d')
  if (!draw) return Promise.reject(new Error('no canvas to draw the QR code on'))
  draw.fillStyle = '#ffffff'
  draw.fillRect(0, 0, size, size)
  draw.fillStyle = '#0b1020'
  rows.forEach((row, y) => {
    for (let x = 0; x < row.length; x++) if (row[x] === '1') draw.fillRect((x + 4) * scale, (y + 4) * scale, scale, scale)
  })
  return new Promise((resolve, reject) => canvas.toBlob((blob) => (blob ? resolve(blob) : reject(new Error('no PNG of the QR code'))), 'image/png'))
}
