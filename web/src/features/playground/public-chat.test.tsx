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
import { createStreamRequestController } from './hooks/use-stream-request'

// @ts-expect-error Bun provides mock.module at runtime.
const { mock } = await import('bun:test')

type ElementNode = {
  props?: Record<string, unknown>
  type?: unknown
}

const state: unknown[] = []
let stateIndex = 0
let chatHandlerOptions: {
  onMessageUpdate: (updater: (messages: unknown[]) => unknown[]) => void
} | null = null
let sentMessages: unknown[][] = []
let stopCalls = 0
let streamController: ReturnType<typeof createStreamRequestController> | null =
  null
let streamSource: FakeStreamSource | null = null
const PlaygroundMessageContent = () => null

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
  usePlaygroundOptions: () => ({ isLoadingModels: false }),
}))
mock.module('./lib', () => ({
  appendUserMessagePair: (messages: unknown[], text: string) => [
    ...messages,
    { key: `user-${messages.length}`, from: 'user', text },
    { key: `assistant-${messages.length}`, from: 'assistant', text: '' },
  ],
  getMessageAlignment: () => 'left',
  getMessageContent: (message: { text: string }) => message.text,
}))

const { PublicChat } = await import('./public-chat')

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
  stateIndex = 0
  return PublicChat()
}

describe('public chat surface contract', () => {
  test('sends exactly one in-memory message pair and delegates stop', () => {
    state.length = 0
    sentMessages = []
    stopCalls = 0
    const page = renderPublicChat()
    const input = findElement(page, PublicChatInput)
    assert.ok(input?.props?.onSubmit)
    assert.ok(input?.props?.onStop)
    const onSubmit = input.props.onSubmit as (text: string) => void
    const onStop = input.props.onStop as () => void

    onSubmit('hello')
    assert.equal(sentMessages.length, 1)
    assert.equal(sentMessages[0].length, 2)

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
    const errorMessage = findElement(page, PlaygroundMessageContent)
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
    const input = findElement(page, PublicChatInput)
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
      page,
      PlaygroundMessageContent,
      isAssistant
    )
    assert.equal(assistantBeforeStop?.props?.versionContent, 'before stop')

    onStop()
    assert.equal(stopCalls, 1)

    const messagesBeforeLateChunk = state[1]
    activeStreamSource.emit(
      'message',
      JSON.stringify({ choices: [{ delta: { content: 'late overwrite' } }] })
    )
    assert.equal(state[1], messagesBeforeLateChunk)

    page = renderPublicChat()
    const assistantAfterLateChunk = findElement(
      page,
      PlaygroundMessageContent,
      isAssistant
    )
    assert.equal(assistantAfterLateChunk?.props?.versionContent, 'before stop')
  })
})
