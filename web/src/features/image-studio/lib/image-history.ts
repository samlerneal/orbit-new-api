import { imageAspectMetadata, type ImageAspect } from '../types'
import { decodePngDimensions, maxImageBytes } from './image-generation'

export const IMAGE_HISTORY_DATABASE = 'orbit-image-history'
const IMAGE_HISTORY_STORE = 'images'
const IMAGE_HISTORY_OWNER_STORE = 'owners'
export const MAX_HISTORY_IMAGES = 20
export const MAX_HISTORY_BYTES = 100 * 1024 * 1024
export const IMAGE_HISTORY_OPEN_TIMEOUT_MS = 3_000

export type ImageHistoryItem = {
  aspect: ImageAspect
  blob: Blob
  createdAt: number
  generationId: string
  height: number
  id: string
  model: string
  ownerId: number
  prompt: string
  size: string
  width: number
}

type StoredImageHistoryItem = Omit<
  ImageHistoryItem,
  'aspect' | 'height' | 'size' | 'width'
> & {
  aspect?: unknown
  height?: unknown
  size?: unknown
  width?: unknown
}

type ImageHistoryOwner = {
  ownerId: number
}

function isValidImage(item: unknown): item is StoredImageHistoryItem {
  if (!item || typeof item !== 'object') return false
  const candidate = item as Partial<StoredImageHistoryItem>
  return (
    typeof candidate.id === 'string' &&
    typeof candidate.generationId === 'string' &&
    typeof candidate.ownerId === 'number' &&
    Number.isSafeInteger(candidate.ownerId) &&
    typeof candidate.createdAt === 'number' &&
    Number.isFinite(candidate.createdAt) &&
    typeof candidate.prompt === 'string' &&
    typeof candidate.model === 'string' &&
    candidate.blob instanceof Blob &&
    candidate.blob.size > 0 &&
    candidate.blob.size <= maxImageBytes &&
    candidate.blob.type === 'image/png'
  )
}

function normalizeImageHistoryItem(
  item: StoredImageHistoryItem
): ImageHistoryItem | null {
  const isLegacySquare = item.aspect === undefined
  let aspect: ImageAspect | null = null
  if (isLegacySquare) {
    aspect = 'square'
  } else if (
    item.aspect === 'square' ||
    item.aspect === 'xiaohongshu' ||
    item.aspect === 'landscape' ||
    item.aspect === 'portrait'
  ) {
    aspect = item.aspect
  }
  if (!aspect) return null
  const metadata = imageAspectMetadata[aspect]
  if (
    (!isLegacySquare &&
      (item.width === undefined ||
        item.height === undefined ||
        item.size === undefined)) ||
    (item.width !== undefined && item.width !== metadata.width) ||
    (item.height !== undefined && item.height !== metadata.height) ||
    (item.size !== undefined && item.size !== metadata.size)
  ) {
    return null
  }
  return {
    ...item,
    aspect,
    height: metadata.height,
    size: metadata.size,
    width: metadata.width,
  }
}

async function hasVerifiedImageFacts(
  item: StoredImageHistoryItem
): Promise<boolean> {
  const normalized = normalizeImageHistoryItem(item)
  if (!normalized) return false
  const metadata = imageAspectMetadata[normalized.aspect]
  try {
    const dimensions = await decodePngDimensions(item.blob)
    return (
      dimensions.width === metadata.width &&
      dimensions.height === metadata.height &&
      normalized.width === metadata.width &&
      normalized.height === metadata.height &&
      normalized.size === metadata.size
    )
  } catch {
    return false
  }
}

function isOwnerRecord(
  value: unknown,
  ownerId: number
): value is ImageHistoryOwner {
  return (
    !!value &&
    typeof value === 'object' &&
    (value as Partial<ImageHistoryOwner>).ownerId === ownerId
  )
}

