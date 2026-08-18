import assert from 'node:assert/strict'
import { after, afterEach, before, beforeEach, describe, test } from 'node:test'

import { GlobalRegistrator } from '@happy-dom/global-registrator'
import { act, createElement } from 'react'
import { flushSync } from 'react-dom'
import { createRoot, type Root } from 'react-dom/client'

import { useAuthStore } from '@/stores/auth-store'

import { IMAGE_HISTORY_DATABASE } from '../lib/image-history'
import type { GeneratedImage } from '../types'
import { useImageHistory } from './use-image-history'

let container: HTMLDivElement
let root: Root
let latestHistory: ReturnType<typeof useImageHistory>

function HistoryHarness() {
  latestHistory = useImageHistory()
  return createElement(
    'div',
    null,
    latestHistory.history.map((item) => item.id).join(',')
  )
}

function setAuthenticatedUser(id: number | null) {
  const auth = useAuthStore.getState().auth
  auth.setUser(id === null ? null : { id, role: 1, username: `user-${id}` })
  auth.setBootstrapState('complete')
}

function deleteHistoryDatabase(): Promise<void> {
  return new Promise((resolve, reject) => {
    const request = indexedDB.deleteDatabase(IMAGE_HISTORY_DATABASE)
    request.onsuccess = () => resolve()
    request.onerror = () => reject(request.error)
  })
}

before(() => {
  GlobalRegistrator.register({ url: 'http://localhost' })
  ;(
    globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }
  ).IS_REACT_ACT_ENVIRONMENT = true
})

beforeEach(() => {
  container = document.createElement('div')
  document.body.replaceChildren(container)
  root = createRoot(container)
  useAuthStore.getState().auth.reset('idle')
  flushSync(() => root.render(createElement(HistoryHarness)))
})

afterEach(async () => {
  flushSync(() => root.unmount())
  await deleteHistoryDatabase()
})

after(() => GlobalRegistrator.unregister())

describe('use image history', () => {
  test('fails closed during auth bootstrap and clears old DOM contents on owner change', async () => {
    flushSync(() => setAuthenticatedUser(1))
    await act(async () => {
      await latestHistory.addGeneratedImage(
        {
          aspect: 'square',
          blob: new Blob([new Uint8Array([1])], { type: 'image/png' }),
          generationId: 'first',
          height: 1024,
          imageUrl: 'blob:first',
          size: '1024×1024 PNG',
          width: 1024,
        },
        'First prompt'
      )
    })
    assert.match(container.textContent ?? '', /image-first/)

    flushSync(() => {
      useAuthStore.getState().auth.setBootstrapState('checking')
      useAuthStore
        .getState()
        .auth.setUser({ id: 2, role: 1, username: 'user-2' })
    })
    assert.equal(container.textContent, '')

    flushSync(() => useAuthStore.getState().auth.setBootstrapState('complete'))
    await act(async () => {
      await Promise.resolve()
    })
    assert.equal(container.textContent, '')
  })

  test('keeps the current success image when persistence is unavailable', async () => {
    flushSync(() => setAuthenticatedUser(1))
    const originalIndexedDb = globalThis.indexedDB
    Object.defineProperty(globalThis, 'indexedDB', {
      configurable: true,
      value: undefined,
    })
    const generatedImage: GeneratedImage = {
      aspect: 'square',
      blob: new Blob([new Uint8Array([1])], { type: 'image/png' }),
      generationId: 'not-saved',
      height: 1024,
      imageUrl: 'blob:not-saved',
      size: '1024×1024 PNG',
      width: 1024,
    }

    await act(async () => {
      await latestHistory.addGeneratedImage(
        generatedImage,
        'A still-visible image'
      )
    })
    assert.equal(latestHistory.history.length, 0)
    assert.equal(latestHistory.saveWarning, true)
    Object.defineProperty(globalThis, 'indexedDB', {
      configurable: true,
      value: originalIndexedDb,
    })
  })
})
