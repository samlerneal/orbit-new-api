import assert from 'node:assert/strict'
import { after, afterEach, before, describe, test } from 'node:test'
import { deflateSync } from 'node:zlib'

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
function png(width: number, height: number) {
  const header = new Uint8Array(13)
  const view = new DataView(header.buffer)
  view.setUint32(0, width)
  view.setUint32(4, height)
  header[8] = 8
  header[9] = 6
  const chunk = (type: string, data: Uint8Array) => {
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
  const raw = new Uint8Array((width * 4 + 1) * height)
  return new Uint8Array(
    Buffer.concat([
      Buffer.from([137, 80, 78, 71, 13, 10, 26, 10]),
      Buffer.from(chunk('IHDR', header)),
      Buffer.from(chunk('IDAT', deflateSync(raw))),
      Buffer.from(chunk('IEND', new Uint8Array())),
    ])
  )
}
const squarePngHeader = png(1024, 1024)

const ownerId = 47

function createItem(
  id: string,
  createdAt: number,
  byteLength = 8,
  itemOwnerId = ownerId
): ImageHistoryItem {
  return {
    aspect: 'square',
    blob: new Blob(
      [
        squarePngHeader,
        new Uint8Array(Math.max(0, byteLength - squarePngHeader.length)),
      ],
      { type: 'image/png' }
    ),
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

  test('retains Blob facts for each native preset after reload', async () => {
    const presets = [
      { aspect: 'square' as const, height: 1024, width: 1024 },
      { aspect: 'landscape' as const, height: 1024, width: 1536 },
      { aspect: 'portrait' as const, height: 1536, width: 1024 },
    ]
    for (const [index, preset] of presets.entries()) {
      await saveImageHistoryItem({
        ...createItem(preset.aspect, index),
        aspect: preset.aspect,
        blob: new Blob([png(preset.width, preset.height)], {
          type: 'image/png',
        }),
        height: preset.height,
        size: `${preset.width}×${preset.height} PNG`,
        width: preset.width,
      })
    }

    const loaded = await loadImageHistory(ownerId)
    for (const preset of presets) {
      const item = loaded.find(
        (candidate) => candidate.aspect === preset.aspect
      )
      assert.equal(item?.width, preset.width)
      assert.equal(item?.height, preset.height)
      assert.equal(item?.size, `${preset.width}×${preset.height} PNG`)
    }
  })

  test('fails closed when a stored aspect declaration disagrees with PNG IHDR', async () => {
    await assert.rejects(
      () =>
        saveImageHistoryItem({
          ...createItem('mismatch', 1),
          aspect: 'landscape',
          width: 1536,
          height: 1024,
          size: '1536×1024 PNG',
        }),
      /Invalid image history item/
    )
  })

  test('deletes direct IDB records with present but incorrect declarations', async () => {
    const database = await openImageHistoryDatabase()
    const transaction = database.transaction(['images', 'owners'], 'readwrite')
    transaction.objectStore('images').put({
      ...createItem('wrong-declaration', 1),
      aspect: 'square',
      width: 1536,
    })
    transaction.objectStore('owners').put({ ownerId })
    await new Promise<void>((resolve, reject) => {
      transaction.oncomplete = () => resolve()
      transaction.onerror = () => reject(transaction.error)
    })
    database.close()

    assert.deepEqual(await loadImageHistory(ownerId), [])
  })

  test('deletes direct IDB records with an unknown aspect instead of normalizing them', async () => {
    const database = await openImageHistoryDatabase()
    const transaction = database.transaction(['images', 'owners'], 'readwrite')
    transaction.objectStore('images').put({
      ...createItem('unknown-aspect', 1),
      aspect: 'unexpected',
    })
    transaction.objectStore('owners').put({ ownerId })
    await new Promise<void>((resolve, reject) => {
      transaction.oncomplete = () => resolve()
      transaction.onerror = () => reject(transaction.error)
    })
    database.close()

    assert.deepEqual(await loadImageHistory(ownerId), [])
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