export function openImageHistoryDatabase(
  timeoutMs = IMAGE_HISTORY_OPEN_TIMEOUT_MS
): Promise<IDBDatabase> {
  if (typeof indexedDB === 'undefined') {
    return Promise.reject(new Error('Image history is unavailable'))
  }
  return new Promise((resolve, reject) => {
    let settled = false
    const settle = (callback: () => void) => {
      if (settled) return
      settled = true
      globalThis.clearTimeout(timeout)
      callback()
    }
    const request = indexedDB.open(IMAGE_HISTORY_DATABASE, 1)
    const timeout = globalThis.setTimeout(
      () => settle(() => reject(new Error('Image history is unavailable'))),
      timeoutMs
    )
    request.onblocked = () =>
      settle(() => reject(new Error('Image history is unavailable')))
    request.onerror = () =>
      settle(() =>
        reject(request.error ?? new Error('Image history is unavailable'))
      )
    request.onupgradeneeded = () => {
      const database = request.result
      if (!database.objectStoreNames.contains(IMAGE_HISTORY_STORE)) {
        const store = database.createObjectStore(IMAGE_HISTORY_STORE, {
          keyPath: 'id',
        })
        store.createIndex('ownerCreatedAt', ['ownerId', 'createdAt'])
        store.createIndex('ownerGenerationId', ['ownerId', 'generationId'], {
          unique: true,
        })
      }
      if (!database.objectStoreNames.contains(IMAGE_HISTORY_OWNER_STORE)) {
        database.createObjectStore(IMAGE_HISTORY_OWNER_STORE, {
          keyPath: 'ownerId',
        })
      }
    }
    request.onsuccess = () => {
      if (settled) {
        request.result.close()
        return
      }
      settle(() => resolve(request.result))
    }
  })
}

function completeTransaction(transaction: IDBTransaction): Promise<void> {
  return new Promise((resolve, reject) => {
    transaction.oncomplete = () => resolve()
    transaction.onerror = () =>
      reject(transaction.error ?? new Error('Image history operation failed'))
    transaction.onabort = () =>
      reject(transaction.error ?? new Error('Image history operation failed'))
  })
}

function requestValue<T>(request: IDBRequest<T>): Promise<T> {
  return new Promise((resolve, reject) => {
    request.onsuccess = () => resolve(request.result)
    request.onerror = () =>
      reject(request.error ?? new Error('Image history operation failed'))
  })
}

function sortHistory(items: ImageHistoryItem[]): ImageHistoryItem[] {
  return [...items].sort((left, right) => right.createdAt - left.createdAt)
}

function retainedItems(items: ImageHistoryItem[]): ImageHistoryItem[] {
  let totalBytes = 0
  return sortHistory(items).filter((item, index) => {
    totalBytes += item.blob.size
    return index < MAX_HISTORY_IMAGES && totalBytes <= MAX_HISTORY_BYTES
  })
}

async function withDatabase<T>(
  operation: (database: IDBDatabase) => Promise<T>
) {
  const database = await openImageHistoryDatabase()
  try {
    return await operation(database)
  } finally {
    database.close()
  }
}

export async function loadImageHistory(
  ownerId: number
): Promise<ImageHistoryItem[]> {
  const { owner, storedItems } = await withDatabase(async (database) => {
    const transaction = database.transaction(
      [IMAGE_HISTORY_STORE, IMAGE_HISTORY_OWNER_STORE],
      'readonly'
    )
    const images = transaction.objectStore(IMAGE_HISTORY_STORE)
    const owners = transaction.objectStore(IMAGE_HISTORY_OWNER_STORE)
    const owner = await requestValue(owners.get(ownerId))
    const storedItems = await requestValue(
      images
        .index('ownerCreatedAt')
        .getAll(
          IDBKeyRange.bound([ownerId, 0], [ownerId, Number.MAX_SAFE_INTEGER])
        )
    )

    await completeTransaction(transaction)
    return { owner, storedItems }
  })

  const validItems: ImageHistoryItem[] = []
  const invalidIds: string[] = []
  for (const item of storedItems) {
    if (
      isOwnerRecord(owner, ownerId) &&
      isValidImage(item) &&
      item.ownerId === ownerId &&
      (await hasVerifiedImageFacts(item))
    ) {
      const normalized = normalizeImageHistoryItem(item)
      if (normalized) validItems.push(normalized)
    } else if (
      item &&
      typeof item === 'object' &&
      typeof (item as { id?: unknown }).id === 'string'
    ) {
      invalidIds.push((item as { id: string }).id)
    }
  }
  if (invalidIds.length > 0) {
    await withDatabase(async (database) => {
      const transaction = database.transaction(IMAGE_HISTORY_STORE, 'readwrite')
      const images = transaction.objectStore(IMAGE_HISTORY_STORE)
      for (const id of invalidIds) images.delete(id)
      await completeTransaction(transaction)
    })
  }
  return retainedItems(validItems)
}

