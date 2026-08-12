import assert from 'node:assert/strict'
import { afterEach, describe, mock, test } from 'node:test'

import { createImageObjectUrl, revokeImageObjectUrl } from './image-generation'

const pngBase64 = 'iVBORw0KGgo='

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

    assert.equal(createImageObjectUrl(pngBase64), 'blob:generated')
    assert.equal(createObjectURL.mock.calls.length, 1)
    const imageBlob = createObjectURL.mock.calls[0]?.arguments[0]
    assert.ok(imageBlob instanceof Blob)
    assert.equal(imageBlob.type, 'image/png')
    assert.equal(imageBlob.size, 8)
    assert.deepEqual(
      [...new Uint8Array(await imageBlob.arrayBuffer())],
      [137, 80, 78, 71, 13, 10, 26, 10]
    )
  })

  test('rejects URL-only, empty, non-PNG, and oversized responses', () => {
    const createObjectURL = mock.method(
      URL,
      'createObjectURL',
      () => 'blob:unexpected'
    )
    const oversizedPng = `${pngBase64}${'A'.repeat(28 * 1024 * 1024)}`

    for (const value of [
      '',
      'https://example.test/image.png',
      'aGVsbG8=',
      oversizedPng,
    ]) {
      assert.throws(() => createImageObjectUrl(value), /Invalid image response/)
    }
    assert.equal(createObjectURL.mock.calls.length, 0)
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
