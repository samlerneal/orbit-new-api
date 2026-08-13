export type ImageGenerationResponse = {
  data?: Array<{ b64_json?: string }>
}

export type GeneratedImage = {
  blob: Blob
  generationId: string
  imageUrl: string
}

export type ImageGenerationState =
  | { status: 'idle'; imageUrl: null; error: null }
  | { status: 'generating'; imageUrl: null; error: null }
  | {
      status: 'success'
      imageUrl: string
      error: null
      generatedImage: GeneratedImage
    }
  | { status: 'error'; imageUrl: null; error: string }