export async function saveImageHistoryItem(
  item: ImageHistoryItem
): Promise<void> {
  const metadata = imageAspectMetadata[item.aspect]
  if (
    !isValidImage(item) ||
    item.width !== metadata.width ||
    item.height !== metadata.height ||
    item.size !== metadata.size
  ) {
    throw new Error('Invalid image history item')
  }
  if (!(await hasVerifiedImageFacts(item))) {
    throw new Error('Invalid image history item')
  }
  const existing = await withDatabase(async (database) => {
    const transaction = database.transaction(IMAGE_HISTORY_STORE, 'readonly')
    const request = transaction
      .objectStore(IMAGE_HISTORY_STORE)
      .index('ownerCreatedAt')
      .getAll(
        IDBKeyRange.bound(
          [item.ownerId, 0],
          [item.ownerId, Number.MAX_SAFE_INTEGER]
        )
      )
    const result = await requestValue(request)
    await completeTransaction(transaction)
    return result
  })
  const verifiedExisting = (
    await Promise.all(
      existing.map(async (candidate) =>
        isValidImage(candidate) &&
        candidate.ownerId === item.ownerId &&
        (await hasVerifiedImageFacts(candidate))
          ? normalizeImageHistoryItem(candidate)
          : null
      )
    )
  ).filter((candidate): candidate is ImageHistoryItem => !!candidate)
  await withDatabase(async (database) => {
    const transaction = database.transaction(
      [IMAGE_HISTORY_STORE, IMAGE_HISTORY_OWNER_STORE],
      'readwrite'
    )
    const images = transaction.objectStore(IMAGE_HISTORY_STORE)
    const owners = transaction.objectStore(IMAGE_HISTORY_OWNER_STORE)
    const current = await requestValue(
      images
        .index('ownerCreatedAt')
        .getAll(
          IDBKeyRange.bound(
            [item.ownerId, 0],
            [item.ownerId, Number.MAX_SAFE_INTEGER]
          )
        )
    )
    if (current.length !== existing.length) {
      transaction.abort()
      throw new Error('Image history operation failed')
    }
    const duplicate = current.some(
      (candidate) =>
        isValidImage(candidate) &&
        candidate.ownerId === item.ownerId &&
        candidate.generationId === item.generationId
    )
    if (!duplicate) {
      const retained = retainedItems([item, ...verifiedExisting])
      const retainedIds = new Set(retained.map((candidate) => candidate.id))
      for (const candidate of current) {
        if (
          candidate &&
          typeof candidate === 'object' &&
          typeof (candidate as { id?: unknown }).id === 'string' &&
          !retainedIds.has((candidate as { id: string }).id)
        ) {
          images.delete((candidate as { id: string }).id)
        }
      }
      images.put(item)
    }
    owners.put({ ownerId: item.ownerId })
    await completeTransaction(transaction)
  })
}

export async function deleteImageHistoryItem(
  ownerId: number,
  id: string
): Promise<void> {
  await withDatabase(async (database) => {
    const transaction = database.transaction(IMAGE_HISTORY_STORE, 'readwrite')
    const images = transaction.objectStore(IMAGE_HISTORY_STORE)
    const item = await requestValue(images.get(id))
    if (isValidImage(item) && item.ownerId === ownerId) images.delete(id)
    await completeTransaction(transaction)
  })
}

export async function clearImageHistory(ownerId: number): Promise<void> {
  await withDatabase(async (database) => {
    const transaction = database.transaction(
      [IMAGE_HISTORY_STORE, IMAGE_HISTORY_OWNER_STORE],
      'readwrite'
    )
    const images = transaction.objectStore(IMAGE_HISTORY_STORE)
    const owners = transaction.objectStore(IMAGE_HISTORY_OWNER_STORE)
    const items = await requestValue(
      images
        .index('ownerCreatedAt')
        .getAll(
          IDBKeyRange.bound([ownerId, 0], [ownerId, Number.MAX_SAFE_INTEGER])
        )
    )
    for (const item of items) {
      if (
        item &&
        typeof item === 'object' &&
        typeof (item as { id?: unknown }).id === 'string'
      ) {
        images.delete((item as { id: string }).id)
      }
    }
    owners.delete(ownerId)
    await completeTransaction(transaction)
  })
}
