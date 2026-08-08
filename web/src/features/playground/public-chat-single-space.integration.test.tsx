/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import assert from 'node:assert/strict'
import { afterEach, before, beforeEach, describe, test } from 'node:test'

import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import type { AxiosAdapter } from 'axios'
import { act, createElement } from 'react'
import { createRoot, type Root } from 'react-dom/client'

import i18n from '@/i18n/config'
import { api } from '@/lib/api'
import { useAuthStore } from '@/stores/auth-store'

import { PUBLIC_CHAT_STORAGE_KEY } from './lib/public-chat-storage'

// Bun isolates this production-chain file in its own process. Only the SSE
// transport and system-config boundary are replaced; the page and all three
// production hooks remain real.
// @ts-expect-error Bun provides mock.module at runtime.
const { mock } = await import('bun:test')

type StreamListener = (
  event: Event & { data?: string; readyState?: number }
) => void

class FakeSSE {
  static instances: FakeSSE[] = []
  readyState = 1
  closed = false
  streamed = false
  listeners = new Map<string, StreamListener[]>()

  constructor() {
    FakeSSE.instances.push(this)
  }

  addEventListener(type: string, listener: StreamListener) {
    this.listeners.set(type, [...(this.listeners.get(type) ?? []), listener])
  }

  close() {
    this.closed = true
  }

  stream() {
    this.streamed = true
  }

  emit(type: string, data = '') {
    for (const listener of this.listeners.get(type) ?? []) {
      listener({ data, readyState: this.readyState } as Event & {
        data: string
        readyState: number
      })
    }
  }
}

mock.module('sse.js', () => ({ SSE: FakeSSE }))

const { PublicChat, submitPublicChatTurn } = await import('./public-chat')
const { usePublicChatState } = await import('./hooks/use-public-chat-state')

type CapturedPublicChatState = ReturnType<typeof usePublicChatState>
let capturedPublicChatState: CapturedPublicChatState | null = null

function PublicChatStateHarness({ userId }: { userId: number }) {
  const state = usePublicChatState(userId, true)
  capturedPublicChatState = state
  return (
    <div data-owner={state.identityReady ? userId : 'pending'}>
      {state.sessions
        .flatMap((session) => session.messages)
        .flatMap((message) => message.versions)
        .map((version) => version.content)
        .join('|')}
    </div>
  )
}

type MountedPage = {
  client: QueryClient
  container: HTMLDivElement
  root: Root
}

const originalAdapter = api.defaults.adapter
const consoleErrors: string[] = []
let originalConsoleError: typeof console.error

function setUser(userId: number | undefined) {
  const auth = useAuthStore.getState().auth
  if (userId === undefined) {
    auth.reset('complete')
    return
  }
  auth.setBundle({
    access_token: `test-token-${userId}`,
    access_expires_at: Math.floor(Date.now() / 1000) + 3600,
    token_type: 'Bearer',
    user: { id: userId, role: 1, username: `user-${userId}` },
    session: {
      sid: `session-${userId}`,
      current: true,
      login_method: 'test',
      ip: '127.0.0.1',
      user_agent: 'happy-dom',
      created_at: 1,
      last_active_at: 1,
      expires_at: Math.floor(Date.now() / 1000) + 3600,
    },
  })
}

function installApiAdapter() {
  const adapter: AxiosAdapter = async (config) => {
    if (config.url === '/api/user/models') {
      return {
        config,
        data: { success: true, data: ['allowed-model'] },
        headers: {},
        request: null,
        status: 200,
        statusText: 'OK',
      }
    }
    if (config.url === '/api/user/self/groups') {
      return {
        config,
        data: {
          success: true,
          data: { default: { desc: 'Default', ratio: 1 } },
        },
        headers: {},
        request: null,
        status: 200,
        statusText: 'OK',
      }
    }
    throw new Error(`Unexpected API request: ${config.url}`)
  }
  api.defaults.adapter = adapter
}

async function flush(delay = 0) {
  await act(async () => {
    await Promise.resolve()
    if (delay) await new Promise((resolve) => window.setTimeout(resolve, delay))
  })
}

