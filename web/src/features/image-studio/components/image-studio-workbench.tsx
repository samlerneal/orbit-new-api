import { type ReactNode, useCallback, useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { useAuthStore } from '@/stores/auth-store'

import {
  useImageGeneration,
  type ImageGenerationRequest,
} from '../hooks/use-image-generation'
import { useImageHistory } from '../hooks/use-image-history'
import type { ImageHistoryItem } from '../lib/image-history'
import { imageAspectMetadata, type ImageAspect } from '../types'

const examples = [
  'A sunlit reading corner with plants',
  'A blue paper-cut mountain landscape',
  'A small robot watering a garden',
  'A quiet lake under a star-filled sky',
]

const configurationSections = [
  {
    id: 'resolution',
    label: 'Resolution',
    options: ['1K', '2K', '4K'],
    selected: '1K',
  },
]

const aspectOptions: Array<{
  aspect: ImageAspect
  available: boolean
  label: string
}> = [
  { aspect: 'square', available: true, label: 'Square image' },
  { aspect: 'landscape', available: true, label: 'Landscape' },
  { aspect: 'portrait', available: false, label: 'Portrait' },
]

function aspectFrameClass(aspect: ImageAspect): string {
  if (aspect === 'xiaohongshu') return 'aspect-[3/4]'
  if (aspect === 'landscape') return 'aspect-[3/2]'
  if (aspect === 'portrait') return 'aspect-[2/3]'
  return 'aspect-square'
}

interface ImageStudioWorkbenchProps {
  embedded?: boolean
  requestImage: ImageGenerationRequest
}

interface DisabledOptionProps {
  id: string
  label: string
  tooltip: string
  fullWidth?: boolean
}

function DisabledOption(props: DisabledOptionProps) {
  const tooltipId = `image-studio-option-${props.id}`

  return (
    <span
      className={`group relative inline-flex${props.fullWidth ? ' w-full' : ''}`}
      tabIndex={0}
      aria-describedby={tooltipId}
    >
      <button
        className={`border-border/70 text-muted-foreground min-h-8 cursor-not-allowed rounded-md border px-2 text-xs opacity-60${props.fullWidth ? ' w-full' : ''}`}
        type='button'
        disabled
        aria-disabled='true'
      >
        {props.label}
      </button>
      <span
        className='bg-popover text-popover-foreground pointer-events-none absolute bottom-[calc(100%+0.45rem)] left-1/2 z-20 w-max max-w-48 -translate-x-1/2 rounded-md border px-2 py-1 text-center text-xs opacity-0 shadow-lg transition-opacity group-hover:opacity-100 group-focus:opacity-100'
        id={tooltipId}
        role='tooltip'
      >
        {props.tooltip}
      </span>
    </span>
  )
}

function useHistoryObjectUrls(history: ImageHistoryItem[]) {
  const [urls, setUrls] = useState<Record<string, string>>({})

  useEffect(() => {
    const nextUrls = Object.fromEntries(
      history.map((item) => [item.id, URL.createObjectURL(item.blob)])
    )
    setUrls(nextUrls)
    return () => {
      Object.values(nextUrls).forEach((url) => URL.revokeObjectURL(url))
    }
  }, [history])

  return urls
}

interface ImageHistoryUrlScopeProps {
  children: (historyUrls: Record<string, string>) => ReactNode
  history: ImageHistoryItem[]
}

export function ImageHistoryUrlScope(props: ImageHistoryUrlScopeProps) {
  const historyUrls = useHistoryObjectUrls(props.history)
  return props.children(historyUrls)
}

interface ImageHistoryPanelProps {
  history: ImageHistoryItem[]
  historyUrls: Record<string, string>
  onClear: () => void
  onRemove: (id: string) => void
  onSelect: (id: string) => void
  saveWarning: boolean
  selectedHistoryId: string | null
}

export function ImageHistoryPanel(props: ImageHistoryPanelProps) {
  const { t } = useTranslation()

  return (
    <section
      className='border-border/70 mt-4 min-w-0 border-t pt-3'
      aria-label={t('Image history')}
    >
      <div className='mb-2 flex items-center justify-between gap-3'>
        <div>
          <h3 className='text-sm font-medium'>{t('Image history')}</h3>
          {props.saveWarning ? (
            <p className='text-muted-foreground text-xs' role='status'>
              {t('This image was not saved to history.')}
            </p>
          ) : null}
        </div>
        {props.history.length > 0 ? (
          <Button
            variant='outline'
            className='min-h-9 px-3 text-xs'
            onClick={() => {
              if (window.confirm(t('Clear all image history?'))) {
                props.onClear()
              }
            }}
          >
            {t('Clear all')}
          </Button>
        ) : null}
      </div>
      {props.history.length === 0 ? (
        <p className='text-muted-foreground text-sm'>
          {t('Your saved images will appear here.')}
        </p>
      ) : (
        <div
          className='flex min-w-0 gap-2 overflow-x-auto pb-1'
          data-image-history-list
        >
          {props.history.map((item) => (
            <div className='relative w-24 shrink-0' key={item.id}>
              <button
                className={`w-full overflow-hidden rounded-lg border ${props.selectedHistoryId === item.id ? 'border-primary ring-primary/30 ring-2' : 'border-border/70'}`}
                type='button'
                onClick={() => props.onSelect(item.id)}
                aria-pressed={props.selectedHistoryId === item.id}
                aria-label={`${t('View saved image')}: ${item.prompt.slice(0, 40)}`}
              >
                <img
                  className={`${aspectFrameClass(item.aspect)} w-full object-contain`}
                  src={props.historyUrls[item.id]}
                  alt={item.prompt}
                  loading='lazy'
                />
              </button>
              <p
                className='text-muted-foreground mt-1 truncate text-xs'
                title={item.prompt}
              >
                {item.prompt}
              </p>
              <p className='text-muted-foreground truncate text-[11px]'>
                {new Date(item.createdAt).toLocaleString()}
              </p>
              <button
                className='text-muted-foreground hover:text-destructive min-h-8 text-xs'
                type='button'
                onClick={() => props.onRemove(item.id)}
              >
                {t('Delete')}
              </button>
            </div>
          ))}
        </div>
      )}
    </section>
  )
}

export function ImageStudioWorkbench(props: ImageStudioWorkbenchProps) {
  const { t } = useTranslation()
  const [prompt, setPrompt] = useState('')
  const [aspect, setAspect] = useState<ImageAspect>('landscape')
  const authOwnerId = useAuthStore((state) => state.auth.user?.id ?? null)
  const authReady = useAuthStore(
    (state) => state.auth.bootstrapState === 'complete'
  )
  const generationOwnerRef = useRef<number | null>(null)
  const invalidateCurrentImageRef = useRef<() => void>(() => undefined)
  const [currentImageOwnerId, setCurrentImageOwnerId] = useState<number | null>(
    null
  )
  const [selectedHistoryId, setSelectedHistoryId] = useState<string | null>(
    null
  )
  const {
    addGeneratedImage,
    history,
    removeAllHistory,
    removeHistoryItem,
    saveWarning,
  } = useImageHistory()
  const historyUrls = useHistoryObjectUrls(history)
  const handleGeneratedImage = useCallback(
    (
      generatedImage: Parameters<typeof addGeneratedImage>[0],
      generatedPrompt: string
    ) => {
      const ownerIdAtRequest = generationOwnerRef.current
      const currentAuth = useAuthStore.getState().auth
      const currentOwnerId = currentAuth.user?.id ?? null
      if (
        currentAuth.bootstrapState !== 'complete' ||
        currentOwnerId === null ||
        ownerIdAtRequest === null ||
        ownerIdAtRequest !== currentOwnerId
      ) {
        invalidateCurrentImageRef.current()
        return
      }
      setCurrentImageOwnerId(ownerIdAtRequest)
      setSelectedHistoryId(null)
      void addGeneratedImage(generatedImage, generatedPrompt)
    },
    [addGeneratedImage]
  )
  const { state, generate, invalidateCurrentImage, stopWaiting } =
    useImageGeneration(props.requestImage, handleGeneratedImage)
  invalidateCurrentImageRef.current = invalidateCurrentImage
  useEffect(() => {
    if (state.status !== 'success') return
    if (
      authReady &&
      authOwnerId !== null &&
      currentImageOwnerId === authOwnerId
    ) {
      return
    }
    invalidateCurrentImage()
    setCurrentImageOwnerId(null)
  }, [
    authOwnerId,
    authReady,
    currentImageOwnerId,
    invalidateCurrentImage,
    state.status,
  ])
  useEffect(() => {
    setSelectedHistoryId((currentId) => {
      if (currentId && history.some((item) => item.id === currentId)) {
        return currentId
      }
      return history[0]?.id ?? null
    })
  }, [history])
  const selectedHistoryItem = history.find(
    (item) => item.id === selectedHistoryId
  )
  const selectedHistoryUrl = selectedHistoryItem
    ? historyUrls[selectedHistoryItem.id]
    : null
  const currentImageUrl =
    authReady && authOwnerId !== null && currentImageOwnerId === authOwnerId
      ? state.imageUrl
      : null
  const displayedImageUrl = selectedHistoryUrl ?? currentImageUrl
  let displayedMetadata = imageAspectMetadata[aspect]
  if (state.status === 'generating') {
    displayedMetadata = imageAspectMetadata[state.aspect]
  } else if (state.status === 'success') {
    displayedMetadata = state.generatedImage
  }
  if (selectedHistoryItem) displayedMetadata = selectedHistoryItem
  const isGenerating = state.status === 'generating'
  const promptLength = [...prompt.trim()].length
  const isPromptValid = promptLength > 0 && promptLength <= 4000
  const disabledTooltip = t('This option is not available in the current beta.')
  let stageTitle = t('Ready')
  let stageContent = (
    <div className='relative z-10 flex max-w-md flex-col items-center text-center'>
      <div className='mb-5 grid size-16 place-items-center rounded-2xl border border-emerald-400/35 bg-emerald-400/10 shadow-[0_0_50px_rgb(16_185_129/.22)]'>
        <span className='grid size-8 place-items-center rounded-lg border border-emerald-300/70'>
          <span className='size-2 rounded-full bg-emerald-300' />
        </span>
      </div>
      <h2 className='text-xl font-semibold tracking-tight'>
        {t('AI image studio')}
      </h2>
      <p className='text-muted-foreground mt-3 text-sm leading-6'>
        {t('Create visuals from a prompt with the current beta image recipe.')}
      </p>
      <div className='mt-6 flex flex-wrap justify-center gap-2'>
        {examples.map((example) => (
          <span
            className='bg-background/60 text-muted-foreground border-border/80 rounded-full border px-3 py-1.5 text-xs'
            key={example}
          >
            {t(example)}
          </span>
        ))}
      </div>
    </div>
  )
  if (state.status === 'generating') {
    stageTitle = t('Generating your image…')
    stageContent = (
      <div className='text-muted-foreground relative z-10 space-y-4 text-center'>
        <div className='border-primary mx-auto size-10 animate-spin rounded-full border-2 border-t-transparent' />
        <p className='text-sm'>{t('Generating your image…')}</p>
      </div>
    )
  } else if (state.status === 'error') {
    stageTitle = t('Image studio')
    stageContent = (
      <div className='text-muted-foreground relative z-10 max-w-xs space-y-3 text-center'>
        <div className='border-destructive/30 bg-destructive/10 text-destructive mx-auto grid size-11 place-items-center rounded-2xl border text-lg'>
          !
        </div>
        <p className='text-sm leading-6'>{t(state.error)}</p>
      </div>
    )
  } else if (displayedImageUrl) {
    stageTitle = t('Generated image')
    stageContent = (
      <div className='relative z-10 flex w-full max-w-[min(100%,38rem)] flex-col gap-4'>
        <div
          className={`${aspectFrameClass(displayedMetadata.aspect)} w-full overflow-hidden rounded-xl border`}
        >
          <img
            className='size-full object-contain shadow-2xl'
            src={displayedImageUrl}
            alt={t('Generated image')}
          />
        </div>
        <a
          className='bg-primary text-primary-foreground inline-flex min-h-11 items-center justify-center rounded-lg px-4 py-2 text-sm font-medium shadow-[0_12px_30px_hsl(var(--primary)/.22)]'
          href={displayedImageUrl}
          download='image-studio.png'
        >
          {t('Download image')}
        </a>
        {selectedHistoryItem ? (
          <Button
            variant='outline'
            className='min-h-11'
            onClick={() => {
              setSelectedHistoryId(null)
              void removeHistoryItem(selectedHistoryItem.id)
            }}
          >
            {t('Delete')}
          </Button>
        ) : null}
      </div>
    )
  }

  return (
    <main
      className={
        props.embedded
          ? 'size-full overflow-x-hidden overflow-y-auto bg-[radial-gradient(circle_at_76%_8%,hsl(var(--primary)/.22),transparent_28rem),radial-gradient(circle_at_15%_87%,hsl(var(--primary)/.1),transparent_24rem),linear-gradient(hsl(var(--border)/.25)_1px,transparent_1px),linear-gradient(90deg,hsl(var(--border)/.25)_1px,transparent_1px)] bg-[size:auto,auto,32px_32px,32px_32px] px-3 py-5 sm:px-5 md:px-6'
          : 'min-h-[calc(100vh-4rem)] overflow-x-hidden bg-[radial-gradient(circle_at_76%_8%,hsl(var(--primary)/.22),transparent_28rem),radial-gradient(circle_at_15%_87%,hsl(var(--primary)/.1),transparent_24rem),linear-gradient(hsl(var(--border)/.25)_1px,transparent_1px),linear-gradient(90deg,hsl(var(--border)/.25)_1px,transparent_1px)] bg-[size:auto,auto,32px_32px,32px_32px] px-3 pt-14 pb-8 sm:px-5 md:px-6 md:pt-16'
      }
    >
      <div
        className='@container/image-studio mx-auto max-w-7xl'
        data-image-studio-page
      >
        <div
          className='grid min-w-0 gap-4 @[768px]/image-studio:grid-cols-[340px_minmax(0,1fr)] @[768px]/image-studio:gap-5'
          data-image-studio-layout
        >
          <section
            className='bg-background/88 min-w-0 rounded-2xl border border-emerald-500/25 p-3 shadow-[0_18px_55px_hsl(var(--background)/.35)] backdrop-blur-xl sm:p-4'
            data-image-studio-config
          >
            <div className='border-border/70 mb-2 flex items-start justify-between border-b pb-2'>
              <div className='min-w-0'>
                <p className='text-muted-foreground text-xs font-medium tracking-[0.18em] uppercase'>
                  {t('Image studio')}
                </p>
                <h1 className='mt-0.5 text-lg font-semibold tracking-tight'>
                  {t('Turn ideas into images')}
                </h1>
              </div>
              <span className='rounded-full bg-emerald-500/15 px-2 py-1 text-xs font-medium text-emerald-700 dark:text-emerald-300'>
                {t('Beta')}
              </span>
            </div>

            <div className='border-border/80 bg-muted/25 mb-2 grid grid-cols-2 rounded-lg border p-1'>
              <button
                className='bg-background text-foreground min-h-11 rounded-md px-2 text-xs font-medium shadow-sm'
                type='button'
                aria-pressed='true'
              >
                {t('Text to image')}
              </button>
              <DisabledOption
                id='mode-image-to-image'
                label={t('Image to image')}
                tooltip={disabledTooltip}
                fullWidth
              />
            </div>

            <div className='mb-2'>
              <div className='mb-1 flex items-center justify-between gap-3'>
                <label
                  className='block text-sm font-medium'
                  htmlFor='image-prompt'
                >
                  {t('Describe your image')}
                </label>
                <button
                  className='text-muted-foreground hover:text-foreground min-h-11 rounded-md px-1.5 text-xs transition focus-visible:ring-2 focus-visible:ring-emerald-400 focus-visible:outline-none'
                  type='button'
                  onClick={() => setPrompt('')}
                >
                  {t('Clear prompt')}
                </button>
              </div>
              <textarea
                id='image-prompt'
                value={prompt}
                onChange={(event) => setPrompt(event.target.value)}
                className='bg-background/70 focus:border-primary/70 focus:ring-primary/25 border-border/80 min-h-20 w-full resize-y rounded-lg border p-3 text-sm leading-5 transition outline-none focus:ring-4'
                placeholder={t('Describe a scene, subject, style, or mood')}
              />
              <p className='text-muted-foreground mt-0.5 text-right text-xs'>
                {promptLength}/4000
              </p>
              {!isPromptValid && promptLength > 4000 ? (
                <p className='text-destructive mt-1 text-sm' role='status'>
                  {t('Prompt must be 1 to 4,000 characters.')}
                </p>
              ) : null}
            </div>

            <div
              className='border-border/70 grid grid-cols-2 gap-1.5 border-t pt-2'
              aria-label={t('Prompt examples')}
            >
              {examples.map((example) => (
                <button
                  type='button'
                  key={example}
                  onClick={() => setPrompt(t(example))}
                  className='hover:border-primary/60 hover:bg-primary/5 focus:ring-primary/30 border-border/80 min-h-11 truncate rounded-lg border px-2.5 text-left text-xs leading-5 transition focus:ring-4 focus:outline-none'
                  title={t(example)}
                  aria-label={t(example)}
                >
                  {t(example)}
                </button>
              ))}
            </div>

            <div
              className='border-border/70 mt-2 space-y-2 border-t pt-2'
              data-image-studio-fixed-settings
            >
              <div className='grid grid-cols-2 gap-2'>
                <section>
                  <label
                    className='text-muted-foreground mb-1 block text-xs font-medium tracking-[0.12em] uppercase'
                    htmlFor='image-model'
                  >
                    {t('Model')}
                  </label>
                  <select
                    className='border-primary/50 bg-primary/10 text-foreground min-h-11 w-full rounded-md border px-2 text-xs font-medium focus-visible:ring-2 focus-visible:ring-emerald-400 focus-visible:outline-none'
                    defaultValue='gpt-image-2'
                    id='image-model'
                    name='image-model'
                  >
                    <option value='gpt-image-2'>gpt-image-2</option>
                  </select>
                </section>
                <section>
                  <p className='text-muted-foreground mb-1 text-xs font-medium tracking-[0.12em] uppercase'>
                    {t('Quantity')}
                  </p>
                  <div className='flex gap-1.5'>
                    <span className='border-primary/50 bg-primary/10 text-foreground inline-flex min-h-8 items-center rounded-md border px-2 text-xs font-medium'>
                      1
                    </span>
                    {['2', '4'].map((option) => (
                      <DisabledOption
                        key={option}
                        id={`quantity-${option}`}
                        label={option}
                        tooltip={disabledTooltip}
                      />
                    ))}
                  </div>
                </section>
              </div>
              <section>
                <p className='text-muted-foreground mb-1 text-xs font-medium tracking-[0.12em] uppercase'>
                  {t('Aspect ratio')}
                </p>
                <div className='grid grid-cols-4 gap-1.5'>
                  {aspectOptions.map((option) =>
                    option.available ? (
                      <button
                        className={`min-h-8 rounded-md border px-2 text-xs font-medium ${aspect === option.aspect ? 'border-primary/50 bg-primary/10 text-foreground' : 'border-border/70 text-muted-foreground hover:border-primary/60'}`}
                        type='button'
                        key={option.aspect}
                        onClick={() => setAspect(option.aspect)}
                        aria-pressed={aspect === option.aspect}
                      >
                        {t(option.label)}
                      </button>
                    ) : (
                      <DisabledOption
                        key={option.aspect}
                        id={`aspect-${option.aspect}`}
                        label={t(option.label)}
                        tooltip={disabledTooltip}
                        fullWidth
                      />
                    )
                  )}
                  <DisabledOption
                    id='aspect-Custom'
                    label={t('Custom')}
                    tooltip={disabledTooltip}
                    fullWidth
                  />
                </div>
              </section>
              {configurationSections.map((section) => (
                <section key={section.label}>
                  <p className='text-muted-foreground mb-1 text-xs font-medium tracking-[0.12em] uppercase'>
                    {t(section.label)}
                  </p>
                  <div className='flex gap-1.5'>
                    {section.options.map((option) =>
                      option === section.selected ? (
                        <span
                          className='border-primary/50 bg-primary/10 text-foreground inline-flex min-h-8 items-center justify-center rounded-md border px-2 text-xs font-medium'
                          key={option}
                        >
                          {t(option)}
                        </span>
                      ) : (
                        <DisabledOption
                          key={option}
                          id={`${section.id}-${option}`}
                          label={t(option)}
                          tooltip={disabledTooltip}
                        />
                      )
                    )}
                  </div>
                </section>
              ))}
              <div className='grid grid-cols-2 gap-2'>
                <section>
                  <p className='text-muted-foreground mb-1 text-xs font-medium tracking-[0.12em] uppercase'>
                    {t('Format')}
                  </p>
                  <span className='border-primary/50 bg-primary/10 text-foreground inline-flex min-h-8 w-full items-center justify-center rounded-md border px-2 text-xs font-medium'>
                    PNG
                  </span>
                </section>
                <section>
                  <p className='text-muted-foreground mb-1 text-xs font-medium tracking-[0.12em] uppercase'>
                    {t('Background')}
                  </p>
                  <div className='grid grid-cols-2 gap-1.5'>
                    <span className='border-primary/50 bg-primary/10 text-foreground inline-flex min-h-8 items-center justify-center rounded-md border px-2 text-xs font-medium'>
                      {t('Opaque')}
                    </span>
                    <DisabledOption
                      id='background-Transparent'
                      label={t('Transparent')}
                      tooltip={disabledTooltip}
                      fullWidth
                    />
                  </div>
                </section>
              </div>
            </div>

            {isGenerating ? (
              <Button
                className='mt-4 min-h-11 w-full rounded-xl'
                variant='outline'
                onClick={stopWaiting}
              >
                {t('Stop waiting')}
              </Button>
            ) : (
              <Button
                className='mt-4 min-h-11 w-full rounded-xl bg-emerald-500 text-slate-950 shadow-[0_14px_34px_rgb(16_185_129/.24)] hover:bg-emerald-400 focus-visible:ring-emerald-500/50'
                disabled={!isPromptValid}
                onClick={() => {
                  generationOwnerRef.current = authReady ? authOwnerId : null
                  setCurrentImageOwnerId(null)
                  void generate(prompt, aspect)
                }}
              >
                {t('Generate now')}
              </Button>
            )}
            <p className='mt-2 flex gap-1 text-xs leading-5'>
              <strong className='shrink-0 font-medium text-amber-600 dark:text-amber-300'>
                {t('Billing notice')}
              </strong>
              <span className='text-muted-foreground'>
                {t('Image generation billing notice')}
              </span>
            </p>
          </section>

          <section
            className='bg-background/78 border-primary/20 flex min-h-[430px] min-w-0 flex-col rounded-2xl border p-3 shadow-[0_18px_55px_hsl(var(--background)/.28)] backdrop-blur-xl sm:p-5'
            data-image-studio-stage
            aria-live='polite'
          >
            <div className='border-border/70 mb-4 border-b px-1 pb-4'>
              <div>
                <p className='text-muted-foreground text-xs tracking-[0.15em] uppercase'>
                  {t('Generation result')}
                </p>
                <h2 className='mt-1 text-sm font-medium'>{stageTitle}</h2>
              </div>
            </div>
            <div
              className={
                displayedImageUrl
                  ? 'border-border/90 ring-primary/10 relative flex w-full max-w-[min(100%,42rem)] flex-col items-center justify-center self-center rounded-xl border border-dashed bg-[radial-gradient(circle_at_50%_38%,hsl(var(--primary)/.16),transparent_34%),linear-gradient(hsl(var(--border)/.3)_1px,transparent_1px),linear-gradient(90deg,hsl(var(--border)/.3)_1px,transparent_1px)] bg-[size:auto,22px_22px,22px_22px] p-5 ring-1 ring-inset'
                  : `border-border/90 ring-primary/10 relative flex ${aspectFrameClass(displayedMetadata.aspect)} w-full max-w-[min(100%,42rem)] items-center justify-center self-center overflow-hidden rounded-xl border border-dashed bg-[radial-gradient(circle_at_50%_38%,hsl(var(--primary)/.16),transparent_34%),linear-gradient(hsl(var(--border)/.3)_1px,transparent_1px),linear-gradient(90deg,hsl(var(--border)/.3)_1px,transparent_1px)] bg-[size:auto,22px_22px,22px_22px] p-5 ring-1 ring-inset`
              }
            >
              {stageContent}
            </div>
            <ImageHistoryPanel
              history={history}
              historyUrls={historyUrls}
              onClear={() => void removeAllHistory()}
              onRemove={(id) => {
                if (selectedHistoryId === id) setSelectedHistoryId(null)
                void removeHistoryItem(id)
              }}
              onSelect={setSelectedHistoryId}
              saveWarning={saveWarning}
              selectedHistoryId={selectedHistoryId}
            />
          </section>
        </div>
      </div>
    </main>
  )
}
