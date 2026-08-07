/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import { SendIcon, SquareIcon } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import {
  PromptInput,
  PromptInputButton,
  PromptInputFooter,
  PromptInputTextarea,
  type PromptInputMessage,
} from '@/components/ai-elements/prompt-input'
import { ModelSelector } from '@/components/model-group-selector'

import { getSubmittableInputText } from '../../lib'
import type { ModelOption } from '../../types'

type PublicChatInputProps = {
  disabled?: boolean
  isGenerating: boolean
  isModelLoading: boolean
  modelValue: string
  models: ModelOption[]
  onModelChange: (value: string) => void
  onStop: () => void
  onSubmit: (text: string) => void
}

// The R2 behavior test imports this pure predicate without mounting browser UI.
// eslint-disable-next-line react/only-export-components
export function canSubmitPublicChatInput({
  disabled,
  hasModels,
  text,
}: {
  disabled: boolean
  hasModels: boolean
  text: string
}): boolean {
  return !disabled && hasModels && text.trim().length > 0
}

export function PublicChatInput({
  disabled = false,
  isGenerating,
  isModelLoading,
  modelValue,
  models,
  onModelChange,
  onStop,
  onSubmit,
}: PublicChatInputProps) {
  const { t } = useTranslation()
  const [text, setText] = useState('')
  const canSubmit = canSubmitPublicChatInput({
    disabled,
    hasModels: models.length > 0,
    text,
  })

  const handleSubmit = (message: PromptInputMessage) => {
    const submittedText = getSubmittableInputText(message, !canSubmit)
    if (!submittedText) return

    onSubmit(submittedText)
    setText('')
  }

  return (
    <PromptInput className='relative' onSubmit={handleSubmit}>
      <PromptInputTextarea
        aria-label={t('Message')}
        autoCapitalize='off'
        autoComplete='off'
        autoCorrect='off'
        className='min-h-24 px-4 pt-4 pb-3 leading-7 md:text-base'
        disabled={disabled || isGenerating}
        onChange={(event) => setText(event.target.value)}
        placeholder={t('Ask anything')}
        spellCheck={false}
        value={text}
      />
      <PromptInputFooter className='border-border/60 border-t px-3 py-2.5'>
        <div aria-label={t('Model')} className='min-w-0' role='group'>
          <ModelSelector
            disabled={disabled || isGenerating || isModelLoading}
            models={models}
            onModelChange={onModelChange}
            selectedModel={modelValue}
          />
        </div>
        {isGenerating ? (
          <PromptInputButton
            aria-label={t('Stop')}
            className='border-destructive/25 bg-destructive/10 text-destructive hover:bg-destructive/15'
            onClick={onStop}
            type='button'
            variant='secondary'
          >
            <SquareIcon className='fill-current' size={16} />
            <span>{t('Stop')}</span>
          </PromptInputButton>
        ) : (
          <PromptInputButton
            aria-label={t('Send')}
            className='bg-primary text-primary-foreground hover:bg-primary/90 disabled:bg-muted disabled:text-muted-foreground'
            disabled={!canSubmit}
            type='submit'
          >
            <SendIcon size={16} />
            <span>{t('Send')}</span>
          </PromptInputButton>
        )}
      </PromptInputFooter>
    </PromptInput>
  )
}
