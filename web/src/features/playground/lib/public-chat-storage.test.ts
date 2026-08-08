/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import assert from 'node:assert/strict'
import { describe, test } from 'node:test'

import { DEFAULT_CONFIG, DEFAULT_PARAMETER_ENABLED } from '../constants'
import type { Message } from '../types'
import {
  clearPublicChatStorage,
  getPublicChatStorageKey,
  isPublicChatSerializedSizeAllowed,
  preparePublicChatStorage,
  PUBLIC_CHAT_MAX_BYTES,
  PUBLIC_CHAT_MAX_ERROR_CODE_CHARS,
  PUBLIC_CHAT_MAX_SOURCES_PER_MESSAGE,
  PUBLIC_CHAT_STORAGE_KEY,
  readPublicChatStorage,
  trimPublicChatSessions,
  writePublicChatStorage,
  type PublicChatStorageEnvelope,
} from './public-chat-storage'

function message(
  key: string,
  from: Message['from'],
  content = 'x',
  status: Message['status'] = 'complete'
): Message {
  return {
    key,
    from,
    versions: [{ id: `${key}-v1`, content }],
    status,
  }
}

function envelope(
  messages: Message[] = [],
  overrides: Partial<PublicChatStorageEnvelope> = {}
): PublicChatStorageEnvelope {
  return {
    schema_version: 2,
    owner_user_id: '123456',
    active_session_id: 'session-a',
    sessions: [
      {
        id: 'session-a',
        title: null,
        created_at: 1,
        updated_at: 1,
        messages,
      },
    ],
    config: { ...DEFAULT_CONFIG, model: 'allowed-model', group: 'default' },
    parameter_enabled: { ...DEFAULT_PARAMETER_ENABLED },
    updated_at: 1,
    ...overrides,
  }
}

function storage(
  initial: Record<string, string> = {},
  overrides: Partial<Storage> = {}
) {
  const values = new Map(Object.entries(initial))
  return {
    values,
    getItem: (key: string) => values.get(key) ?? null,
    setItem: (key: string, value: string) => void values.set(key, value),
    removeItem: (key: string) => void values.delete(key),
    clear: () => values.clear(),
    key: (index: number) => [...values.keys()][index] ?? null,
    get length() {
      return values.size
    },
    ...overrides,
  } as Storage & { values: Map<string, string> }
}

function withStorage(target: Storage, run: () => void) {
  withWindow({ localStorage: target }, run)
}

function withWindow(target: unknown, run: () => void) {
  const previous = globalThis.window
  Object.defineProperty(globalThis, 'window', {
    configurable: true,
    value: target,
  })
  try {
    run()
  } finally {
    Object.defineProperty(globalThis, 'window', {
      configurable: true,
      value: previous,
    })
  }
}

