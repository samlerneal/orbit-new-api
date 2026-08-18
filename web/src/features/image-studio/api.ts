import { api, type ApiRequestConfig } from '@/lib/api'

import type { ImageAspect, ImageGenerationResponse } from './types'

export async function generateImage(
  prompt: string,
  aspect: ImageAspect,
  signal: AbortSignal
): Promise<ImageGenerationResponse> {
  const config: ApiRequestConfig = { signal, skipErrorHandler: true }
  const response = await api.post<ImageGenerationResponse>(
    '/pg/images/generations',
    { prompt, aspect },
    config
  )
  return response.data
}
