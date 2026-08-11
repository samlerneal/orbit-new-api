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
import assert from 'node:assert/strict'
import { describe, test } from 'node:test'

import { PublicChatInput } from './components/input/public-chat-input'
import { PublicChatSidebar } from './components/public-chat-sidebar'
import { PublicChatWelcome } from './components/public-chat-welcome'
import { createStreamRequestController } from './hooks/use-stream-request'

// @ts-expect-error Bun provides mock.module at runtime.
const { mock } = await import('bun:test')

type ElementNode = {
  props?: Record<string, unknown>
  type?: unknown
}

const state: unknown[] = []
let stateIndex = 0
const refs: Array<{ current: unknown }> = []
let refIndex = 0
let chatHandlerOptions: {
  onMessageUpdate: (updater: (messages: unknown[]) => unknown[]) => void
} | null = null
let sentMessages: unknown[][] = []
let submissionEvents: string[] = []
let stopCalls = 0
let streamController: ReturnType<typeof createStreamRequestController> | null =
  null
let streamSource: FakeStreamSource | null = null
const PlaygroundMessageContent = () => null
let publicChatSessions: Array<Record<string, unknown>> = []
let publicChatActiveSessionId = ''
let publicChatModelProjection:
  | ((models: Array<{ label: string; value: string }>) => Array<{
      label: string
      value: string
    }>)
  | undefined

function resetPublicChatState() {
  publicChatActiveSessionId = 'chat-1'
  publicChatSessions = [
    {
      id: publicChatActiveSessionId,
      title: null,
      created_at: 1,
      updated_at: 1,
      messages: [],
    },
  ]
}

class FakeStreamSource {
  private listeners = new Map<
    string,
    Array<(event: Event & { data?: string; readyState?: number }) => void>
  >()

  addEventListener(
    type: string,
    listener: (event: Event & { data?: string; readyState?: number }) => void
  ) {
    const listeners = this.listeners.get(type) ?? []
    listeners.push(listener)
    this.listeners.set(type, listeners)
  }

  close() {}

  stream() {}

  emit(type: string, data: string) {
    for (const listener of this.listeners.get(type) ?? []) {
      listener({ data } as Event & { data: string })
    }
  }
}

