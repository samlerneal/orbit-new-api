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
import { useCallback, useState } from 'react'
import { useTranslation } from 'react-i18next'

import {
  Conversation,
  ConversationContent,
  ConversationScrollButton,
} from '@/components/ai-elements/conversation'
import { Message } from '@/components/ai-elements/message'

import { PublicChatInput } from './components/input/public-chat-input'
import { PlaygroundMessageContent } from './components/message/playground-message-content'
import { DEFAULT_CONFIG, DEFAULT_PARAMETER_ENABLED } from './constants'
import { useChatHandler, usePlaygroundOptions } from './hooks'
import {
  appendUserMessagePair,
  getMessageAlignment,
  getMessageContent,
} from './lib'
import type {
  GroupOption,
  Message as ChatMessage,
  ModelOption,
  PlaygroundConfig,
} from './types'

export function PublicChat() {
  const { t } = useTranslation()
  const [config, setConfig] = useState<PlaygroundConfig>(() => ({
    ...DEFAULT_CONFIG,
  }))
  const [messages, setMessages] = useState<ChatMessage[]>([])
  const [models, setModels] = useState<ModelOption[]>([])
  const [, setGroups] = useState<GroupOption[]>([])

  const updateConfig = useCallback(
    <K extends keyof PlaygroundConfig>(key: K, value: PlaygroundConfig[K]) => {
      setConfig((currentConfig) => ({ ...currentConfig, [key]: value }))
    },
    []
  )

  const updateMessages = useCallback(
    (updater: (currentMessages: ChatMessage[]) => ChatMessage[]) => {
      setMessages((currentMessages) => updater(currentMessages))
    },
    []
  )

  const { isLoadingModels } = usePlaygroundOptions({
    currentGroup: config.group,
    currentModel: config.model,
    setGroups,
    setModels,
    updateConfig,
  })
  const { sendChat, stopGeneration, isGenerating } = useChatHandler({
    config,
    parameterEnabled: { ...DEFAULT_PARAMETER_ENABLED },
    onMessageUpdate: updateMessages,
  })

  const handleSubmit = useCallback(
    (text: string) => {
      const nextMessages = appendUserMessagePair(messages, text)
      setMessages(nextMessages)
      sendChat(nextMessages)
    },
    [messages, sendChat]
  )

  return (
    <div className='mx-auto flex min-h-[calc(100svh-7rem)] w-full max-w-4xl flex-col px-4 py-6 md:min-h-[calc(100svh-5rem)]'>
      <Conversation className='mb-4'>
        <ConversationContent className='p-0'>
          {messages.length === 0 ? (
            <div className='flex min-h-[min(420px,calc(100svh-20rem))] items-center justify-center px-4 text-center'>
              <h1 className='text-2xl font-semibold tracking-tight md:text-3xl'>
                {t('Start a conversation')}
              </h1>
            </div>
          ) : (
            <div className='space-y-3 py-2'>
              {messages.map((message) => (
                <Message
                  className='py-1.5'
                  from={message.from}
                  key={message.key}
                >
                  <div className='w-full min-w-0 flex-1'>
                    <PlaygroundMessageContent
                      actions={null}
                      alignment={getMessageAlignment(message, 'alternating')}
                      message={message}
                      versionContent={getMessageContent(message)}
                    />
                  </div>
                </Message>
              ))}
            </div>
          )}
        </ConversationContent>
        <ConversationScrollButton />
      </Conversation>
      <div className='shrink-0'>
        <PublicChatInput
          disabled={isLoadingModels}
          isGenerating={isGenerating}
          isModelLoading={isLoadingModels}
          models={models}
          modelValue={config.model}
          onModelChange={(model) => updateConfig('model', model)}
          onStop={stopGeneration}
          onSubmit={handleSubmit}
        />
      </div>
    </div>
  )
}
