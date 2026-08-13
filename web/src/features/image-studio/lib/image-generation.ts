export const maxImageBytes = 20 * 1024 * 1024
const pngSignature = [137, 80, 78, 71, 13, 10, 26, 10]

export function createImageBlob(base64: string): Blob {
  if (!base64 || !/^[A-Za-z0-9+/]+={0,2}$/.test(base64)) {
    throw new Error('Invalid image response')
  }
  const binary = atob(base64)
  if (binary.length === 0 || binary.length > maxImageBytes) {
    throw new Error('Invalid image response')
  }
  const bytes = Uint8Array.from(binary, (character) => character.charCodeAt(0))
  if (!pngSignature.every((byte, index) => bytes[index] === byte)) {
    throw new Error('Invalid image response')
  }
  return new Blob([bytes], { type: 'image/png' })
}

export function createImageObjectUrl(base64: string): string {
  return URL.createObjectURL(createImageBlob(base64))
}

export function revokeImageObjectUrl(imageUrl: string | null) {
  if (imageUrl) URL.revokeObjectURL(imageUrl)
}