mock.module('react', () => ({
  useCallback: <T,>(callback: T) => callback,
  useEffect: () => undefined,
  useMemo: <T,>(factory: () => T) => factory(),
  useRef: <T,>(initialValue: T) => {
    const index = refIndex++
    refs[index] ??= { current: initialValue }
    return refs[index] as { current: T }
  },
  useState: <T,>(initialValue: T | (() => T)) => {
    const index = stateIndex++
    if (!(index in state)) {
      state[index] =
        typeof initialValue === 'function'
          ? (initialValue as () => T)()
          : initialValue
    }
    return [
      state[index] as T,
      (nextValue: T | ((currentValue: T) => T)) => {
        state[index] =
          typeof nextValue === 'function'
            ? (nextValue as (currentValue: T) => T)(state[index] as T)
            : nextValue
      },
    ]
  },
}))
mock.module('react/jsx-dev-runtime', () => ({
  Fragment: Symbol.for('react.fragment'),
  jsxDEV: (type: unknown, props: Record<string, unknown>) => ({ type, props }),
}))
mock.module('react/jsx-runtime', () => ({
  Fragment: Symbol.for('react.fragment'),
  jsx: (type: unknown, props: Record<string, unknown>) => ({ type, props }),
  jsxs: (type: unknown, props: Record<string, unknown>) => ({ type, props }),
}))
mock.module('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}))
mock.module('@/hooks/use-system-config', () => ({
  useSystemConfig: () => ({ logo: '/logo.svg', systemName: 'Mydaily API' }),
}))
mock.module('@/stores/auth-store', () => ({
  useAuthStore: () => 123456,
}))
mock.module('@/components/ai-elements/conversation', () => ({
  Conversation: () => null,
  ConversationContent: () => null,
  ConversationScrollButton: () => null,
}))
mock.module('@/components/ai-elements/message', () => ({ Message: () => null }))
mock.module('./components/message/playground-message-content', () => ({
  PlaygroundMessageContent,
}))
mock.module('./constants', () => ({
  DEFAULT_CONFIG: {
    group: 'default',
    model: 'allowed-model',
    stream: true,
  },
  DEFAULT_PARAMETER_ENABLED: {},
}))
mock.module('./hooks', () => ({
  useChatHandler: (options: NonNullable<typeof chatHandlerOptions>) => {
    chatHandlerOptions = options
    streamController ??= createStreamRequestController({
      getHeaders: () => Promise.resolve({}),
      createSource: () => {
        streamSource = new FakeStreamSource()
        return streamSource
      },
      setStreaming: () => undefined,
    })
    return {
      isGenerating: false,
      sendChat: (messages: unknown[]) => {
        submissionEvents.push('sendChat')
        sentMessages.push(messages)
        void streamController?.send({} as never, {
          onUpdate: (_type, chunk) => {
            options.onMessageUpdate((currentMessages) =>
              currentMessages.map((message, index) =>
                index === currentMessages.length - 1
                  ? { ...(message as object), text: chunk }
                  : message
              )
            )
          },
          onComplete: () => undefined,
          onError: () => undefined,
        })
      },
      stopGeneration: () => {
        stopCalls += 1
        streamController?.stop()
      },
    }
  },
  usePlaygroundOptions: (options: {
    projectModels?: typeof publicChatModelProjection
  }) => {
    publicChatModelProjection = options.projectModels
    return { isLoadingModels: false }
  },
}))
function getPublicChatState() {
  return {
    activeSession: publicChatSessions.find(
      (session) => session.id === publicChatActiveSessionId
    ),
    activeSessionId: publicChatActiveSessionId,
    clearSessions: () => resetPublicChatState(),
    config: { group: 'default', model: 'allowed-model', stream: true },
    createSession: () => {
      const session = {
        id: `chat-${publicChatSessions.length + 1}`,
        title: null,
        created_at: 1,
        updated_at: 1,
        messages: [],
      }
      publicChatSessions = [...publicChatSessions, session]
      publicChatActiveSessionId = session.id
      return true
    },
    deleteSession: (sessionId: string) => {
      publicChatSessions = publicChatSessions.filter(
        (session) => session.id !== sessionId
      )
      publicChatActiveSessionId = publicChatSessions[0]?.id as string
    },
    identityReady: true,
    notice: null,
    parameterEnabled: {},
    sessions: publicChatSessions,
    setActiveSessionId: (sessionId: string) => {
      publicChatActiveSessionId = sessionId
    },
    submitMessages: (messages: unknown[]) => {
      submissionEvents.push('persist')
      publicChatSessions = publicChatSessions.map((session) =>
        session.id === publicChatActiveSessionId
          ? { ...session, messages }
          : session
      )
      return true
    },
    updateConfig: () => undefined,
    updateMessages: (updater: (messages: unknown[]) => unknown[]) => {
      publicChatSessions = publicChatSessions.map((session) =>
        session.id === publicChatActiveSessionId
          ? { ...session, messages: updater(session.messages as unknown[]) }
          : session
      )
    },
    updateParameterEnabled: () => undefined,
  }
}
mock.module('./lib', () => ({
  appendUserMessagePair: (messages: unknown[], text: string) => [
    ...messages,
    { key: `user-${messages.length}`, from: 'user', text },
    { key: `assistant-${messages.length}`, from: 'assistant', text: '' },
  ],
  filterPublicChatModels: (models: Array<{ label: string; value: string }>) =>
    models.filter((model) => model.value !== 'gpt-image-2'),
  getMessageAlignment: () => 'left',
  getMessageContent: (message: { text: string }) => message.text,
}))

const { PublicChatReady } = await import('./public-chat')

function findElement(
  node: unknown,
  type: unknown,
  matches: (element: ElementNode) => boolean = () => true
): ElementNode | undefined {
  if (!node || typeof node !== 'object') return undefined
  const element = node as ElementNode
  if (element.type === type && matches(element)) return element
  const children = element.props?.children
  for (const child of Array.isArray(children) ? children : [children]) {
    const found = findElement(child, type, matches)
    if (found) return found
  }
  return undefined
}

function renderPublicChat() {
  if (state.length === 0) resetPublicChatState()
  stateIndex = 0
  refIndex = 0
  return PublicChatReady({
    state: getPublicChatState() as never,
  })
}

function renderNestedComponent(page: unknown, componentName: string): unknown {
  const findComponent = (node: unknown): ElementNode | undefined => {
    if (!node || typeof node !== 'object') return undefined
    const element = node as ElementNode
    if (
      typeof element.type === 'function' &&
      element.type.name === componentName
    ) {
      return element
    }
    const children = element.props?.children
    for (const child of Array.isArray(children) ? children : [children]) {
      const found = findComponent(child)
      if (found) return found
    }
    return undefined
  }
  const component = findComponent(page)
  if (!component || typeof component.type !== 'function') {
    assert.fail(`missing nested component ${componentName}`)
  }
  return component.type(component.props ?? {})
}

function findPublicChatInput(page: unknown) {
  return findElement(
    renderNestedComponent(page, 'PublicChatComposer'),
    PublicChatInput
  )
}

function findRenderedMessage(page: unknown) {
  return findElement(
    renderNestedComponent(page, 'PublicChatMessages'),
    PlaygroundMessageContent
  )
}

describe('public chat surface contract', () => {
  test('wires the exact image-model projection only into public chat', () => {
    state.length = 0
    renderPublicChat()
    assert.ok(publicChatModelProjection)

    const projected = publicChatModelProjection([
      { label: 'image', value: 'gpt-image-2' },
      { label: 'text', value: 'gpt-5.6-sol' },
      { label: 'similar', value: 'gpt-image-2-preview' },
    ])
    assert.deepEqual(
      projected.map((model) => model.value),
      ['gpt-5.6-sol', 'gpt-image-2-preview']
    )
  })

  test('sends exactly one in-memory message pair and delegates stop', () => {
    state.length = 0
    sentMessages = []
    submissionEvents = []
    stopCalls = 0
    const page = renderPublicChat()
    const input = findPublicChatInput(page)
    assert.ok(input?.props?.onSubmit)
    assert.ok(input?.props?.onStop)
    const onSubmit = input.props.onSubmit as (text: string) => void
    const onStop = input.props.onStop as () => void

    onSubmit('hello')
    assert.equal(sentMessages.length, 1)
    assert.equal(sentMessages[0].length, 2)
    assert.deepEqual(submissionEvents, ['persist', 'sendChat'])

    onStop()
    assert.equal(stopCalls, 1)
  })

  test('renders a safe error update', () => {
    state.length = 0
    let page = renderPublicChat()
    assert.ok(chatHandlerOptions)
    chatHandlerOptions.onMessageUpdate(() => [
      { key: 'error', from: 'assistant', text: 'Request error occurred: safe' },
    ])
    page = renderPublicChat()
    const errorMessage = findRenderedMessage(page)
    assert.equal(
      errorMessage?.props?.versionContent,
      'Request error occurred: safe'
    )
  })

  test('ignores a content chunk that arrives after the user stops', async () => {
    state.length = 0
    sentMessages = []
    stopCalls = 0
    streamController = null
    streamSource = null
    let page = renderPublicChat()
    const input = findPublicChatInput(page)
    assert.ok(input?.props?.onSubmit)
    assert.ok(input?.props?.onStop)

    const onSubmit = input.props.onSubmit as (text: string) => void
    const onStop = input.props.onStop as () => void
    onSubmit('hello')
    await Promise.resolve()
    const activeStreamSource = streamSource as FakeStreamSource | null
    assert.ok(activeStreamSource)

    activeStreamSource.emit(
      'message',
      JSON.stringify({ choices: [{ delta: { content: 'before stop' } }] })
    )
    page = renderPublicChat()
    const isAssistant = (element: ElementNode) =>
      (element.props?.message as { from?: string } | undefined)?.from ===
      'assistant'
    const assistantBeforeStop = findElement(
      renderNestedComponent(page, 'PublicChatMessages'),
      PlaygroundMessageContent,
      isAssistant
    )
    assert.equal(assistantBeforeStop?.props?.versionContent, 'before stop')

    onStop()
    assert.equal(stopCalls, 1)

    const messagesBeforeLateChunk = publicChatSessions[0]?.messages
    activeStreamSource.emit(
      'message',
      JSON.stringify({ choices: [{ delta: { content: 'late overwrite' } }] })
    )
    assert.equal(publicChatSessions[0]?.messages, messagesBeforeLateChunk)

    page = renderPublicChat()
    const assistantAfterLateChunk = findElement(
      renderNestedComponent(page, 'PublicChatMessages'),
      PlaygroundMessageContent,
      isAssistant
    )
    assert.equal(assistantAfterLateChunk?.props?.versionContent, 'before stop')
  })

  test('creates unique conversations from one render and activates the latest', () => {
    state.length = 0
    refs.length = 0
    let page = renderPublicChat()
    let sidebar = findElement(page, PublicChatSidebar)
    assert.equal((sidebar?.props?.sessions as unknown[] | undefined)?.length, 1)

    const onCreateSession = sidebar?.props?.onCreateSession as () => void
    onCreateSession()
    onCreateSession()
    page = renderPublicChat()
    sidebar = findElement(page, PublicChatSidebar)
    const sessions = sidebar?.props?.sessions as
      | Array<{ id: string }>
      | undefined
    assert.equal(sessions?.length, 3)
    assert.notEqual(sessions?.[0]?.id, sessions?.[2]?.id)
    assert.equal(sidebar?.props?.activeSessionId, sessions?.[2]?.id)

    const onSelectSession = sidebar?.props?.onSelectSession as (
      sessionId: string
    ) => void
    onSelectSession(sessions?.[0]?.id ?? '')
    page = renderPublicChat()
    sidebar = findElement(page, PublicChatSidebar)
    assert.equal(sidebar?.props?.activeSessionId, sessions?.[0]?.id)
  })

  test('fills a quick prompt without sending until the user submits', () => {
    state.length = 0
    refs.length = 0
    sentMessages = []
    let page = renderPublicChat()
    const renderedWelcome = renderNestedComponent(page, 'PublicChatMessages')
    const welcomeComponent = findElement(renderedWelcome, PublicChatWelcome)
    assert.ok(welcomeComponent?.props?.onPromptSelect)

    const onPromptSelect = welcomeComponent.props.onPromptSelect as (
      prompt: string
    ) => void
    onPromptSelect('Please summarize this text:')
    assert.equal(sentMessages.length, 0)

    page = renderPublicChat()
    const input = findPublicChatInput(page)
    assert.equal(input?.props?.text, 'Please summarize this text:')
    const onSubmit = input?.props?.onSubmit as (text: string) => void
    onSubmit(input?.props?.text as string)
    assert.equal(sentMessages.length, 1)
  })

  test('clears the composer draft when creating or switching conversations', () => {
    state.length = 0
    refs.length = 0
    let page = renderPublicChat()
    let input = findPublicChatInput(page)
    const onTextChange = input?.props?.onTextChange as (text: string) => void
    onTextChange('draft before create')

    page = renderPublicChat()
    input = findPublicChatInput(page)
    assert.equal(input?.props?.text, 'draft before create')
    let sidebar = findElement(page, PublicChatSidebar)
    const onCreateSession = sidebar?.props?.onCreateSession as () => void
    onCreateSession()

    page = renderPublicChat()
    input = findPublicChatInput(page)
    assert.equal(input?.props?.text, '')
    const onTextChangeAfterCreate = input?.props?.onTextChange as (
      text: string
    ) => void
    onTextChangeAfterCreate('draft before switch')

    page = renderPublicChat()
    sidebar = findElement(page, PublicChatSidebar)
    const onSelectSession = sidebar?.props?.onSelectSession as (
      sessionId: string
    ) => void
    onSelectSession('chat-1')

    page = renderPublicChat()
    input = findPublicChatInput(page)
    assert.equal(input?.props?.text, '')
  })
})
