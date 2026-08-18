import { useCallback, useEffect, useMemo, useRef, useState } from 'react'

import { useAuthStore } from '@/stores/auth-store'

import {
  clearImageHistory,
  deleteImageHistoryItem,
  loadImageHistory,
  saveImageHistoryItem,
  type ImageHistoryItem,
} from '../lib/image-history'
import type { GeneratedImage } from '../types'

const FIXED_MODEL = 'gpt-image-2'

function createHistoryId(generationId: string): string {
  return `image-${generationId}`
}

export function useImageHistory() {
  const ownerId = useAuthStore((state) => state.auth.user?.id ?? null)
  const authReady = useAuthStore(
    (state) => state.auth.bootstrapState === 'complete'
  )
  const [history, setHistory] = useState<ImageHistoryItem[]>([])
  const [saveWarning, setSaveWarning] = useState(false)
  const activeOwnerRef = useRef<number | null>(null)
  const previousOwnerRef = useRef<number | null>(null)

  useEffect(() => {
    const previousOwnerId = previousOwnerRef.current
    activeOwnerRef.current = null
    setHistory([])
    setSaveWarning(false)
    if (ownerId === null) {
      previousOwnerRef.current = null
      if (previousOwnerId !== null) {
        void clearImageHistory(previousOwnerId).catch(() => undefined)
      }
      return
    }
    previousOwnerRef.current = ownerId
    if (!authReady) return
    let cancelled = false
    activeOwnerRef.current = ownerId
    void loadImageHistory(ownerId)
      .then((items) => {
        if (cancelled || activeOwnerRef.current !== ownerId) return
        setHistory(items)
      })
      .catch(() => {
        if (!cancelled) setSaveWarning(true)
      })
    return () => {
      cancelled = true
    }
  }, [authReady, ownerId])

  const addGeneratedImage = useCallback(
    async (generatedImage: GeneratedImage, prompt: string) => {
      const currentOwnerId = activeOwnerRef.current
      const currentAuth = useAuthStore.getState().auth
      if (
        currentOwnerId === null ||
        currentAuth.bootstrapState !== 'complete' ||
        currentAuth.user?.id !== currentOwnerId
      ) {
        return
      }
      const item: ImageHistoryItem = {
        blob: generatedImage.blob,
        aspect: generatedImage.aspect,
        createdAt: Date.now(),
        generationId: generatedImage.generationId,
        height: generatedImage.height,
        id: createHistoryId(generatedImage.generationId),
        model: FIXED_MODEL,
        ownerId: currentOwnerId,
        prompt,
        size: generatedImage.size,
        width: generatedImage.width,
      }
      try {
        await saveImageHistoryItem(item)
        if (activeOwnerRef.current !== currentOwnerId) return
        const updatedHistory = await loadImageHistory(currentOwnerId)
        if (activeOwnerRef.current === currentOwnerId) {
          setHistory(updatedHistory)
          setSaveWarning(false)
        }
      } catch {
        if (activeOwnerRef.current === currentOwnerId) setSaveWarning(true)
      }
    },
    []
  )

  const removeHistoryItem = useCallback(async (id: string) => {
    const currentOwnerId = activeOwnerRef.current
    if (currentOwnerId === null) return
    try {
      await deleteImageHistoryItem(currentOwnerId, id)
      if (activeOwnerRef.current === currentOwnerId) {
        setHistory((current) => current.filter((item) => item.id !== id))
      }
    } catch {
      if (activeOwnerRef.current === currentOwnerId) setSaveWarning(true)
    }
  }, [])

  const removeAllHistory = useCallback(async () => {
    const currentOwnerId = activeOwnerRef.current
    if (currentOwnerId === null) return
    try {
      await clearImageHistory(currentOwnerId)
      if (activeOwnerRef.current === currentOwnerId) setHistory([])
    } catch {
      if (activeOwnerRef.current === currentOwnerId) setSaveWarning(true)
    }
  }, [])

  const visibleHistory = useMemo(
    () =>
      authReady && ownerId !== null
        ? history.filter((item) => item.ownerId === ownerId)
        : [],
    [authReady, history, ownerId]
  )

  return {
    addGeneratedImage,
    history: visibleHistory,
    removeAllHistory,
    removeHistoryItem,
    saveWarning,
  }
}
