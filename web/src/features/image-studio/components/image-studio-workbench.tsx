import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'

import {
  useImageGeneration,
  type ImageGenerationRequest,
} from '../hooks/use-image-generation'

const examples = [
  'A sunlit reading corner with plants',
  'A blue paper-cut mountain landscape',
  'A small robot watering a garden',
]

interface ImageStudioWorkbenchProps {
  requestImage: ImageGenerationRequest
}

export function ImageStudioWorkbench({
  requestImage,
}: ImageStudioWorkbenchProps) {
  const { t } = useTranslation()
  const [prompt, setPrompt] = useState('')
  const { state, generate, stopWaiting } = useImageGeneration(requestImage)
  const isGenerating = state.status === 'generating'
  const promptLength = [...prompt.trim()].length
  const isPromptValid = promptLength > 0 && promptLength <= 4000
  let stageTitle = t('Generated image')
  let resultContent = (
    <div className='text-muted-foreground max-w-sm space-y-3 text-center'>
      <div className='border-primary/25 bg-primary/10 mx-auto size-12 rounded-full border' />
      <p>{t('Your generated image will appear here.')}</p>
    </div>
  )
  if (state.status === 'generating') {
    stageTitle = t('Generating your image…')
    resultContent = (
      <div className='text-muted-foreground max-w-sm space-y-3 text-center'>
        <div className='border-primary mx-auto size-9 animate-spin rounded-full border-2 border-t-transparent' />
        <p>{t('Generating your image…')}</p>
      </div>
    )
  } else if (state.status === 'error') {
    stageTitle = t('Image studio')
    resultContent = (
      <div className='text-muted-foreground max-w-sm space-y-3 text-center'>
        <p>{t(state.error)}</p>
      </div>
    )
  } else if (state.status === 'success') {
    resultContent = (
      <div className='w-full max-w-2xl'>
        <img
          className='w-full rounded-lg border'
          src={state.imageUrl}
          alt={t('Generated image')}
        />
        <a
          className='bg-primary text-primary-foreground mt-4 inline-flex rounded-md px-4 py-2 text-sm font-medium'
          href={state.imageUrl}
          download='image-studio.png'
        >
          {t('Download image')}
        </a>
      </div>
    )
  }

  return (
    <main className='min-h-[calc(100vh-4rem)] bg-[radial-gradient(circle_at_75%_15%,rgb(16_185_129/.13),transparent_28%),linear-gradient(hsl(var(--border)/.3)_1px,transparent_1px),linear-gradient(90deg,hsl(var(--border)/.3)_1px,transparent_1px)] bg-[size:auto,28px_28px,28px_28px] p-4 pt-20 md:p-6 md:pt-24'>
      <div className='mx-auto max-w-7xl' data-image-studio-page>
        <header className='mb-5 max-w-2xl space-y-2'>
          <div className='text-primary text-sm font-medium tracking-wide uppercase'>
            {t('Beta')}
          </div>
          <h1 className='text-3xl font-semibold tracking-tight'>
            {t('Image studio')}
          </h1>
          <p className='text-muted-foreground text-sm'>
            {t(
              'Fixed beta settings: gpt-image-2 · 1024×1024 · 1 image · low · PNG'
            )}
          </p>
        </header>
        <div
          className='grid min-w-0 gap-4 lg:grid-cols-[390px_minmax(0,1fr)]'
          data-image-studio-layout
        >
          <section
            className='bg-background/90 rounded-xl border border-emerald-500/20 p-5 shadow-sm backdrop-blur'
            data-image-studio-config
          >
            <div className='mb-5 flex items-center justify-between border-b pb-4'>
              <h2 className='text-lg font-semibold'>
                {t('Describe your image')}
              </h2>
              <span className='rounded-full bg-emerald-500/15 px-2 py-1 text-xs font-medium text-emerald-700 dark:text-emerald-300'>
                {t('Beta')}
              </span>
            </div>
            <label
              className='mb-2 block text-sm font-medium'
              htmlFor='image-prompt'
            >
              {t('Describe your image')}
            </label>
            <textarea
              id='image-prompt'
              value={prompt}
              onChange={(event) => setPrompt(event.target.value)}
              className='bg-background focus:ring-primary min-h-40 w-full rounded-lg border p-3 outline-none focus:ring-2'
              placeholder={t('Describe a scene, subject, style, or mood')}
            />
            <p className='text-muted-foreground mt-1 text-right text-xs'>
              {promptLength}/4000
            </p>
            {!isPromptValid && promptLength > 4000 ? (
              <p className='text-destructive mt-1 text-sm' role='status'>
                {t('Prompt must be 1 to 4,000 characters.')}
              </p>
            ) : null}
            <div className='mt-5 space-y-2 border-t pt-5'>
              {examples.map((example) => (
                <button
                  type='button'
                  key={example}
                  onClick={() => setPrompt(example)}
                  className='hover:border-primary focus:ring-primary w-full rounded-lg border px-3 py-2 text-left text-sm transition focus:ring-2 focus:outline-none'
                >
                  {t(example)}
                </button>
              ))}
            </div>
            <div
              className='bg-muted/40 text-muted-foreground mt-5 rounded-lg border p-3 text-sm'
              data-image-studio-fixed-settings
            >
              {t(
                'Fixed beta settings: gpt-image-2 · 1024×1024 · 1 image · low · PNG'
              )}
            </div>
            {isGenerating ? (
              <Button
                className='mt-5 w-full'
                variant='outline'
                onClick={stopWaiting}
              >
                {t('Stop waiting')}
              </Button>
            ) : (
              <Button
                className='mt-5 w-full bg-emerald-600 text-white hover:bg-emerald-700 focus-visible:ring-emerald-500/50'
                disabled={!isPromptValid}
                onClick={() => void generate(prompt)}
              >
                {t('Generate image')}
              </Button>
            )}
          </section>
          <section
            className='bg-background/90 border-primary/15 flex min-h-[420px] min-w-0 flex-col rounded-xl border p-5 shadow-sm backdrop-blur'
            data-image-studio-stage
          >
            <div className='mb-4 flex items-center justify-between border-b pb-4'>
              <h2 className='text-sm font-medium'>{stageTitle}</h2>
              <span className='text-muted-foreground text-xs'>{t('Beta')}</span>
            </div>
            <div className='bg-muted/20 flex min-h-80 flex-1 items-center justify-center rounded-lg border border-dashed p-5'>
              {resultContent}
            </div>
          </section>
        </div>
      </div>
    </main>
  )
}