async function mount(userId: number): Promise<MountedPage> {
  setUser(userId)
  const container = document.createElement('div')
  document.body.append(container)
  const root = createRoot(container)
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  await act(async () => {
    root.render(
      createElement(QueryClientProvider, { client }, createElement(PublicChat))
    )
    await new Promise((resolve) => window.setTimeout(resolve, 120))
  })
  await flush()
  return { client, container, root }
}

async function unmount(page: MountedPage) {
  await act(async () => page.root.unmount())
  page.client.clear()
  page.container.remove()
}

async function mountStateHarness(userId: number) {
  const container = document.createElement('div')
  document.body.append(container)
  const root = createRoot(container)
  await act(async () => {
    root.render(<PublicChatStateHarness userId={userId} />)
    await new Promise((resolve) => window.setTimeout(resolve, 20))
  })
  await flush()
  return { container, root }
}

async function rerenderStateHarness(root: Root, userId: number) {
  await act(async () => {
    root.render(<PublicChatStateHarness userId={userId} />)
    await new Promise((resolve) => window.setTimeout(resolve, 20))
  })
  await flush()
}

function buttonByText(text: string): HTMLButtonElement {
  const button = [...document.querySelectorAll('button')].find(
    (candidate) => candidate.textContent?.trim() === text
  )
  assert.ok(button instanceof HTMLButtonElement, `missing button ${text}`)
  return button
}

function buttonByLabel(label: string): HTMLButtonElement {
  const button = document.querySelector(`button[aria-label="${label}"]`)
  assert.ok(button instanceof HTMLButtonElement, `missing button ${label}`)
  return button
}

async function click(element: HTMLElement) {
  await act(async () => {
    element.dispatchEvent(
      new MouseEvent('click', { bubbles: true, cancelable: true })
    )
    await new Promise((resolve) => window.setTimeout(resolve, 20))
  })
}

async function submitPrompt(text: string) {
  const textarea = document.querySelector('textarea[aria-label="Message"]')
  assert.ok(textarea instanceof HTMLTextAreaElement)
  await act(async () => {
    const setter = Object.getOwnPropertyDescriptor(
      HTMLTextAreaElement.prototype,
      'value'
    )?.set
    setter?.call(textarea, text)
    textarea.dispatchEvent(new Event('input', { bubbles: true }))
    await new Promise((resolve) => window.setTimeout(resolve, 20))
  })
  await click(buttonByLabel('Send'))
}

async function switchIdentity(userId: number | undefined) {
  await act(async () => {
    setUser(userId)
    await new Promise((resolve) => window.setTimeout(resolve, 30))
  })
}

function latestSource(): FakeSSE {
  const source = FakeSSE.instances.at(-1)
  assert.ok(source, 'missing SSE source')
  return source
}

before(async () => {
  await i18n.changeLanguage('en')
})

beforeEach(() => {
  document.body.replaceChildren()
  window.localStorage.clear()
  FakeSSE.instances = []
  capturedPublicChatState = null
  consoleErrors.length = 0
  // eslint-disable-next-line no-console
  originalConsoleError = console.error
  // eslint-disable-next-line no-console
  console.error = (...args: unknown[]) => {
    consoleErrors.push(args.map(String).join(' '))
    originalConsoleError(...args)
  }
  installApiAdapter()
})

afterEach(async () => {
  api.defaults.adapter = originalAdapter
  await act(async () => useAuthStore.getState().auth.reset('complete'))
  // eslint-disable-next-line no-console
  console.error = originalConsoleError
  assert.deepEqual(
    consoleErrors,
    [],
    `Unexpected console.error output: ${consoleErrors.join('\n')}`
  )
})

