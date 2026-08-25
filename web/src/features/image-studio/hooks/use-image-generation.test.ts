import assert from 'node:assert/strict'
import { afterEach, beforeEach, describe, mock, test } from 'node:test'
import { deflateSync } from 'node:zlib'

import type {
  ImageAspect,
  ImageGenerationResponse,
  ImageGenerationState,
} from '../types'
import {
  createImageGenerationLifecycle,
  type ImageGenerationRequest,
} from './use-image-generation'

function crc32(bytes: Uint8Array) {
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
  return Buffer.concat([
    Buffer.from([137, 80, 78, 71, 13, 10, 26, 10]),
    Buffer.from(chunk('IHDR', header)),
    Buffer.from(chunk('IDAT', deflateSync(raw))),
    Buffer.from(chunk('IEND', new Uint8Array())),
  ]).toString('base64')
}
const squarePngBase64 = pngBase64(1024, 1024)
const portraitPngBase64 = pngBase64(1024, 1536)
const landscapePngBase64 = pngBase64(1536, 1024)

function deferred<T>() {
  let resolve!: (value: T) => void
  let reject!: (reason?: unknown) => void
  const promise = new Promise<T>((promiseResolve, promiseReject) => {
    resolve = promiseResolve
    reject = promiseReject
  })
  return { promise, reject, resolve }
}

const originalCreateImageBitmap = globalThis.createImageBitmap

afterEach(() => {
  mock.restoreAll()
  if (originalCreateImageBitmap === undefined) {
    delete (globalThis as { createImageBitmap?: typeof createImageBitmap })
      .createImageBitmap
  } else {
    globalThis.createImageBitmap = originalCreateImageBitmap
  }
})

beforeEach(() => {
  ;(
    globalThis as typeof globalThis & {
      createImageBitmap: typeof createImageBitmap
    }
  ).createImageBitmap = async (blob) => {
    const bytes = new Uint8Array(
      await (blob as Blob).slice(0, 33).arrayBuffer()
    )
    const view = new DataView(bytes.buffer)
    return {
      close: () => undefined,
      height: view.getUint32(20),
      width: view.getUint32(16),
    } as ImageBitmap
  }
})

