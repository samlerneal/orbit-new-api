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
  messages: Message[]
}

const MAX_SESSION_TITLE_LENGTH = 28

export function createPublicChatSession(ordinal: number): PublicChatSession {
  return {
    id: `chat-${ordinal}`,
    title: null,
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
      title: session.title ?? createPublicChatSessionTitle(firstPrompt),
      messages,
    }
  })
}
