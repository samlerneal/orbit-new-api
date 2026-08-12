import { api, type ApiRequestConfig } from '@/lib/api'

import type { ImageGenerationResponse } from './types'

export async function generateImage(
  prompt: string,
  signal: AbortSignal
): Promise<ImageGenerationResponse> {
  const config: ApiRequestConfig = { signal, skipErrorHandler: true }
  const response = await api.post<ImageGenerationResponse>(
    '/pg/images/generations',
    { prompt },
    config
  )
  return response.data
}