describe('image generation request lifecycle', () => {
  test('sends one request through repeated generate calls', async () => {
    const pending = deferred<ImageGenerationResponse>()
    let requests = 0
    const aspects: string[] = []
    const states: ImageGenerationState[] = []
    const lifecycle = createImageGenerationLifecycle(
      (_prompt, aspect) => {
        requests += 1
        aspects.push(aspect)
        return pending.promise
      },
      (state) => states.push(state)
    )

    void lifecycle.generate('first', 'landscape')
    void lifecycle.generate('second', 'portrait')
    assert.equal(requests, 1)
    assert.deepEqual(aspects, ['landscape'])
    assert.equal(lifecycle.getState().status, 'generating')

    pending.reject(new Error('fake failure'))
    await Promise.resolve()
    assert.equal(lifecycle.getState().status, 'error')
    assert.equal(requests, 1)
    assert.equal(states.length, 2)
  })

  test('does not retry after success, failure, or stopping the wait', async () => {
    const responses = [
      Promise.resolve({ data: [{ b64_json: squarePngBase64 }] }),
      Promise.reject(new Error('fake failure')),
      deferred<ImageGenerationResponse>(),
    ]
    let requests = 0
    mock.method(URL, 'createObjectURL', () => 'blob:generated')
    const lifecycle = createImageGenerationLifecycle(
      (_prompt, _aspect, signal) => {
        const response = responses[requests++]
        assert.ok(response)
        if ('promise' in response) return response.promise
        assert.equal(signal.aborted, false)
        return response
      },
      () => undefined
    )

    await lifecycle.generate('success', 'square')
    assert.equal(lifecycle.getState().status, 'success')
    assert.equal(lifecycle.getState().imageUrl, 'blob:generated')
    await lifecycle.generate('failure', 'square')
    assert.equal(lifecycle.getState().status, 'error')
    void lifecycle.generate('stop', 'square')
    lifecycle.stopWaiting()
    assert.equal(lifecycle.getState().status, 'error')
    assert.equal(requests, 3)
    const pending = responses[2]
    assert.ok('reject' in pending)
    pending.reject(new DOMException('aborted', 'AbortError'))
    await Promise.resolve()
    assert.equal(requests, 3)
  })

  test('rejects a valid PNG whose IHDR does not match the request snapshot', async () => {
    const mismatched = squarePngBase64
    mock.method(URL, 'createObjectURL', () => 'blob:unexpected')
    const lifecycle = createImageGenerationLifecycle(
      () => Promise.resolve({ data: [{ b64_json: mismatched }] }),
      () => undefined
    )
    await lifecycle.generate('fixture', 'landscape')
    assert.equal(lifecycle.getState().status, 'error')
  })

  test('accepts each native preset only when its PNG matches the request snapshot', async () => {
    const fixtures: Record<ImageAspect, string> = {
      landscape: landscapePngBase64,
      portrait: portraitPngBase64,
      square: squarePngBase64,
      xiaohongshu: pngBase64(1056, 1408),
    }
    mock.method(URL, 'createObjectURL', () => 'blob:generated')
    const lifecycle = createImageGenerationLifecycle(
      (_prompt, aspect) =>
        Promise.resolve({ data: [{ b64_json: fixtures[aspect] }] }),
      () => undefined
    )

    for (const aspect of ['square', 'landscape', 'portrait'] as const) {
      await lifecycle.generate('fixture', aspect)
      assert.equal(lifecycle.getState().status, 'success')
    }
  })

  test('persists only the single decoded blob from a successful response', async () => {
    const successfulImages: Array<{ generationId: string; prompt: string }> = []
    mock.method(URL, 'createObjectURL', () => 'blob:generated')
    mock.method(crypto, 'randomUUID', () => 'fixed-generation-id')
    const lifecycle = createImageGenerationLifecycle(
      (_prompt, aspect) =>
        Promise.resolve({
          data: [
            {
              b64_json:
                aspect === 'portrait' ? portraitPngBase64 : squarePngBase64,
            },
          ],
        }),
      () => undefined,
      (image, prompt) => {
        successfulImages.push({ generationId: image.generationId, prompt })
        assert.equal(image.blob.type, 'image/png')
        assert.equal(image.imageUrl, 'blob:generated')
      }
    )

    await lifecycle.generate('A local fixture', 'portrait')

    assert.deepEqual(successfulImages, [
      { generationId: 'fixed-generation-id', prompt: 'A local fixture' },
    ])
  })

  test('revokes replaced and disposed image object URLs', async () => {
    const revoked: string[] = []
    let created = 0
    mock.method(URL, 'createObjectURL', () => `blob:${++created}`)
    mock.method(URL, 'revokeObjectURL', (url: string) => revoked.push(url))
    const requestImage: ImageGenerationRequest = (_prompt, aspect) =>
      Promise.resolve({
        data: [
          {
            b64_json:
              aspect === 'landscape' ? landscapePngBase64 : squarePngBase64,
          },
        ],
      })
    const lifecycle = createImageGenerationLifecycle(
      requestImage,
      () => undefined
    )

    await lifecycle.generate('first', 'square')
    await lifecycle.generate('replacement', 'landscape')
    assert.deepEqual(revoked, ['blob:1'])
    lifecycle.dispose()
    assert.deepEqual(revoked, ['blob:1', 'blob:2'])
  })

  test('revokes the current URL exactly once when its owner becomes invalid', async () => {
    const revoked: string[] = []
    mock.method(URL, 'createObjectURL', () => 'blob:current')
    mock.method(URL, 'revokeObjectURL', (url: string) => revoked.push(url))
    const lifecycle = createImageGenerationLifecycle(
      () => Promise.resolve({ data: [{ b64_json: squarePngBase64 }] }),
      () => undefined
    )

    await lifecycle.generate('local fixture', 'square')
    lifecycle.invalidateCurrentImage()
    lifecycle.invalidateCurrentImage()

    assert.deepEqual(revoked, ['blob:current'])
    assert.equal(lifecycle.getState().status, 'idle')
  })

  for (const action of ['stopWaiting', 'dispose'] as const) {
    test(`does not publish a late decoded image after ${action}`, async () => {
      const decoded = deferred<ArrayBuffer>()
      const states: ImageGenerationState[] = []
      const successfulImages: unknown[] = []
      const header = Uint8Array.from(atob(squarePngBase64), (character) =>
        character.charCodeAt(0)
      )
      mock.method(
        Blob.prototype,
        'slice',
        () =>
          ({
            arrayBuffer: () => decoded.promise,
          }) as Blob
      )
      const createObjectURL = mock.method(
        URL,
        'createObjectURL',
        () => 'blob:late'
      )
      const lifecycle = createImageGenerationLifecycle(
        () => Promise.resolve({ data: [{ b64_json: squarePngBase64 }] }),
        (state) => states.push(state),
        (image) => successfulImages.push(image)
      )

      const generating = lifecycle.generate('late fixture', 'square')
      await Promise.resolve()
      lifecycle[action]()
      decoded.resolve(header.buffer)
      await generating

      assert.equal(createObjectURL.mock.calls.length, 0)
      assert.deepEqual(successfulImages, [])
      assert.equal(
        lifecycle.getState().status,
        action === 'dispose' ? 'idle' : 'error'
      )
      assert.equal(
        states.at(-1)?.status,
        action === 'dispose' ? 'idle' : 'error'
      )
    })
  }
})
