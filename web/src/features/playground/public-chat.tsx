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
import { useCallback, useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'

import {
  Conversation,
  ConversationContent,
  ConversationScrollButton,
} from '@/components/ai-elements/conversation'
import { Message } from '@/components/ai-elements/message'
import { Button } from '@/components/ui/button'
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet'
import { useSystemConfig } from '@/hooks/use-system-config'
import { useAuthStore } from '@/stores/auth-store'

import { PublicChatInput } from './components/input/public-chat-input'
import { PlaygroundMessageContent } from './components/message/playground-message-content'
import { PublicChatSidebar } from './components/public-chat-sidebar'
import { PublicChatWelcome } from './components/public-chat-welcome'
import { useChatHandler, usePlaygroundOptions } from './hooks'
import {
  canMutatePublicChatSession,
  usePublicChatState,
} from './hooks/use-public-chat-state'
import {
  appendUserMessagePair,
  filterPublicChatModels,
  getMessageAlignment,
  getMessageContent,
} from './lib'
import type {
  GroupOption,
  Message as ChatMessage,
  ModelOption,
  PlaygroundConfig,
} from './types'

type PublicChatToolbarProps = {
  onOpenSessions: () => void
}

function getPublicChatNoticeKey(
  notice: 'limit' | 'reset' | 'trimmed' | 'unavailable'
) {
  if (notice === 'limit') return 'Public chat session limit'
  if (notice === 'reset') return 'Public chat history reset'
  if (notice === 'trimmed') return 'Public chat history trimmed'
  return 'Public chat history unavailable'
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
      <div className='flex min-w-0 flex-1 items-center' />
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
  config: PlaygroundConfig
  groups: GroupOption[]
  models: ModelOption[]
  parameterEnabled: import('./types').ParameterEnabled
  onConfigChange: <K extends keyof PlaygroundConfig>(
    key: K,
    value: PlaygroundConfig[K]
  ) => void
  onParameterEnabledChange: (
    key: keyof import('./types').ParameterEnabled,
    value: boolean
  ) => void
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
          config={props.config}
          groups={props.groups}
          models={props.models}
          parameterEnabled={props.parameterEnabled}
          onConfigChange={props.onConfigChange}
          onParameterEnabledChange={props.onParameterEnabledChange}
        />
      </div>
    </div>
  )
}

type PublicChatState = ReturnType<typeof usePublicChatState>

type PublicChatReadyProps = {
  state: PublicChatState
}

type PublicChatSubmitTurnArgs = {
  activeSession: PublicChatState['activeSession']
  sendChat: (messages: ChatMessage[]) => void
  submitMessages: (messages: ChatMessage[], firstPrompt: string) => boolean
  text: string
}

// The DOM test uses this exact transition so persistence cannot be tested as a
// separate mock from the request that follows it.
// eslint-disable-next-line react/only-export-components
export function submitPublicChatTurn({
  activeSession,
  sendChat,
  submitMessages,
  text,
}: PublicChatSubmitTurnArgs): boolean {
  if (!activeSession) return false
  const nextMessages = appendUserMessagePair(activeSession.messages, text)
  if (!submitMessages(nextMessages, text)) return false
  sendChat(nextMessages)
  return true
}

// eslint-disable-next-line react/only-export-components
export function runPublicChatSessionMutation(
  isGenerating: boolean,
  mutation: () => void
): boolean {
  if (!canMutatePublicChatSession(isGenerating)) return false
  mutation()
  return true
}

export function PublicChatReady(props: PublicChatReadyProps) {
  const { t } = useTranslation()
  const { logo, systemName } = useSystemConfig()
  const {
    activeSession,
    activeSessionId,
    clearSessions,
    config,
    createSession,
    deleteSession,
    notice,
    parameterEnabled,
    sessions,
    setActiveSessionId,
    submitMessages,
    updateConfig,
    updateMessages,
    updateParameterEnabled,
  } = props.state
  const [draftText, setDraftText] = useState('')
  const [models, setModels] = useState<ModelOption[]>([])
  const [groups, setGroups] = useState<GroupOption[]>([])
  const [mobileSessionsOpen, setMobileSessionsOpen] = useState(false)

  const { isLoadingModels } = usePlaygroundOptions({
    currentGroup: config.group,
    currentModel: config.model,
    projectModels: filterPublicChatModels,
    setGroups,
    setModels,
    updateConfig,
  })
  const { sendChat, stopGeneration, isGenerating } = useChatHandler({
    config,
    parameterEnabled,
    onMessageUpdate: updateMessages,
  })
  const stopGenerationRef = useRef(stopGeneration)
  stopGenerationRef.current = stopGeneration

  useEffect(
    () => () => {
      stopGenerationRef.current()
    },
    []
  )

  const handleSubmit = useCallback(
    (text: string) => {
      submitPublicChatTurn({
        activeSession,
        sendChat,
        submitMessages,
        text,
      })
    },
    [activeSession, sendChat, submitMessages]
  )

  const handleCreateSession = useCallback(() => {
    if (isGenerating) return
    createSession()
    setDraftText('')
    setMobileSessionsOpen(false)
  }, [createSession, isGenerating])

  const handleSelectSession = useCallback(
    (sessionId: string) => {
      if (isGenerating) return
      setActiveSessionId(sessionId)
      setDraftText('')
      setMobileSessionsOpen(false)
    },
    [isGenerating, setActiveSessionId]
  )

  const sidebar = (
    <PublicChatSidebar
      activeSessionId={activeSessionId}
      disabled={isGenerating}
      onClearSessions={() => {
        runPublicChatSessionMutation(isGenerating, clearSessions)
      }}
      onCreateSession={handleCreateSession}
      onDeleteSession={(sessionId) => {
        runPublicChatSessionMutation(isGenerating, () =>
          deleteSession(sessionId)
        )
      }}
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
        <PublicChatToolbar onOpenSessions={() => setMobileSessionsOpen(true)} />
        <PublicChatMessages
          disabled={isGenerating || isLoadingModels || models.length === 0}
          logo={logo || undefined}
          messages={activeSession?.messages ?? []}
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
          config={config}
          groups={groups}
          models={models}
          parameterEnabled={parameterEnabled}
          onConfigChange={updateConfig}
          onParameterEnabledChange={updateParameterEnabled}
        />
      </main>
      <Sheet open={mobileSessionsOpen} onOpenChange={setMobileSessionsOpen}>
        <SheetContent className='w-72 p-0 sm:max-w-72' side='left'>
          <SheetHeader className='sr-only'>
            <SheetTitle>{t('Public chat conversations')}</SheetTitle>
            <SheetDescription>
              {t('Public chat stored in this browser')}
            </SheetDescription>
          </SheetHeader>
          {sidebar}
        </SheetContent>
      </Sheet>
      {notice && (
        <div
          className='bg-muted text-muted-foreground absolute right-3 bottom-3 max-w-[calc(100%-1.5rem)] rounded px-3 py-2 text-xs shadow'
          role='status'
        >
          {t(getPublicChatNoticeKey(notice))}
        </div>
      )}
    </div>
  )
}

export function PublicChat() {
  const userId = useAuthStore((state) => state.auth.user?.id)
  const authReady = useAuthStore(
    (state) => state.auth.bootstrapState === 'complete'
  )
  const publicChatState = usePublicChatState(userId, authReady)

  if (!publicChatState.identityReady || userId === undefined) {
    return (
      <div className='flex h-[calc(100svh-7rem)] min-h-0 overflow-hidden md:h-[calc(100svh-5rem)]' />
    )
  }

  return <PublicChatReady state={publicChatState} />
}
