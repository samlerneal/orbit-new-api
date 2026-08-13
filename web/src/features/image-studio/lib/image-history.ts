import { maxImageBytes } from './image-generation'

export const IMAGE_HISTORY_DATABASE = 'orbit-image-history'
const IMAGE_HISTORY_STORE = 'images'
const IMAGE_HISTORY_OWNER_STORE = 'owners'
export const MAX_HISTORY_IMAGES = 20
export const MAX_HISTORY_BYTES = 100 * 1024 * 1024

export type ImageHistoryItem = {
  blob: Blob
  createdAt: number
  generationId: string
  id: string
  model: string
  ownerId: number
  prompt: string
  size: string
}

type ImageHistoryOwner = {
  ownerId: number
}

function isValidImage(item: unknown): item is ImageHistoryItem {
  if (!item || typeof item !== 'object') return false
  const candidate = item as Partial<ImageHistoryItem>
  return (
    typeof candidate.id === 'string' &&
    typeof candidate.generationId === 'string' &&
    typeof candidate.ownerId === 'number' &&
    Number.isSafeInteger(candidate.ownerId) &&
    typeof candidate.createdAt === 'number' &&
    Number.isFinite(candidate.createdAt) &&
    typeof candidate.prompt === 'string' &&
    typeof candidate.model === 'string' &&
    typeof candidate.size === 'string' &&
    candidate.blob instanceof Blob &&
    candidate.blob.size > 0 &&
    candidate.blob.size <= maxImageBytes &&
    candidate.blob.type === 'image/png'
  )
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

function openDatabase(): Promise<IDBDatabase> {
  if (typeof indexedDB === 'undefined') {
    return Promise.reject(new Error('Image history is unavailable'))
  }
  return new Promise((resolve, reject) => {
    const request = indexedDB.open(IMAGE_HISTORY_DATABASE, 1)
    request.onerror = () =>
      reject(request.error ?? new Error('Image history is unavailable'))
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
    request.onsuccess = () => resolve(request.result)
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
  const database = await openDatabase()
  try {
    return await operation(database)
  } finally {
    database.close()
  }
}

export async function loadImageHistory(
  ownerId: number
): Promise<ImageHistoryItem[]> {
  return withDatabase(async (database) => {
    const transaction = database.transaction(
      [IMAGE_HISTORY_STORE, IMAGE_HISTORY_OWNER_STORE],
      'readwrite'
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

    if (!isOwnerRecord(owner, ownerId)) {
      for (const item of storedItems) {
        if (item && typeof item === 'object' && (item as { id?: unknown }).id) {
          images.delete((item as { id: string }).id)
        }
      }
      await completeTransaction(transaction)
      return []
    }

    const validItems: ImageHistoryItem[] = []
    for (const item of storedItems) {
      if (isValidImage(item) && item.ownerId === ownerId) {
        validItems.push(item)
      } else if (
        item &&
        typeof item === 'object' &&
        (item as { id?: unknown }).id
      ) {
        images.delete((item as { id: string }).id)
      }
    }
    await completeTransaction(transaction)
    return retainedItems(validItems)
  })
}

export async function saveImageHistoryItem(
  item: ImageHistoryItem
): Promise<void> {
  if (!isValidImage(item)) throw new Error('Invalid image history item')
  await withDatabase(async (database) => {
    const transaction = database.transaction(
      [IMAGE_HISTORY_STORE, IMAGE_HISTORY_OWNER_STORE],
      'readwrite'
    )
    const images = transaction.objectStore(IMAGE_HISTORY_STORE)
    const owners = transaction.objectStore(IMAGE_HISTORY_OWNER_STORE)
    const existing = await requestValue(
      images
        .index('ownerCreatedAt')
        .getAll(
          IDBKeyRange.bound(
            [item.ownerId, 0],
            [item.ownerId, Number.MAX_SAFE_INTEGER]
          )
        )
    )
    const validExisting = existing.filter(
      (candidate): candidate is ImageHistoryItem =>
        isValidImage(candidate) && candidate.ownerId === item.ownerId
    )
    const duplicate = validExisting.some(
      (candidate) => candidate.generationId === item.generationId
    )
    if (!duplicate) {
      const retained = retainedItems([item, ...validExisting])
      const retainedIds = new Set(retained.map((candidate) => candidate.id))
      for (const candidate of existing) {
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
