import assert from 'node:assert/strict'
import { after, afterEach, before, describe, test } from 'node:test'

import { GlobalRegistrator } from '@happy-dom/global-registrator'

import {
  clearImageHistory,
  deleteImageHistoryItem,
  IMAGE_HISTORY_DATABASE,
  openImageHistoryDatabase,
  loadImageHistory,
  MAX_HISTORY_BYTES,
  MAX_HISTORY_IMAGES,
  saveImageHistoryItem,
  type ImageHistoryItem,
} from './image-history'

const ownerId = 47

function createItem(
  id: string,
  createdAt: number,
  byteLength = 8,
  itemOwnerId = ownerId
): ImageHistoryItem {
  return {
    aspect: 'square',
    blob: new Blob([new Uint8Array(byteLength)], { type: 'image/png' }),
    createdAt,
    generationId: `generation-${id}`,
    height: 1024,
    id,
    model: 'gpt-image-2',
    ownerId: itemOwnerId,
    prompt: `Prompt ${id}`,
    size: '1024×1024 PNG',
    width: 1024,
  }
}

function deleteHistoryDatabase(): Promise<void> {
  return new Promise((resolve, reject) => {
    const request = indexedDB.deleteDatabase(IMAGE_HISTORY_DATABASE)
    request.onsuccess = () => resolve()
    request.onerror = () => reject(request.error)
  })
}

before(() => GlobalRegistrator.register({ url: 'http://localhost' }))
after(() => GlobalRegistrator.unregister())
afterEach(async () => deleteHistoryDatabase())

describe('image history storage', () => {
  test('fails open when IndexedDB is blocked and closes a late connection', async () => {
    const originalIndexedDb = globalThis.indexedDB
    let closeCalls = 0
    const lateDatabase = {
      close: () => (closeCalls += 1),
    } as unknown as IDBDatabase
    const request = {} as IDBOpenDBRequest
    Object.defineProperties(request, {
      onblocked: { configurable: true, writable: true, value: null },
      onerror: { configurable: true, writable: true, value: null },
      onsuccess: { configurable: true, writable: true, value: null },
      onupgradeneeded: { configurable: true, writable: true, value: null },
      result: { configurable: true, value: lateDatabase },
      error: { configurable: true, value: null },
    })
    globalThis.indexedDB = { open: () => request } as unknown as IDBFactory
    try {
      const opening = openImageHistoryDatabase(1_000)
      ;(request.onblocked as (() => void) | null)?.()
      await assert.rejects(() => opening, /unavailable/)
      ;(request.onsuccess as (() => void) | null)?.()
      assert.equal(closeCalls, 1)
    } finally {
      globalThis.indexedDB = originalIndexedDb
    }
  })

  test('keeps each owner isolated and uses owner metadata to fail closed', async () => {
    await saveImageHistoryItem(createItem('one', 1))
    await saveImageHistoryItem(createItem('two', 2, 8, 48))

    assert.deepEqual(
      (await loadImageHistory(ownerId)).map((item) => item.id),
      ['one']
    )
    assert.deepEqual(
      (await loadImageHistory(48)).map((item) => item.id),
      ['two']
    )
    assert.deepEqual(await loadImageHistory(49), [])
  })

  test('treats generation IDs as idempotent', async () => {
    const first = createItem('one', 1)
    await saveImageHistoryItem(first)
    await saveImageHistoryItem({ ...first, id: 'duplicate', createdAt: 2 })

    assert.deepEqual(
      (await loadImageHistory(ownerId)).map((item) => item.id),
      ['one']
    )
  })

  test('retains the aspect and dimensions of newly saved records', async () => {
    await saveImageHistoryItem({
      ...createItem('portrait', 1),
      aspect: 'portrait',
      height: 1536,
      size: '1024×1536 PNG',
    })

    const [loaded] = await loadImageHistory(ownerId)
    assert.equal(loaded?.aspect, 'portrait')
    assert.equal(loaded?.width, 1024)
    assert.equal(loaded?.height, 1536)
    assert.equal(loaded?.size, '1024×1536 PNG')
  })

  test('reads legacy records as square images without modifying the stored record', async () => {
    const legacy = createItem('legacy', 1)
    const {
      aspect: _aspect,
      height: _height,
      width: _width,
      ...storedLegacy
    } = legacy
    const database = await openImageHistoryDatabase()
    const transaction = database.transaction(['images', 'owners'], 'readwrite')
    transaction.objectStore('images').put(storedLegacy)
    transaction.objectStore('owners').put({ ownerId })
    await new Promise<void>((resolve, reject) => {
      transaction.oncomplete = () => resolve()
      transaction.onerror = () => reject(transaction.error)
    })
    database.close()

    const [loaded] = await loadImageHistory(ownerId)
    assert.equal(loaded?.aspect, 'square')
    assert.equal(loaded?.size, '1024×1024 PNG')
    assert.equal(loaded?.width, 1024)
    assert.equal(loaded?.height, 1024)
  })

  test('retains exactly twenty items then evicts the oldest', async () => {
    for (let index = 0; index < MAX_HISTORY_IMAGES; index += 1) {
      await saveImageHistoryItem(createItem(`${index}`, index))
    }
    assert.equal((await loadImageHistory(ownerId)).length, MAX_HISTORY_IMAGES)

    await saveImageHistoryItem(createItem('newest', MAX_HISTORY_IMAGES))
    const history = await loadImageHistory(ownerId)
    assert.equal(history.length, MAX_HISTORY_IMAGES)
    assert.equal(history.at(-1)?.id, '1')
    assert.equal(history[0]?.id, 'newest')
  })

  test('retains exactly one hundred MiB then evicts by oldest creation time', async () => {
    const imageBytes = MAX_HISTORY_BYTES / 5
    for (let index = 0; index < 5; index += 1) {
      await saveImageHistoryItem(createItem(`${index}`, index, imageBytes))
    }
    assert.equal(
      (await loadImageHistory(ownerId)).reduce(
        (total, item) => total + item.blob.size,
        0
      ),
      MAX_HISTORY_BYTES
    )

    await saveImageHistoryItem(createItem('newest', 5, imageBytes))
    const history = await loadImageHistory(ownerId)
    assert.equal(history.length, 5)
    assert.equal(history.at(-1)?.id, '1')
  })

  test('deletes individual items and clears the current owner only', async () => {
    await saveImageHistoryItem(createItem('one', 1))
    await saveImageHistoryItem(createItem('two', 2))
    await saveImageHistoryItem(createItem('other', 3, 8, 48))

    await deleteImageHistoryItem(ownerId, 'one')
    assert.deepEqual(
      (await loadImageHistory(ownerId)).map((item) => item.id),
      ['two']
    )
    await clearImageHistory(ownerId)
    assert.deepEqual(await loadImageHistory(ownerId), [])
    assert.deepEqual(
      (await loadImageHistory(48)).map((item) => item.id),
      ['other']
    )
  })
})
