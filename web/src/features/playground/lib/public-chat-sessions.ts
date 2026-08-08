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
import type { Message } from '../types'

export type PublicChatSession = {
  id: string
  title: string | null
  created_at: number
  updated_at: number
  messages: Message[]
}

const MAX_SESSION_TITLE_LENGTH = 28

export function createPublicChatSession(): PublicChatSession | null {
  const cryptoApi = globalThis.crypto
  if (!cryptoApi?.getRandomValues) return null
  const random = new Uint32Array(4)
  cryptoApi.getRandomValues(random)
  const now = Date.now()
  return {
    id: Array.from(random, (value) => value.toString(16).padStart(8, '0')).join(
      ''
    ),
    title: null,
    created_at: now,
    updated_at: now,
    messages: [],
  }
}

export function createPublicChatSessionTitle(prompt: string): string {
  const normalizedPrompt = prompt.replaceAll(/\s+/g, ' ').trim()
  if (normalizedPrompt.length <= MAX_SESSION_TITLE_LENGTH) {
    return normalizedPrompt
  }
  return `${normalizedPrompt.slice(0, MAX_SESSION_TITLE_LENGTH)}…`
}

export function updatePublicChatSessionMessages(
  sessions: PublicChatSession[],
  sessionId: string,
  updater: (messages: Message[]) => Message[]
): PublicChatSession[] {
  return sessions.map((session) => {
    if (session.id !== sessionId) return session

    return {
      ...session,
      updated_at: Date.now(),
      messages: updater(session.messages),
    }
  })
}

export function setPublicChatSessionMessages(
  sessions: PublicChatSession[],
  sessionId: string,
  messages: Message[],
  firstPrompt: string
): PublicChatSession[] {
  return sessions.map((session) => {
    if (session.id !== sessionId) return session

    return {
      ...session,
      updated_at: Date.now(),
      title: session.title ?? createPublicChatSessionTitle(firstPrompt),
      messages,
    }
  })
}
