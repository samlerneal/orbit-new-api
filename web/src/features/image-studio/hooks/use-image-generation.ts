import { useEffect, useRef, useState } from 'react'

import {
  createImageObjectUrl,
  revokeImageObjectUrl,
} from '../lib/image-generation'
import type { ImageGenerationResponse, ImageGenerationState } from '../types'

const initialState: ImageGenerationState = {
  status: 'idle',
  imageUrl: null,
  error: null,
}

export type ImageGenerationRequest = (
  prompt: string,
  signal: AbortSignal
) => Promise<ImageGenerationResponse>

export function createImageGenerationLifecycle(
  requestImage: ImageGenerationRequest,
  onStateChange: (state: ImageGenerationState) => void
) {
  let currentRequest = requestImage
  let controller: AbortController | null = null
  let imageUrl: string | null = null
  let state = initialState

  const publishState = (nextState: ImageGenerationState) => {
    state = nextState
    onStateChange(state)
  }

  const clearImage = () => {
    revokeImageObjectUrl(imageUrl)
    imageUrl = null
  }

  const generate = async (prompt: string) => {
    if (controller) return
    clearImage()
    const activeController = new AbortController()
    controller = activeController
    publishState({ status: 'generating', imageUrl: null, error: null })
    try {
      const response = await currentRequest(prompt, activeController.signal)
      if (controller !== activeController) return
      imageUrl = createImageObjectUrl(response.data?.[0]?.b64_json || '')
      publishState({ status: 'success', imageUrl, error: null })
    } catch {
      if (controller !== activeController) return
      publishState({
        status: 'error',
        imageUrl: null,
        error: activeController.signal.aborted
          ? 'The request may still be processing. You can generate again when ready.'
          : 'We could not generate that image. Please try again later.',
      })
    } finally {
      if (controller === activeController) controller = null
    }
  }

  const stopWaiting = () => {
    if (!controller) return
    controller.abort()
    publishState({
      status: 'error',
      imageUrl: null,
      error:
        'The request may still be processing. You can generate again when ready.',
    })
    controller = null
  }

  const dispose = () => {
    controller?.abort()
    controller = null
    clearImage()
  }

  return {
    dispose,
    generate,
    getState: () => state,
    setRequestImage: (nextRequest: ImageGenerationRequest) => {
      currentRequest = nextRequest
    },
    stopWaiting,
  }
}

export function useImageGeneration(requestImage: ImageGenerationRequest) {
  const [state, setState] = useState<ImageGenerationState>(initialState)
  const lifecycleRef = useRef<ReturnType<
    typeof createImageGenerationLifecycle
  > | null>(null)

  if (!lifecycleRef.current) {
    lifecycleRef.current = createImageGenerationLifecycle(
      requestImage,
      setState
    )
  }
  lifecycleRef.current.setRequestImage(requestImage)

  useEffect(() => () => lifecycleRef.current?.dispose(), [])

  return {
    state,
    generate: lifecycleRef.current.generate,
    stopWaiting: lifecycleRef.current.stopWaiting,
  }
}
