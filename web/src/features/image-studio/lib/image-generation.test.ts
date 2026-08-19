import assert from 'node:assert/strict'
import { afterEach, describe, mock, test } from 'node:test'
import { deflateSync } from 'node:zlib'

import {
  createImageBlob,
  createImageObjectUrl,
  decodePngDimensions,
  maxImageBytes,
  readPngDimensions,
  revokeImageObjectUrl,
} from './image-generation'

function crc32(bytes: Uint8Array): number {
  let crc = 0xffffffff
  for (const byte of bytes) {
    crc ^= byte
    for (let bit = 0; bit < 8; bit += 1) {
      crc = (crc >>> 1) ^ (crc & 1 ? 0xedb88320 : 0)
    }
  }
  return (crc ^ 0xffffffff) >>> 0
}

function chunk(type: string, data: Uint8Array) {
  const bytes = new Uint8Array(data.length + 12)
  new DataView(bytes.buffer).setUint32(0, data.length)
  bytes.set(
    [...type].map((character) => character.charCodeAt(0)),
    4
  )
  bytes.set(data, 8)
  new DataView(bytes.buffer).setUint32(
    8 + data.length,
    crc32(bytes.slice(4, 8 + data.length))
  )
  return bytes
}

function pngBase64(width: number, height: number) {
  const header = new Uint8Array(13)
  const view = new DataView(header.buffer)
  view.setUint32(0, width)
  view.setUint32(4, height)
  header[8] = 8
  header[9] = 6
  const raw = new Uint8Array((width * 4 + 1) * height)
  const bytes = [
    Uint8Array.of(137, 80, 78, 71, 13, 10, 26, 10),
    chunk('IHDR', header),
    chunk('IDAT', deflateSync(raw)),
    chunk('IEND', new Uint8Array()),
  ]
  return Buffer.from(
    Buffer.concat(bytes.map((value) => Buffer.from(value)))
  ).toString('base64')
}

const squarePngBase64 = pngBase64(1024, 1024)

afterEach(() => {
  mock.restoreAll()
})

describe('image generation result parsing', () => {
  test('turns a PNG base64 response into a PNG Blob object URL', async () => {
    const createObjectURL = mock.method(
      URL,
      'createObjectURL',
      () => 'blob:generated'
    )

    assert.equal(createImageObjectUrl(squarePngBase64), 'blob:generated')
    assert.equal(createObjectURL.mock.calls.length, 1)
    const imageBlob = createObjectURL.mock.calls[0]?.arguments[0]
    assert.ok(imageBlob instanceof Blob)
    assert.equal(imageBlob.type, 'image/png')
    assert.equal(imageBlob.size, 4144)
    assert.deepEqual(
      [...new Uint8Array(await imageBlob.slice(0, 33).arrayBuffer())],
      [
        137, 80, 78, 71, 13, 10, 26, 10, 0, 0, 0, 13, 73, 72, 68, 82, 0, 0, 4,
        0, 0, 0, 4, 0, 8, 6, 0, 0, 0, 127, 29, 43, 131,
      ]
    )
  })

  test('rejects URL-only, empty, non-PNG, and oversized responses', () => {
    const createObjectURL = mock.method(
      URL,
      'createObjectURL',
      () => 'blob:unexpected'
    )
    const oversizedPng = `${squarePngBase64}${'A'.repeat(28 * 1024 * 1024)}`

    for (const value of [
      '',
      'https://example.test/image.png',
      'aGVsbG8=',
      `${squarePngBase64}\r\n`,
      oversizedPng,
    ]) {
      assert.throws(() => createImageObjectUrl(value), /Invalid image response/)
    }
    assert.equal(createObjectURL.mock.calls.length, 0)
  })

  test('accepts exactly twenty MiB with base64 padding and rejects one byte more', () => {
    const exact = new Uint8Array(maxImageBytes)
    exact.set(new Uint8Array(Buffer.from(squarePngBase64, 'base64')))
    const exactBase64 = Buffer.from(exact).toString('base64')
    assert.match(exactBase64, /=$/)
    assert.equal(createImageBlob(exactBase64).size, maxImageBytes)

    const overLimit = new Uint8Array(maxImageBytes + 1)
    overLimit.set(exact)
    assert.throws(
      () => createImageBlob(Buffer.from(overLimit).toString('base64')),
      /Invalid image response/
    )
  })

  test('reads a complete IHDR and rejects truncated or corrupt headers', async () => {
    assert.deepEqual(
      await readPngDimensions(createImageBlob(squarePngBase64)),
      {
        height: 1024,
        width: 1024,
      }
    )
    await assert.rejects(
      () =>
        readPngDimensions(
          new Blob([new Uint8Array([137, 80, 78, 71, 13, 10, 26, 10])], {
            type: 'image/png',
          })
        ),
      /Invalid image response/
    )
    const completeHeader = new Uint8Array(
      await createImageBlob(squarePngBase64).arrayBuffer()
    )
    await assert.rejects(
      () =>
        readPngDimensions(
          new Blob([completeHeader.slice(0, 24)], { type: 'image/png' })
        ),
      /Invalid image response/
    )
    const corrupt = [...completeHeader]
    corrupt[32] ^= 1
    assert.throws(
      () => createImageBlob(btoa(String.fromCharCode(...corrupt))),
      /Invalid image response/
    )
  })

  test('rejects a header-only PNG when browser decoding fails', async () => {
    const headerOnly = createImageBlob(
      squarePngBase64.slice(0, Math.ceil(33 / 3) * 4)
    )
    const original = globalThis.createImageBitmap
    globalThis.createImageBitmap = async () => {
      throw new Error('decode failed')
    }
    try {
      await assert.rejects(
        () => decodePngDimensions(headerOnly),
        /decode failed/
      )
    } finally {
      globalThis.createImageBitmap = original
    }
  })

  test('revokes the fallback URL when Image.decode rejects', async () => {
    const originalBitmap = globalThis.createImageBitmap
    const originalImage = globalThis.Image
    const revoked: string[] = []
    globalThis.createImageBitmap =
      undefined as unknown as typeof createImageBitmap
    globalThis.Image = class {
      decode() {
        return Promise.reject(new Error('decode failed'))
      }
      set src(_value: string) {}
    } as unknown as typeof Image
    mock.method(URL, 'createObjectURL', () => 'blob:fallback')
    mock.method(URL, 'revokeObjectURL', (url: string) => revoked.push(url))
    try {
      await assert.rejects(
        () => decodePngDimensions(createImageBlob(squarePngBase64)),
        /decode failed/
      )
      assert.deepEqual(revoked, ['blob:fallback'])
    } finally {
      globalThis.createImageBitmap = originalBitmap
      globalThis.Image = originalImage
    }
  })

  test('releases each replaced object URL exactly once', () => {
    const revoked: string[] = []
    const revokeObjectURL = mock.method(
      URL,
      'revokeObjectURL',
      (url: string) => {
        revoked.push(url)
      }
    )

    revokeImageObjectUrl('blob:first')
    revokeImageObjectUrl(null)
    revokeImageObjectUrl('blob:replacement')

    assert.deepEqual(revoked, ['blob:first', 'blob:replacement'])
    assert.equal(revokeObjectURL.mock.calls.length, 2)
  })
})
