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
  'A quiet lake under a star-filled sky',
]

const configurationSections = [
  {
    id: 'aspect',
    label: 'Aspect ratio',
    options: ['1:1', 'Landscape', 'Portrait', 'Custom'],
    selected: '1:1',
  },
  {
    id: 'resolution',
    label: 'Resolution',
    options: ['1K', '2K', '4K'],
    selected: '1K',
  },
]

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

export function ImageStudioWorkbench(props: ImageStudioWorkbenchProps) {
  const { t } = useTranslation()
  const [prompt, setPrompt] = useState('')
  const { state, generate, stopWaiting } = useImageGeneration(
    props.requestImage
  )
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
  } else if (state.status === 'success') {
    stageTitle = t('Generated image')
    stageContent = (
      <div className='relative z-10 flex w-full max-w-[min(100%,38rem)] flex-col gap-4'>
        <div className='aspect-square w-full overflow-hidden rounded-xl border'>
          <img
            className='size-full object-cover shadow-2xl'
            src={state.imageUrl}
            alt={t('Generated image')}
          />
        </div>
        <a
          className='bg-primary text-primary-foreground inline-flex min-h-11 items-center justify-center rounded-lg px-4 py-2 text-sm font-medium shadow-[0_12px_30px_hsl(var(--primary)/.22)]'
          href={state.imageUrl}
          download='image-studio.png'
        >
          {t('Download image')}
        </a>
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
      <div className='mx-auto max-w-7xl' data-image-studio-page>
        <div
          className='grid min-w-0 gap-4 min-[917px]:grid-cols-[340px_minmax(0,1fr)] lg:grid-cols-[390px_minmax(0,1fr)] lg:gap-5'
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
                  onClick={() => setPrompt(example)}
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
              {configurationSections.map((section) => (
                <section key={section.label}>
                  <p className='text-muted-foreground mb-1 text-xs font-medium tracking-[0.12em] uppercase'>
                    {t(section.label)}
                  </p>
                  <div
                    className={
                      section.id === 'aspect'
                        ? 'grid grid-cols-4 gap-1.5'
                        : 'flex gap-1.5'
                    }
                  >
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
                          fullWidth={section.id === 'aspect'}
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
                onClick={() => void generate(prompt)}
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
            <div className='border-border/70 mb-4 flex items-center justify-between border-b px-1 pb-4'>
              <div>
                <p className='text-muted-foreground text-xs tracking-[0.15em] uppercase'>
                  {t('Generation result')}
                </p>
                <h2 className='mt-1 text-sm font-medium'>{stageTitle}</h2>
              </div>
              <span className='text-muted-foreground text-xs'>1024×1024</span>
            </div>
            <div
              className={
                state.status === 'success'
                  ? 'border-border/90 ring-primary/10 relative flex w-full max-w-[min(100%,42rem)] flex-col items-center justify-center self-center rounded-xl border border-dashed bg-[radial-gradient(circle_at_50%_38%,hsl(var(--primary)/.16),transparent_34%),linear-gradient(hsl(var(--border)/.3)_1px,transparent_1px),linear-gradient(90deg,hsl(var(--border)/.3)_1px,transparent_1px)] bg-[size:auto,22px_22px,22px_22px] p-5 ring-1 ring-inset'
                  : 'border-border/90 ring-primary/10 relative flex aspect-square w-full max-w-[min(100%,42rem)] items-center justify-center self-center overflow-hidden rounded-xl border border-dashed bg-[radial-gradient(circle_at_50%_38%,hsl(var(--primary)/.16),transparent_34%),linear-gradient(hsl(var(--border)/.3)_1px,transparent_1px),linear-gradient(90deg,hsl(var(--border)/.3)_1px,transparent_1px)] bg-[size:auto,22px_22px,22px_22px] p-5 ring-1 ring-inset'
              }
            >
              {stageContent}
            </div>
          </section>
        </div>
      </div>
    </main>
  )
}