describe('real PublicChat single-browser chain', () => {
  test('persists once, streams, and restores the same account after remount', async () => {
    const page = await mount(101)
    assert.ok(document.querySelector('button[role="combobox"]'))
    assert.ok(buttonByLabel('Parameters'))

    await submitPrompt('remember this')
    assert.equal(FakeSSE.instances.length, 1)
    const source = latestSource()
    assert.equal(source.streamed, true)
    await act(async () => {
      source.emit(
        'message',
        JSON.stringify({ choices: [{ delta: { content: 'restored answer' } }] })
      )
      source.emit('message', '[DONE]')
      await new Promise((resolve) => window.setTimeout(resolve, 60))
    })
    assert.match(document.body.textContent ?? '', /restored answer/)
    assert.ok(window.localStorage.getItem(PUBLIC_CHAT_STORAGE_KEY))

    await unmount(page)
    const remounted = await mount(101)
    assert.match(document.body.textContent ?? '', /remember this/)
    assert.match(document.body.textContent ?? '', /restored answer/)
    assert.equal(FakeSSE.instances.length, 1)
    await unmount(remounted)
  })

  test('real state hook restores config and parameter-enabled values after remount', async () => {
    const harness = await mountStateHarness(131)
    assert.ok(capturedPublicChatState?.identityReady)
    const firstState = capturedPublicChatState
    await act(async () => {
      firstState.updateConfig('seed', 123)
      firstState.updateParameterEnabled('seed', true)
      await new Promise((resolve) => window.setTimeout(resolve, 20))
    })
    await act(async () => harness.root.unmount())
    harness.container.remove()

    const remounted = await mountStateHarness(131)
    assert.ok(capturedPublicChatState?.identityReady)
    assert.equal(capturedPublicChatState.config.seed, 123)
    assert.equal(capturedPublicChatState.parameterEnabled.seed, true)
    const stored = JSON.parse(
      window.localStorage.getItem(PUBLIC_CHAT_STORAGE_KEY) ?? '{}'
    )
    assert.equal(stored.config.seed, 123)
    assert.equal(stored.parameter_enabled.seed, true)
    await act(async () => remounted.root.unmount())
    remounted.container.remove()
  })

  test('mounted state hook makes old submit and updater inert after identity change', async () => {
    const harness = await mountStateHarness(151)
    assert.ok(capturedPublicChatState?.identityReady)
    const oldState = capturedPublicChatState
    assert.ok(oldState?.activeSession)
    const oldSubmit = oldState.submitMessages
    const oldUpdate = oldState.updateMessages

    await rerenderStateHarness(harness.root, 152)
    assert.ok(capturedPublicChatState?.identityReady)
    const before = window.localStorage.getItem(PUBLIC_CHAT_STORAGE_KEY)
    assert.ok(before)
    assert.equal(JSON.parse(before).owner_user_id, '152')
    let requestCount = 0
    let updaterCount = 0

    await act(async () => {
      assert.equal(
        submitPublicChatTurn({
          activeSession: oldState.activeSession,
          sendChat: () => {
            requestCount += 1
          },
          submitMessages: oldSubmit,
          text: 'stale submit',
        }),
        false
      )
      oldUpdate((messages) => {
        updaterCount += 1
        return messages
      })
    })
    await flush()

    assert.equal(requestCount, 0)
    assert.equal(updaterCount, 0)
    assert.equal(window.localStorage.getItem(PUBLIC_CHAT_STORAGE_KEY), before)
    assert.doesNotMatch(harness.container.textContent ?? '', /stale submit/)
    await act(async () => harness.root.unmount())
    harness.container.remove()
  })

  test('logout clears and closes; old update, complete, and error callbacks are inert', async () => {
    const page = await mount(201)
    await submitPrompt('discard me')
    const source = latestSource()
    const streamCount = FakeSSE.instances.length
    await switchIdentity(undefined)
    assert.equal(source.closed, true)
    assert.equal(window.localStorage.getItem(PUBLIC_CHAT_STORAGE_KEY), null)

    await act(async () => {
      source.emit(
        'message',
        JSON.stringify({ choices: [{ delta: { content: 'late update' } }] })
      )
      source.emit('message', '[DONE]')
      source.emit('error', JSON.stringify({ error: { message: 'late error' } }))
      await new Promise((resolve) => window.setTimeout(resolve, 60))
    })
    assert.doesNotMatch(document.body.textContent ?? '', /late update/)
    assert.equal(window.localStorage.getItem(PUBLIC_CHAT_STORAGE_KEY), null)
    assert.equal(FakeSSE.instances.length, streamCount)
    await unmount(page)
  })

  test('account change clears the single space and stale source cannot affect B', async () => {
    const page = await mount(301)
    await submitPrompt('account A')
    const source = latestSource()
    await switchIdentity(302)
    assert.equal(source.closed, true)
    assert.doesNotMatch(document.body.textContent ?? '', /account A/)
    const stored = window.localStorage.getItem(PUBLIC_CHAT_STORAGE_KEY)
    assert.ok(stored)
    assert.equal(JSON.parse(stored).owner_user_id, '302')

    await act(async () => {
      source.emit(
        'message',
        JSON.stringify({ choices: [{ delta: { content: 'stale A' } }] })
      )
      source.emit('message', '[DONE]')
      source.emit(
        'error',
        JSON.stringify({ error: { message: 'stale A error' } })
      )
      await new Promise((resolve) => window.setTimeout(resolve, 60))
    })
    assert.doesNotMatch(document.body.textContent ?? '', /stale A/)
    assert.equal(
      JSON.parse(window.localStorage.getItem(PUBLIC_CHAT_STORAGE_KEY) ?? '{}')
        .owner_user_id,
      '302'
    )
    assert.equal(FakeSSE.instances.length, 1)
    await unmount(page)
  })

  test('restored pending response becomes interrupted and never resends', async () => {
    const page = await mount(401)
    await submitPrompt('pending request')
    assert.equal(FakeSSE.instances.length, 1)
    await unmount(page)

    const remounted = await mount(401)
    assert.equal(FakeSSE.instances.length, 1)
    const stored = JSON.parse(
      window.localStorage.getItem(PUBLIC_CHAT_STORAGE_KEY) ?? '{}'
    )
    assert.equal(stored.sessions[0].messages[1].status, 'error')
    assert.equal(stored.sessions[0].messages[1].errorCode, 'interrupted')
    await unmount(remounted)
  })

  test('real confirmation actions delete and clear, while generation locks controls', async () => {
    const page = await mount(501)
    await click(buttonByText('New conversation'))
    assert.equal(
      document.querySelectorAll('button[aria-label="Delete conversation"]')
        .length,
      2
    )

    await click(buttonByLabel('Delete conversation'))
    await click(buttonByText('Delete'))
    assert.equal(
      document.querySelectorAll('button[aria-label="Delete conversation"]')
        .length,
      1
    )

    await click(buttonByText('Clear conversations'))
    await click(buttonByText('Delete'))
    assert.equal(
      document.querySelectorAll('button[aria-label="Delete conversation"]')
        .length,
      1
    )

    await submitPrompt('lock controls')
    assert.equal(buttonByText('New conversation').disabled, true)
    assert.equal(buttonByLabel('Parameters').disabled, true)
    const modelSelectors = [
      ...document.querySelectorAll('button[role="combobox"]'),
    ]
    assert.ok(modelSelectors.length > 0)
    assert.equal(
      modelSelectors.every(
        (selector) => selector instanceof HTMLButtonElement && selector.disabled
      ),
      true
    )
    assert.equal(buttonByLabel('Stop').disabled, false)
    await click(buttonByLabel('Stop'))
    await unmount(page)
  })

  test('stale delete and clear confirmations cannot mutate after lock or identity change', async () => {
    const page = await mount(551)
    await click(buttonByText('New conversation'))
    assert.equal(
      document.querySelectorAll('button[aria-label="Delete conversation"]')
        .length,
      2
    )

    await click(buttonByLabel('Delete conversation'))
    const staleDeleteAction = buttonByText('Delete')
    await submitPrompt('start lock')
    await act(async () => {
      staleDeleteAction.dispatchEvent(
        new MouseEvent('click', { bubbles: true, cancelable: true })
      )
    })
    await flush()
    assert.equal(
      document.querySelectorAll('button[aria-label="Delete conversation"]')
        .length,
      2
    )
    await click(buttonByLabel('Stop'))

    await click(buttonByText('Clear conversations'))
    const staleClearAction = buttonByText('Delete')
    await switchIdentity(552)
    const before = window.localStorage.getItem(PUBLIC_CHAT_STORAGE_KEY)
    assert.ok(before)
    await act(async () => {
      staleClearAction.dispatchEvent(
        new MouseEvent('click', { bubbles: true, cancelable: true })
      )
    })
    await flush()
    assert.equal(window.localStorage.getItem(PUBLIC_CHAT_STORAGE_KEY), before)
    assert.equal(JSON.parse(before).owner_user_id, '552')
    assert.equal(
      document.querySelectorAll('button[aria-label="Delete conversation"]')
        .length,
      1
    )
    await unmount(page)
  })
})
