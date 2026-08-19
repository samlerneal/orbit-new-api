export type ImageAspect = 'square' | 'xiaohongshu' | 'landscape' | 'portrait'

export const imageAspectMetadata: Record<
  ImageAspect,
  { aspect: ImageAspect; height: number; size: string; width: number }
> = {
  square: {
    aspect: 'square',
    height: 1024,
    size: '1024×1024 PNG',
    width: 1024,
  },
  xiaohongshu: {
    aspect: 'xiaohongshu',
    height: 1408,
    size: '1056×1408 PNG',
    width: 1056,
  },
  landscape: {
    aspect: 'landscape',
    height: 864,
    size: '1536×864 PNG',
    width: 1536,
  },
  portrait: {
    aspect: 'portrait',
    height: 1536,
    size: '864×1536 PNG',
    width: 864,
  },
}

export type ImageGenerationResponse = {
  data?: Array<{ b64_json?: string }>
}

export type GeneratedImage = {
  aspect: ImageAspect
  blob: Blob
  generationId: string
  height: number
  imageUrl: string
  size: string
  width: number
}

export type ImageGenerationState =
  | { status: 'idle'; imageUrl: null; error: null }
  | {
      status: 'generating'
      aspect: ImageAspect
      imageUrl: null
      error: null
    }
  | {
      status: 'success'
      imageUrl: string
      error: null
      generatedImage: GeneratedImage
    }
  | { status: 'error'; imageUrl: null; error: string }
