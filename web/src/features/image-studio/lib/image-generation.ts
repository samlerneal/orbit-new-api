export const maxImageBytes = 20 * 1024 * 1024
const pngSignature = [137, 80, 78, 71, 13, 10, 26, 10]
const pngHeaderBytes = 33

export type PngDimensions = { height: number; width: number }

function pngCrc32(bytes: Uint8Array): number {
  let crc = 0xffffffff
  for (const byte of bytes) {
    crc ^= byte
    for (let bit = 0; bit < 8; bit += 1) {
      crc = (crc >>> 1) ^ (crc & 1 ? 0xedb88320 : 0)
    }
  }
  return (crc ^ 0xffffffff) >>> 0
}

function pngDimensionsFromHeader(bytes: Uint8Array): PngDimensions {
  if (
    bytes.length < pngHeaderBytes ||
    !pngSignature.every((byte, index) => bytes[index] === byte) ||
    bytes[8] !== 0 ||
    bytes[9] !== 0 ||
    bytes[10] !== 0 ||
    bytes[11] !== 13 ||
    bytes[12] !== 73 ||
    bytes[13] !== 72 ||
    bytes[14] !== 68 ||
    bytes[15] !== 82
  ) {
    throw new Error('Invalid image response')
  }
  const data = new DataView(bytes.buffer, bytes.byteOffset, bytes.byteLength)
  const width = data.getUint32(16)
  const height = data.getUint32(20)
  if (
    width === 0 ||
    height === 0 ||
    data.getUint32(29) !== pngCrc32(bytes.slice(12, 29))
  ) {
    throw new Error('Invalid image response')
  }
  return { height, width }
}

export function createImageBlob(base64: string): Blob {
  const maximumEncodedBytes = Math.ceil(maxImageBytes / 3) * 4
  if (
    !base64 ||
    base64.length > maximumEncodedBytes ||
    base64.length % 4 !== 0 ||
    /[\s]/.test(base64)
  ) {
    throw new Error('Invalid image response')
  }
  const binary = atob(base64)
  if (binary.length === 0 || binary.length > maxImageBytes) {
    throw new Error('Invalid image response')
  }
  const bytes = Uint8Array.from(binary, (character) => character.charCodeAt(0))
  pngDimensionsFromHeader(bytes)
  return new Blob([bytes], { type: 'image/png' })
}

export async function readPngDimensions(blob: Blob): Promise<PngDimensions> {
  if (
    blob.type !== 'image/png' ||
    blob.size < pngHeaderBytes ||
    blob.size > maxImageBytes
  ) {
    throw new Error('Invalid image response')
  }
  const bytes = new Uint8Array(
    await blob.slice(0, pngHeaderBytes).arrayBuffer()
  )
  return pngDimensionsFromHeader(bytes)
}

export async function decodePngDimensions(blob: Blob): Promise<PngDimensions> {
  const header = await readPngDimensions(blob)
  if (typeof createImageBitmap === 'function') {
    const bitmap = await createImageBitmap(blob)
    try {
      if (bitmap.width !== header.width || bitmap.height !== header.height) {
        throw new Error('Invalid image response')
      }
      return { height: bitmap.height, width: bitmap.width }
    } finally {
      bitmap.close()
    }
  }
  if (typeof Image === 'undefined') throw new Error('Invalid image response')
  const imageUrl = URL.createObjectURL(blob)
  try {
    const image = new Image()
    image.src = imageUrl
    await image.decode()
    if (
      image.naturalWidth !== header.width ||
      image.naturalHeight !== header.height
    ) {
      throw new Error('Invalid image response')
    }
    return { height: image.naturalHeight, width: image.naturalWidth }
  } finally {
    URL.revokeObjectURL(imageUrl)
  }
}

export function createImageObjectUrl(base64: string): string {
  return URL.createObjectURL(createImageBlob(base64))
}

export function revokeImageObjectUrl(imageUrl: string | null) {
  if (imageUrl) URL.revokeObjectURL(imageUrl)
}