describe('public chat single-space storage', () => {
  test('uses one stable key and never derives a user key', () => {
    assert.equal(getPublicChatStorageKey(), 'mydaily_public_chat:v2:single')
    assert.equal(getPublicChatStorageKey(), PUBLIC_CHAT_STORAGE_KEY)
    assert.equal(getPublicChatStorageKey().includes('123456'), false)
  })

  test('accepts exactly 2 MiB and rejects one additional UTF-8 byte', () => {
    assert.equal(
      isPublicChatSerializedSizeAllowed('a'.repeat(PUBLIC_CHAT_MAX_BYTES)),
      true
    )
    assert.equal(
      isPublicChatSerializedSizeAllowed(
        `${'a'.repeat(PUBLIC_CHAT_MAX_BYTES)}b`
      ),
      false
    )
  })

  test('trims complete pairs without splitting a pending pair', () => {
    const complete = Array.from({ length: 102 }, (_, index) =>
      message(String(index), index % 2 === 0 ? 'user' : 'assistant', 'x')
    )
    const trimmed = trimPublicChatSessions(envelope(complete).sessions)
    assert.ok(trimmed)
    assert.equal(trimmed[0]?.messages.length, 100)
    assert.equal(trimmed[0]?.messages[0]?.key, '2')

    const pending = [
      message('user', 'user', 'x'.repeat(40001)),
      message('assistant', 'assistant', '', 'streaming'),
    ]
    assert.equal(trimPublicChatSessions(envelope(pending).sessions), null)
  })

  test('enforces 20/21 sessions, 100/400 messages and 40k/240k chars', () => {
    const baseSession = envelope().sessions[0]
    assert.ok(baseSession)
    const twenty = Array.from({ length: 20 }, (_, index) => ({
      ...baseSession,
      id: `s-${index}`,
    }))
    assert.ok(preparePublicChatStorage(envelope([], { sessions: twenty })))
    assert.equal(
      preparePublicChatStorage(
        envelope([], {
          sessions: [...twenty, { ...baseSession, id: 's-20' }],
        })
      ),
      null
    )

    const oneHundred = Array.from({ length: 100 }, (_, index) =>
      message(`m-${index}`, index % 2 === 0 ? 'user' : 'assistant', 'x')
    )
    assert.ok(preparePublicChatStorage(envelope(oneHundred)))
    const oneHundredTwo = [
      ...oneHundred,
      message('m-100', 'user'),
      message('m-101', 'assistant'),
    ]
    const trimmed = preparePublicChatStorage(envelope(oneHundredTwo))
    assert.ok(trimmed)
    assert.equal(trimmed.envelope.sessions[0]?.messages.length, 100)

    const fourSessions = Array.from({ length: 4 }, (_, sessionIndex) => ({
      ...baseSession,
      id: `four-${sessionIndex}`,
      messages: oneHundred.map((item) => ({
        ...item,
        key: `${sessionIndex}-${item.key}`,
      })),
    }))
    assert.ok(
      preparePublicChatStorage(envelope([], { sessions: fourSessions }))
    )
    const fourHundredOne = preparePublicChatStorage(
      envelope([], {
        sessions: [
          ...fourSessions,
          {
            ...baseSession,
            id: 'message-401',
            messages: [message('message-401', 'user')],
          },
        ],
      })
    )
    assert.ok(fourHundredOne)
    assert.equal(
      fourHundredOne.envelope.sessions.reduce(
        (sum, session) => sum + session.messages.length,
        0
      ),
      399
    )

    assert.ok(
      preparePublicChatStorage(
        envelope([message('40k', 'user', 'x'.repeat(40000))])
      )
    )
    assert.equal(
      preparePublicChatStorage(
        envelope([message('40k+1', 'user', 'x'.repeat(40001))])
      ),
      null
    )
    const charSessions = Array.from({ length: 6 }, (_, sessionIndex) => ({
      ...baseSession,
      id: `chars-${sessionIndex}`,
      messages: [message(`chars-${sessionIndex}`, 'user', 'x'.repeat(40000))],
    }))
    const maxCharEnvelope = envelope([], {
      active_session_id: 'chars-0',
      sessions: charSessions,
    })
    assert.ok(preparePublicChatStorage(maxCharEnvelope))
    const target = storage()
    withStorage(target, () => {
      assert.equal(writePublicChatStorage(maxCharEnvelope), true)
      const restored = readPublicChatStorage(123456)
      assert.equal(restored.status, 'ok')
      if (restored.status !== 'ok') return
      assert.equal(
        restored.envelope.sessions.reduce(
          (total, session) =>
            total +
            session.messages.reduce(
              (sessionTotal, item) =>
                sessionTotal +
                item.versions.reduce(
                  (versionTotal, version) =>
                    versionTotal + version.content.length,
                  0
                ),
              0
            ),
          0
        ),
        240000
      )
    })
    assert.equal(
      preparePublicChatStorage(
        envelope([], {
          sessions: [
            ...charSessions,
            {
              ...baseSession,
              id: 'chars-over-limit',
              messages: [message('chars-over-limit', 'user', 'x')],
            },
          ],
        })
      ),
      null
    )
  })

  test('bounds nested metadata and preserves safe sources', () => {
    const withSource = message('source', 'assistant')
    withSource.sources = [
      { href: 'https://example.com/reference', title: 'Reference' },
    ]
    const prepared = preparePublicChatStorage(envelope([withSource]))
    assert.deepEqual(prepared?.envelope.sessions[0]?.messages[0]?.sources, [
      { href: 'https://example.com/reference', title: 'Reference' },
    ])

    const tooManyVersions = message('versions', 'assistant')
    tooManyVersions.versions = Array.from({ length: 11 }, (_, index) => ({
      id: `v-${index}`,
      content: '',
    }))
    assert.equal(preparePublicChatStorage(envelope([tooManyVersions])), null)

    const oversizedVersionId = message('version-id', 'assistant')
    oversizedVersionId.versions[0] = {
      id: 'v'.repeat(129),
      content: '',
    }
    assert.equal(preparePublicChatStorage(envelope([oversizedVersionId])), null)

    const tooManySources = message('sources', 'assistant')
    tooManySources.sources = Array.from(
      { length: PUBLIC_CHAT_MAX_SOURCES_PER_MESSAGE + 1 },
      (_, index) => ({
        href: `https://example.com/${index}`,
        title: `Reference ${index}`,
      })
    )
    assert.equal(preparePublicChatStorage(envelope([tooManySources])), null)

    const unsafeSource = message('unsafe-source', 'assistant')
    unsafeSource.sources = [{ href: 'javascript:alert(1)', title: 'Unsafe' }]
    assert.equal(preparePublicChatStorage(envelope([unsafeSource])), null)

    const credentialsInSource = message('credential-source', 'assistant')
    credentialsInSource.sources = [
      { href: 'https://user:secret@example.com/', title: 'Unsafe' },
    ]
    assert.equal(
      preparePublicChatStorage(envelope([credentialsInSource])),
      null
    )

    const oversizedError = message('error', 'assistant')
    oversizedError.errorCode = 'e'.repeat(PUBLIC_CHAT_MAX_ERROR_CODE_CHARS + 1)
    assert.equal(preparePublicChatStorage(envelope([oversizedError])), null)

    assert.equal(
      preparePublicChatStorage(
        envelope([], { active_session_id: 's'.repeat(129) })
      ),
      null
    )
  })

  test('accepts only the existing parameter control ranges and integer fields', () => {
    const boundaryConfigs = [
      {
        ...DEFAULT_CONFIG,
        temperature: 0.1,
        top_p: 0.1,
        frequency_penalty: -2,
        presence_penalty: -2,
        max_tokens: 0,
        seed: 0,
      },
      {
        ...DEFAULT_CONFIG,
        temperature: 1,
        top_p: 1,
        frequency_penalty: 2,
        presence_penalty: 2,
        max_tokens: 200000,
        seed: 2147483647,
      },
      { ...DEFAULT_CONFIG, seed: null },
    ]
    for (const config of boundaryConfigs) {
      assert.ok(preparePublicChatStorage(envelope([], { config })))
    }

    const invalidConfigs = [
      { ...DEFAULT_CONFIG, temperature: 0.09 },
      { ...DEFAULT_CONFIG, temperature: 1.01 },
      { ...DEFAULT_CONFIG, top_p: 0 },
      { ...DEFAULT_CONFIG, top_p: 1.01 },
      { ...DEFAULT_CONFIG, frequency_penalty: -2.01 },
      { ...DEFAULT_CONFIG, presence_penalty: 2.01 },
      { ...DEFAULT_CONFIG, max_tokens: -1 },
      { ...DEFAULT_CONFIG, max_tokens: 1.5 },
      { ...DEFAULT_CONFIG, max_tokens: 200001 },
      { ...DEFAULT_CONFIG, seed: -1 },
      { ...DEFAULT_CONFIG, seed: 1.5 },
      { ...DEFAULT_CONFIG, seed: 2147483648 },
      { ...DEFAULT_CONFIG, temperature: Number.NaN },
      { ...DEFAULT_CONFIG, top_p: Number.POSITIVE_INFINITY },
    ]
    for (const config of invalidConfigs) {
      assert.equal(preparePublicChatStorage(envelope([], { config })), null)
    }
  })

  test('restores pending assistants as interrupted without changing live writes', () => {
    const pendingEnvelope = envelope([
      message('u', 'user', 'hello'),
      message('a', 'assistant', '', 'streaming'),
    ])
    const prepared = preparePublicChatStorage(pendingEnvelope)
    assert.ok(prepared)
    assert.equal(prepared.trimmed, false)
    assert.equal(
      prepared.envelope.sessions[0]?.messages[1]?.status,
      'streaming'
    )

    const target = storage()
    withStorage(target, () => {
      assert.equal(writePublicChatStorage(pendingEnvelope), true)
      const restored = readPublicChatStorage(123456)
      assert.equal(restored.status, 'ok')
      if (restored.status !== 'ok') return
      const assistant = restored.envelope.sessions[0]?.messages[1]
      assert.equal(assistant?.status, 'error')
      assert.equal(assistant?.errorCode, 'interrupted')
      assert.equal(assistant?.isContentComplete, true)
    })
  })

  test('deletes corrupt, wrong-owner, unknown and oversized snapshots', () => {
    const invalidValues = [
      '{',
      JSON.stringify(envelope([], { owner_user_id: '654321' })),
      JSON.stringify({ ...envelope(), unexpected: true }),
      JSON.stringify(
        envelope([], {
          config: { ...DEFAULT_CONFIG, max_tokens: 1.5 },
        })
      ),
      'x'.repeat(PUBLIC_CHAT_MAX_BYTES + 1),
    ]
    for (const raw of invalidValues) {
      const target = storage({ [PUBLIC_CHAT_STORAGE_KEY]: raw })
      withStorage(target, () => {
        assert.deepEqual(readPublicChatStorage(123456), { status: 'invalid' })
      })
      assert.equal(target.getItem(PUBLIC_CHAT_STORAGE_KEY), null)
    }
  })

  test('fails safely across the localStorage exception matrix', () => {
    for (const exceptionName of ['QuotaExceededError', 'SecurityError']) {
      const getTarget = storage(
        {},
        {
          getItem: () => {
            throw new DOMException('get', exceptionName)
          },
        }
      )
      withStorage(getTarget, () => {
        assert.deepEqual(readPublicChatStorage(123456), { status: 'invalid' })
      })

      const setTarget = storage(
        {},
        {
          setItem: () => {
            throw new DOMException('set', exceptionName)
          },
        }
      )
      withStorage(setTarget, () => {
        assert.equal(writePublicChatStorage(envelope()), false)
      })

      const removeTarget = storage(
        {},
        {
          removeItem: () => {
            throw new DOMException('remove', exceptionName)
          },
        }
      )
      withStorage(removeTarget, () => {
        assert.equal(clearPublicChatStorage(), false)
        removeTarget.setItem(PUBLIC_CHAT_STORAGE_KEY, '{')
        assert.deepEqual(readPublicChatStorage(123456), { status: 'invalid' })
      })

      const inaccessibleWindow = {}
      Object.defineProperty(inaccessibleWindow, 'localStorage', {
        configurable: true,
        get() {
          throw new DOMException('getter', exceptionName)
        },
      })
      withWindow(inaccessibleWindow, () => {
        assert.deepEqual(readPublicChatStorage(123456), {
          status: 'unavailable',
        })
        assert.equal(writePublicChatStorage(envelope()), false)
        assert.equal(clearPublicChatStorage(), false)
      })
    }

    withWindow(undefined, () => {
      assert.deepEqual(readPublicChatStorage(123456), {
        status: 'unavailable',
      })
      assert.equal(writePublicChatStorage(envelope()), false)
      assert.equal(clearPublicChatStorage(), false)
    })
  })
})
