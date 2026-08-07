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
import { MenuIcon } from 'lucide-react'
import { useCallback, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'

import {
  Conversation,
  ConversationContent,
  ConversationScrollButton,
} from '@/components/ai-elements/conversation'
import { Message } from '@/components/ai-elements/message'
import { ModelSelector } from '@/components/model-group-selector'
import { Button } from '@/components/ui/button'
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet'
import { useSystemConfig } from '@/hooks/use-system-config'

import { PublicChatInput } from './components/input/public-chat-input'
import { PlaygroundMessageContent } from './components/message/playground-message-content'
import { PublicChatSidebar } from './components/public-chat-sidebar'
import { PublicChatWelcome } from './components/public-chat-welcome'
import { DEFAULT_CONFIG, DEFAULT_PARAMETER_ENABLED } from './constants'
import { useChatHandler, usePlaygroundOptions } from './hooks'
import {
  appendUserMessagePair,
  getMessageAlignment,
  getMessageContent,
} from './lib'
import {
  createPublicChatSession,
  setPublicChatSessionMessages,
  updatePublicChatSessionMessages,
  type PublicChatSession,
} from './lib/public-chat-sessions'
import type {
  GroupOption,
  Message as ChatMessage,
  ModelOption,
  PlaygroundConfig,
} from './types'

type PublicChatToolbarProps = {
  disabled: boolean
  isModelLoading: boolean
  models: ModelOption[]
  modelValue: string
  onModelChange: (model: string) => void
  onOpenSessions: () => void
}

function PublicChatToolbar(props: PublicChatToolbarProps) {
  const { t } = useTranslation()
  return (
    <div className='bg-background/95 flex h-14 shrink-0 items-center gap-3 border-b px-3 backdrop-blur md:px-5'>
      <Button
        aria-label={t('Public chat open conversations')}
        className='md:hidden'
        onClick={props.onOpenSessions}
        size='icon-sm'
        variant='outline'
      >
        <MenuIcon className='size-4' />
      </Button>
      <div className='flex min-w-0 flex-1 items-center justify-between gap-3'>
        <span className='text-muted-foreground hidden text-sm sm:block'>
          {t('Model')}
        </span>
        <ModelSelector
          className='ml-auto'
          disabled={props.disabled || props.isModelLoading}
          models={props.models}
          onModelChange={props.onModelChange}
          selectedModel={props.modelValue}
        />
      </div>
    </div>
  )
}

type PublicChatMessagesProps = {
  disabled: boolean
  logo?: string
  messages: ChatMessage[]
  onPromptSelect: (prompt: string) => void
  systemName: string
}

function PublicChatMessages(props: PublicChatMessagesProps) {
  return (
    <Conversation className='min-h-0 flex-1'>
      <ConversationContent className='mx-auto w-full max-w-4xl px-4 py-2'>
        {props.messages.length === 0 ? (
          <PublicChatWelcome
            disabled={props.disabled}
            logo={props.logo}
            onPromptSelect={props.onPromptSelect}
            systemName={props.systemName}
          />
        ) : (
          <div className='space-y-3 py-4'>
            {props.messages.map((message) => (
              <Message className='py-1.5' from={message.from} key={message.key}>
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
  )
}

type PublicChatComposerProps = {
  disabled: boolean
  hasModels: boolean
  isGenerating: boolean
  onStop: () => void
  onSubmit: (text: string) => void
  onTextChange: (text: string) => void
  text: string
}

function PublicChatComposer(props: PublicChatComposerProps) {
  const { t } = useTranslation()
  return (
    <div className='bg-background/95 shrink-0 border-t px-3 py-3 backdrop-blur md:px-5'>
      <div className='mx-auto w-full max-w-4xl'>
        <p className='text-muted-foreground mb-2 text-center text-xs leading-5'>
          {t('Public chat billing reminder')}
        </p>
        <PublicChatInput
          disabled={props.disabled}
          hasModels={props.hasModels}
          isGenerating={props.isGenerating}
          onStop={props.onStop}
          onSubmit={props.onSubmit}
          onTextChange={props.onTextChange}
          text={props.text}
        />
      </div>
    </div>
  )
}

export function PublicChat() {
  const { t } = useTranslation()
  const { logo, systemName } = useSystemConfig()
  const [config, setConfig] = useState<PlaygroundConfig>(() => ({
    ...DEFAULT_CONFIG,
  }))
  const [sessions, setSessions] = useState<PublicChatSession[]>(() => [
    createPublicChatSession(1),
  ])
  const [activeSessionId, setActiveSessionId] = useState('chat-1')
  const nextSessionOrdinal = useRef(2)
  const [draftText, setDraftText] = useState('')
  const [models, setModels] = useState<ModelOption[]>([])
  const [, setGroups] = useState<GroupOption[]>([])
  const [mobileSessionsOpen, setMobileSessionsOpen] = useState(false)
  const activeSession =
    sessions.find((session) => session.id === activeSessionId) ?? sessions[0]

  const updateConfig = useCallback(
    <K extends keyof PlaygroundConfig>(key: K, value: PlaygroundConfig[K]) => {
      setConfig((currentConfig) => ({ ...currentConfig, [key]: value }))
    },
    []
  )

  const updateMessages = useCallback(
    (updater: (currentMessages: ChatMessage[]) => ChatMessage[]) => {
      setSessions((currentSessions) =>
        updatePublicChatSessionMessages(
          currentSessions,
          activeSessionId,
          updater
        )
      )
    },
    [activeSessionId]
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
      const nextMessages = appendUserMessagePair(activeSession.messages, text)
      setSessions((currentSessions) =>
        setPublicChatSessionMessages(
          currentSessions,
          activeSessionId,
          nextMessages,
          text
        )
      )
      sendChat(nextMessages)
    },
    [activeSession.messages, activeSessionId, sendChat]
  )

  const handleCreateSession = useCallback(() => {
    if (isGenerating) return
    const session = createPublicChatSession(nextSessionOrdinal.current)
    nextSessionOrdinal.current += 1
    setSessions((currentSessions) => [...currentSessions, session])
    setActiveSessionId(session.id)
    setDraftText('')
    setMobileSessionsOpen(false)
  }, [isGenerating])

  const handleSelectSession = useCallback(
    (sessionId: string) => {
      if (isGenerating) return
      setActiveSessionId(sessionId)
      setDraftText('')
      setMobileSessionsOpen(false)
    },
    [isGenerating]
  )

  const sidebar = (
    <PublicChatSidebar
      activeSessionId={activeSessionId}
      disabled={isGenerating}
      onCreateSession={handleCreateSession}
      onSelectSession={handleSelectSession}
      sessions={sessions}
    />
  )

  return (
    <div className='flex h-[calc(100svh-7rem)] min-h-0 overflow-hidden md:h-[calc(100svh-5rem)]'>
      <aside className='bg-muted/20 hidden w-64 shrink-0 border-r md:block'>
        {sidebar}
      </aside>
      <main className='flex min-w-0 flex-1 flex-col overflow-hidden'>
        <PublicChatToolbar
          disabled={isGenerating}
          isModelLoading={isLoadingModels}
          modelValue={config.model}
          models={models}
          onModelChange={(model) => updateConfig('model', model)}
          onOpenSessions={() => setMobileSessionsOpen(true)}
        />
        <PublicChatMessages
          disabled={isGenerating || isLoadingModels || models.length === 0}
          logo={logo || undefined}
          messages={activeSession.messages}
          onPromptSelect={setDraftText}
          systemName={systemName}
        />
        <PublicChatComposer
          disabled={isLoadingModels}
          hasModels={models.length > 0}
          isGenerating={isGenerating}
          onStop={stopGeneration}
          onSubmit={handleSubmit}
          onTextChange={setDraftText}
          text={draftText}
        />
      </main>
      <Sheet open={mobileSessionsOpen} onOpenChange={setMobileSessionsOpen}>
        <SheetContent className='w-72 p-0 sm:max-w-72' side='left'>
          <SheetHeader className='sr-only'>
            <SheetTitle>{t('Public chat conversations')}</SheetTitle>
            <SheetDescription>
              {t('Public chat stored in this browser tab')}
            </SheetDescription>
          </SheetHeader>
          {sidebar}
        </SheetContent>
      </Sheet>
    </div>
  )
}
