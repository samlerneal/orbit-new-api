import assert from 'node:assert/strict'
import { afterEach, describe, mock, test } from 'node:test'

import type { ImageGenerationResponse, ImageGenerationState } from '../types'
import {
  createImageGenerationLifecycle,
  type ImageGenerationRequest,
} from './use-image-generation'

function deferred<T>() {
  let resolve!: (value: T) => void
  let reject!: (reason?: unknown) => void
  const promise = new Promise<T>((promiseResolve, promiseReject) => {
    resolve = promiseResolve
    reject = promiseReject
  })
  return { promise, reject, resolve }
}

afterEach(() => mock.restoreAll())

describe('image generation request lifecycle', () => {
  test('sends one request through repeated generate calls', async () => {
    const pending = deferred<ImageGenerationResponse>()
    let requests = 0
    const states: ImageGenerationState[] = []
    const lifecycle = createImageGenerationLifecycle(
      () => {
        requests += 1
        return pending.promise
      },
      (state) => states.push(state)
    )

    void lifecycle.generate('first')
    void lifecycle.generate('second')
    assert.equal(requests, 1)
    assert.equal(lifecycle.getState().status, 'generating')

    pending.reject(new Error('fake failure'))
    await Promise.resolve()
    assert.equal(lifecycle.getState().status, 'error')
    assert.equal(requests, 1)
    assert.equal(states.length, 2)
  })

  test('does not retry after success, failure, or stopping the wait', async () => {
    const responses = [
      Promise.resolve({ data: [{ b64_json: 'iVBORw0KGgo=' }] }),
      Promise.reject(new Error('fake failure')),
      deferred<ImageGenerationResponse>(),
    ]
    let requests = 0
    mock.method(URL, 'createObjectURL', () => 'blob:generated')
    const lifecycle = createImageGenerationLifecycle(
      (_prompt, signal) => {
        const response = responses[requests++]
        assert.ok(response)
        if ('promise' in response) return response.promise
        assert.equal(signal.aborted, false)
        return response
      },
      () => undefined
    )

    await lifecycle.generate('success')
    assert.deepEqual(lifecycle.getState(), {
      status: 'success',
      imageUrl: 'blob:generated',
      error: null,
    })
    await lifecycle.generate('failure')
    assert.equal(lifecycle.getState().status, 'error')
    void lifecycle.generate('stop')
    lifecycle.stopWaiting()
    assert.equal(lifecycle.getState().status, 'error')
    assert.equal(requests, 3)
    const pending = responses[2]
    assert.ok('reject' in pending)
    pending.reject(new DOMException('aborted', 'AbortError'))
    await Promise.resolve()
    assert.equal(requests, 3)
  })

  test('revokes replaced and disposed image object URLs', async () => {
    const revoked: string[] = []
    let created = 0
    mock.method(URL, 'createObjectURL', () => `blob:${++created}`)
    mock.method(URL, 'revokeObjectURL', (url: string) => revoked.push(url))
    const requestImage: ImageGenerationRequest = () =>
      Promise.resolve({ data: [{ b64_json: 'iVBORw0KGgo=' }] })
    const lifecycle = createImageGenerationLifecycle(
      requestImage,
      () => undefined
    )

    await lifecycle.generate('first')
    await lifecycle.generate('replacement')
    assert.deepEqual(revoked, ['blob:1'])
    lifecycle.dispose()
    assert.deepEqual(revoked, ['blob:1', 'blob:2'])
  })
})
