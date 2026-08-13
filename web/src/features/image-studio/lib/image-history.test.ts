import assert from 'node:assert/strict'
import { after, afterEach, before, describe, test } from 'node:test'

import { GlobalRegistrator } from '@happy-dom/global-registrator'

import {
  clearImageHistory,
  deleteImageHistoryItem,
  IMAGE_HISTORY_DATABASE,
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
    blob: new Blob([new Uint8Array(byteLength)], { type: 'image/png' }),
    createdAt,
    generationId: `generation-${id}`,
    id,
    model: 'gpt-image-2',
    ownerId: itemOwnerId,
    prompt: `Prompt ${id}`,
    size: '1024×1024 PNG',
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
