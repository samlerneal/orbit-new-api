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

import {
  createPublicChatSession,
  createPublicChatSessionTitle,
  setPublicChatSessionMessages,
  updatePublicChatSessionMessages,
} from './public-chat-sessions'

describe('public chat in-memory sessions', () => {
  test('creates sessions with cryptographically random identifiers', () => {
    const session = createPublicChatSession()
    assert.ok(session)
    assert.match(session.id, /^[a-f0-9]{32}$/)
    assert.equal(session.title, null)
  })

  test('does not fall back to weak identifiers when crypto is unavailable', () => {
    const descriptor = Object.getOwnPropertyDescriptor(globalThis, 'crypto')
    Object.defineProperty(globalThis, 'crypto', {
      configurable: true,
      value: undefined,
    })
    try {
      assert.equal(createPublicChatSession(), null)
    } finally {
      if (descriptor) {
        Object.defineProperty(globalThis, 'crypto', descriptor)
      } else {
        delete (globalThis as { crypto?: Crypto }).crypto
      }
    }
  })

  test('normalizes and limits the first prompt used as a session title', () => {
    assert.equal(
      createPublicChatSessionTitle('  hello\n world  '),
      'hello world'
    )
    assert.equal(
      createPublicChatSessionTitle('12345678901234567890123456789'),
      '1234567890123456789012345678…'
    )
  })

  test('updates only the selected session and preserves existing titles', () => {
    const firstSession = createPublicChatSession()
    const secondSession = createPublicChatSession()
    assert.ok(firstSession)
    assert.ok(secondSession)
    const sessions = [firstSession, secondSession]
    const firstMessages = [
      { key: 'user-1', from: 'user' as const, versions: [] },
    ]
    const withFirstPrompt = setPublicChatSessionMessages(
      sessions,
      secondSession.id,
      firstMessages,
      'Analyze this report'
    )
    const withResponse = updatePublicChatSessionMessages(
      withFirstPrompt,
      secondSession.id,
      (messages) => [
        ...messages,
        { key: 'assistant-1', from: 'assistant' as const, versions: [] },
      ]
    )

    assert.equal(withResponse[0], sessions[0])
    assert.equal(withResponse[1].title, 'Analyze this report')
    assert.equal(withResponse[1].messages.length, 2)
  })
})
