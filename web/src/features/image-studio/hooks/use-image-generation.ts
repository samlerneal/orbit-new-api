import { useEffect, useRef, useState } from 'react'

import { createImageBlob, revokeImageObjectUrl } from '../lib/image-generation'
import type {
  GeneratedImage,
  ImageGenerationResponse,
  ImageGenerationState,
} from '../types'

const initialState: ImageGenerationState = {
  status: 'idle',
  imageUrl: null,
  error: null,
}

export type ImageGenerationRequest = (
  prompt: string,
  signal: AbortSignal
) => Promise<ImageGenerationResponse>

export type ImageGenerationSuccessHandler = (
  generatedImage: GeneratedImage,
  prompt: string
) => void

export function createImageGenerationLifecycle(
  requestImage: ImageGenerationRequest,
  onStateChange: (state: ImageGenerationState) => void,
  onSuccess?: ImageGenerationSuccessHandler
) {
  let currentRequest = requestImage
  let currentSuccessHandler = onSuccess
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
      const blob = createImageBlob(response.data?.[0]?.b64_json || '')
      imageUrl = URL.createObjectURL(blob)
      const generatedImage = {
        blob,
        generationId: crypto.randomUUID(),
        imageUrl,
      }
      publishState({ status: 'success', imageUrl, error: null, generatedImage })
      currentSuccessHandler?.(generatedImage, prompt)
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
    publishState(initialState)
  }

  const invalidateCurrentImage = () => {
    clearImage()
    publishState(initialState)
  }

  return {
    dispose,
    generate,
    getState: () => state,
    invalidateCurrentImage,
    setRequestImage: (nextRequest: ImageGenerationRequest) => {
      currentRequest = nextRequest
    },
    setSuccessHandler: (
      nextSuccessHandler: ImageGenerationSuccessHandler | undefined
    ) => {
      currentSuccessHandler = nextSuccessHandler
    },
    stopWaiting,
  }
}

export function useImageGeneration(
  requestImage: ImageGenerationRequest,
  onSuccess?: ImageGenerationSuccessHandler
) {
  const [state, setState] = useState<ImageGenerationState>(initialState)
  const lifecycleRef = useRef<ReturnType<
    typeof createImageGenerationLifecycle
  > | null>(null)

  if (!lifecycleRef.current) {
    lifecycleRef.current = createImageGenerationLifecycle(
      requestImage,
      setState,
      onSuccess
    )
  }
  lifecycleRef.current.setRequestImage(requestImage)
  lifecycleRef.current.setSuccessHandler(onSuccess)

  useEffect(() => () => lifecycleRef.current?.dispose(), [])

  return {
    state,
    generate: lifecycleRef.current.generate,
    invalidateCurrentImage: lifecycleRef.current.invalidateCurrentImage,
    stopWaiting: lifecycleRef.current.stopWaiting,
  }
}
