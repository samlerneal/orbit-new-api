export type ImageGenerationResponse = {
  data?: Array<{ b64_json?: string }>
}

export type ImageGenerationState =
  | { status: 'idle'; imageUrl: null; error: null }
  | { status: 'generating'; imageUrl: null; error: null }
  | { status: 'success'; imageUrl: string; error: null }
  | { status: 'error'; imageUrl: null; error: string }
