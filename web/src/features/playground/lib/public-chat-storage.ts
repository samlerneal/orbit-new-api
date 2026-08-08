/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import type { Message, ParameterEnabled, PlaygroundConfig } from '../types'
import {
  PLAYGROUND_PARAMETER_CONTROLS,
  type PlaygroundParameterKey,
} from './parameters/playground-parameters'
import type { PublicChatSession } from './public-chat-sessions'

export const PUBLIC_CHAT_STORAGE_KEY = 'mydaily_public_chat:v2:single'
export const PUBLIC_CHAT_MAX_SESSIONS = 20
export const PUBLIC_CHAT_MAX_BYTES = 2 * 1024 * 1024
export const PUBLIC_CHAT_MAX_MESSAGES_PER_SESSION = 100
export const PUBLIC_CHAT_MAX_MESSAGES = 400
export const PUBLIC_CHAT_MAX_MESSAGE_CHARS = 40000
export const PUBLIC_CHAT_MAX_CHARS = 240000
export const PUBLIC_CHAT_MAX_VERSIONS_PER_MESSAGE = 10
export const PUBLIC_CHAT_MAX_VERSION_ID_CHARS = 128
export const PUBLIC_CHAT_MAX_SOURCES_PER_MESSAGE = 20
export const PUBLIC_CHAT_MAX_SOURCE_HREF_CHARS = 2048
export const PUBLIC_CHAT_MAX_SOURCE_TITLE_CHARS = 512
export const PUBLIC_CHAT_MAX_ERROR_CODE_CHARS = 256

export type PublicChatStorageEnvelope = {
  schema_version: 2
  owner_user_id: string
  active_session_id: string
  sessions: PublicChatSession[]
  config: PlaygroundConfig
  parameter_enabled: ParameterEnabled
  updated_at: number
}

export type PublicChatStorageResult =
  | { status: 'ok'; envelope: PublicChatStorageEnvelope }
  | { status: 'missing' | 'unavailable' | 'invalid' }

export type PreparedPublicChatStorage = {
  envelope: PublicChatStorageEnvelope
  trimmed: boolean
}

const MESSAGE_KEYS = [
  'key',
  'from',
  'versions',
  'createdAt',
  'startedAt',
  'completedAt',
  'durationMs',
  'sources',
  'reasoning',
  'isReasoningStreaming',
  'isReasoningComplete',
  'isContentComplete',
  'status',
  'errorCode',
]

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value)
}

function isFiniteNumber(value: unknown): value is number {
  return typeof value === 'number' && Number.isFinite(value)
}

function hasOnlyKeys(
  value: Record<string, unknown>,
  allowed: readonly string[]
): boolean {
  return Object.keys(value).every((key) => allowed.includes(key))
}

function getStorage(): Storage | null {
  try {
    return typeof window === 'undefined' ? null : window.localStorage
  } catch {
    return null
  }
}

function isOptionalFiniteNumber(value: unknown): boolean {
  return value === undefined || isFiniteNumber(value)
}

function isOptionalBoolean(value: unknown): boolean {
  return value === undefined || typeof value === 'boolean'
}

function isOptionalStringOrNull(value: unknown): boolean {
  return value === undefined || value === null || typeof value === 'string'
}

function isSafeSourceHref(value: string): boolean {
  if (value.length === 0 || value.length > PUBLIC_CHAT_MAX_SOURCE_HREF_CHARS) {
    return false
  }
  try {
    const parsed = new URL(value)
    return (
      (parsed.protocol === 'https:' || parsed.protocol === 'http:') &&
      parsed.username === '' &&
      parsed.password === ''
    )
  } catch {
    return false
  }
}

function getMessageChars(message: Message): number {
  return (
    message.versions.reduce((sum, version) => sum + version.content.length, 0) +
    (message.reasoning?.content.length ?? 0)
  )
}

function getSessionChars(session: PublicChatSession): number {
  return session.messages.reduce(
    (sum, message) => sum + getMessageChars(message),
    0
  )
}

function isPending(message: Message | undefined): boolean {
  return message?.status === 'loading' || message?.status === 'streaming'
}

function removeOldestCompletePair(
  sessions: PublicChatSession[]
): PublicChatSession[] | null {
  const candidates = sessions
    .flatMap((session) =>
      session.messages.map((_, index) => ({ session, index }))
    )
    .filter(
      ({ session, index }) =>
        session.messages[index]?.from === 'user' &&
        session.messages[index + 1]?.from === 'assistant' &&
        !isPending(session.messages[index]) &&
        !isPending(session.messages[index + 1])
    )
    .sort(
      (left, right) =>
        (left.session.messages[left.index]?.createdAt ??
          left.session.created_at) -
        (right.session.messages[right.index]?.createdAt ??
          right.session.created_at)
    )
  const target = candidates[0]
  if (!target) return null

  return sessions.map((session) => {
    if (session.id !== target.session.id) return session
    return {
      ...session,
      messages: [
        ...session.messages.slice(0, target.index),
        ...session.messages.slice(target.index + 2),
      ],
      updated_at: Date.now(),
    }
  })
}

export function trimPublicChatSessions(
  sessions: PublicChatSession[]
): PublicChatSession[] | null {
  let next = sessions
  while (true) {
    const totalMessages = next.reduce(
      (sum, session) => sum + session.messages.length,
      0
    )
    const totalChars = next.reduce(
      (sum, session) => sum + getSessionChars(session),
      0
    )
    const violatesMessageLimit = next.some((session) =>
      session.messages.some(
        (message) => getMessageChars(message) > PUBLIC_CHAT_MAX_MESSAGE_CHARS
      )
    )
    const violatesSessionLimit = next.some(
      (session) =>
        session.messages.length > PUBLIC_CHAT_MAX_MESSAGES_PER_SESSION
    )
    if (
      !violatesMessageLimit &&
      !violatesSessionLimit &&
      totalMessages <= PUBLIC_CHAT_MAX_MESSAGES &&
      totalChars <= PUBLIC_CHAT_MAX_CHARS
    ) {
      return next
    }

    const trimmed = removeOldestCompletePair(next)
    if (!trimmed) return null
    next = trimmed
  }
}

export function getPublicChatEnvelopeBytes(
  envelope: PublicChatStorageEnvelope
): number {
  return new TextEncoder().encode(JSON.stringify(envelope)).byteLength
}

function isParameterValueInRange(
  key: PlaygroundParameterKey,
  value: unknown
): boolean {
  if (key === 'seed' && value === null) return true
  if (!isFiniteNumber(value)) return false
  const control = PLAYGROUND_PARAMETER_CONTROLS.find(
    (candidate) => candidate.key === key
  )
  return Boolean(
    control &&
    value >= control.min &&
    value <= control.max &&
    (control.step < 1 || Number.isInteger(value))
  )
}

function isConfig(value: unknown): value is PlaygroundConfig {
  if (!isRecord(value)) return false
  return (
    hasOnlyKeys(value, [
      'model',
      'group',
      'stream',
      'temperature',
      'top_p',
      'max_tokens',
      'frequency_penalty',
      'presence_penalty',
      'seed',
    ]) &&
    typeof value.model === 'string' &&
    value.model.length > 0 &&
    value.model.length <= 256 &&
    typeof value.group === 'string' &&
    value.group.length > 0 &&
    value.group.length <= 256 &&
    typeof value.stream === 'boolean' &&
    isParameterValueInRange('temperature', value.temperature) &&
    isParameterValueInRange('top_p', value.top_p) &&
    isParameterValueInRange('max_tokens', value.max_tokens) &&
    isParameterValueInRange('frequency_penalty', value.frequency_penalty) &&
    isParameterValueInRange('presence_penalty', value.presence_penalty) &&
    isParameterValueInRange('seed', value.seed)
  )
}

function isParameterEnabled(value: unknown): value is ParameterEnabled {
  if (!isRecord(value)) return false
  const keys = [
    'temperature',
    'top_p',
    'max_tokens',
    'frequency_penalty',
    'presence_penalty',
    'seed',
  ] as const
  return (
    hasOnlyKeys(value, keys) &&
    keys.every((key) => typeof value[key] === 'boolean')
  )
}

function sanitizeReasoning(value: unknown): Message['reasoning'] | null {
  if (value === undefined) return undefined
  if (
    !isRecord(value) ||
    !hasOnlyKeys(value, [
      'content',
      'duration',
      'startedAt',
      'completedAt',
      'durationMs',
    ]) ||
    typeof value.content !== 'string' ||
    !isFiniteNumber(value.duration) ||
    !isOptionalFiniteNumber(value.startedAt) ||
    !isOptionalFiniteNumber(value.completedAt) ||
    !isOptionalFiniteNumber(value.durationMs)
  ) {
    return null
  }

  return {
    content: value.content,
    duration: value.duration,
    ...(isFiniteNumber(value.startedAt) ? { startedAt: value.startedAt } : {}),
    ...(isFiniteNumber(value.completedAt)
      ? { completedAt: value.completedAt }
      : {}),
    ...(isFiniteNumber(value.durationMs)
      ? { durationMs: value.durationMs }
      : {}),
  }
}

function sanitizeMessage(
  value: unknown,
  restorePending: boolean
): Message | null {
  if (
    !isRecord(value) ||
    !hasOnlyKeys(value, MESSAGE_KEYS) ||
    typeof value.key !== 'string' ||
    value.key.length === 0 ||
    value.key.length > 128 ||
    !['user', 'assistant', 'system'].includes(String(value.from)) ||
    !Array.isArray(value.versions) ||
    value.versions.length === 0 ||
    value.versions.length > PUBLIC_CHAT_MAX_VERSIONS_PER_MESSAGE ||
    !isOptionalFiniteNumber(value.createdAt) ||
    !isOptionalFiniteNumber(value.startedAt) ||
    !isOptionalFiniteNumber(value.completedAt) ||
    !isOptionalFiniteNumber(value.durationMs) ||
    !isOptionalBoolean(value.isReasoningStreaming) ||
    !isOptionalBoolean(value.isReasoningComplete) ||
    !isOptionalBoolean(value.isContentComplete) ||
    !isOptionalStringOrNull(value.errorCode) ||
    (typeof value.errorCode === 'string' &&
      value.errorCode.length > PUBLIC_CHAT_MAX_ERROR_CODE_CHARS)
  ) {
    return null
  }
  if (
    value.status !== undefined &&
    !['loading', 'streaming', 'complete', 'error'].includes(
      String(value.status)
    )
  ) {
    return null
  }
  if (
    value.sources !== undefined &&
    (!Array.isArray(value.sources) ||
      value.sources.length > PUBLIC_CHAT_MAX_SOURCES_PER_MESSAGE ||
      value.sources.some(
        (source) =>
          !isRecord(source) ||
          !hasOnlyKeys(source, ['href', 'title']) ||
          typeof source.href !== 'string' ||
          !isSafeSourceHref(source.href) ||
          typeof source.title !== 'string' ||
          source.title.length > PUBLIC_CHAT_MAX_SOURCE_TITLE_CHARS
      ))
  ) {
    return null
  }

  const versions = value.versions.map((version) =>
    isRecord(version) &&
    hasOnlyKeys(version, ['id', 'content']) &&
    typeof version.id === 'string' &&
    version.id.length > 0 &&
    version.id.length <= PUBLIC_CHAT_MAX_VERSION_ID_CHARS &&
    typeof version.content === 'string'
      ? { id: version.id, content: version.content }
      : null
  )
  if (versions.some((version) => version === null)) return null

  const reasoning = sanitizeReasoning(value.reasoning)
  if (reasoning === null) return null
  const pending =
    restorePending &&
    (value.status === 'loading' || value.status === 'streaming')
  const restoredStatus = pending
    ? 'error'
    : ((value.status ?? 'complete') as Message['status'])

  return {
    key: value.key,
    from: value.from as Message['from'],
    versions: versions as Message['versions'],
    ...(isFiniteNumber(value.createdAt) ? { createdAt: value.createdAt } : {}),
    ...(isFiniteNumber(value.startedAt) ? { startedAt: value.startedAt } : {}),
    ...(isFiniteNumber(value.completedAt)
      ? { completedAt: value.completedAt }
      : {}),
    ...(isFiniteNumber(value.durationMs)
      ? { durationMs: value.durationMs }
      : {}),
    ...(Array.isArray(value.sources)
      ? {
          sources: value.sources.map((source) => ({
            href: source.href as string,
            title: source.title as string,
          })),
        }
      : {}),
    ...(reasoning ? { reasoning } : {}),
    isReasoningStreaming: pending
      ? false
      : (value.isReasoningStreaming as boolean | undefined),
    isReasoningComplete: pending
      ? true
      : (value.isReasoningComplete as boolean | undefined),
    isContentComplete: pending
      ? true
      : (value.isContentComplete as boolean | undefined),
    status: restoredStatus,
    errorCode: pending
      ? 'interrupted'
      : ((value.errorCode as string | null | undefined) ?? null),
  }
}

function sanitizeSessions(
  value: unknown,
  restorePending: boolean
): PublicChatSession[] | null {
  if (
    !Array.isArray(value) ||
    value.length === 0 ||
    value.length > PUBLIC_CHAT_MAX_SESSIONS
  ) {
    return null
  }
  const ids = new Set<string>()
  const sessions: PublicChatSession[] = []
  for (const rawSession of value) {
    if (
      !isRecord(rawSession) ||
      !hasOnlyKeys(rawSession, [
        'id',
        'title',
        'created_at',
        'updated_at',
        'messages',
      ]) ||
      typeof rawSession.id !== 'string' ||
      rawSession.id.length === 0 ||
      rawSession.id.length > 128 ||
      ids.has(rawSession.id) ||
      (rawSession.title !== null &&
        (typeof rawSession.title !== 'string' ||
          rawSession.title.length > 256)) ||
      !isFiniteNumber(rawSession.created_at) ||
      !isFiniteNumber(rawSession.updated_at) ||
      !Array.isArray(rawSession.messages)
    ) {
      return null
    }
    const messages = rawSession.messages.map((message) =>
      sanitizeMessage(message, restorePending)
    )
    if (messages.some((message) => message === null)) return null
    ids.add(rawSession.id)
    sessions.push({
      id: rawSession.id,
      title: rawSession.title,
      created_at: rawSession.created_at,
      updated_at: rawSession.updated_at,
      messages: messages as Message[],
    })
  }
  return sessions
}

function validateEnvelope(
  value: unknown,
  ownerUserId: number | string,
  restorePending: boolean
): PublicChatStorageEnvelope | null {
  if (
    !isRecord(value) ||
    !hasOnlyKeys(value, [
      'schema_version',
      'owner_user_id',
      'active_session_id',
      'sessions',
      'config',
      'parameter_enabled',
      'updated_at',
    ]) ||
    value.schema_version !== 2 ||
    value.owner_user_id !== String(ownerUserId) ||
    typeof value.active_session_id !== 'string' ||
    value.active_session_id.length === 0 ||
    value.active_session_id.length > 128 ||
    !isFiniteNumber(value.updated_at) ||
    !isConfig(value.config) ||
    !isParameterEnabled(value.parameter_enabled)
  ) {
    return null
  }
  const sessions = sanitizeSessions(value.sessions, restorePending)
  if (!sessions) return null
  return {
    schema_version: 2,
    owner_user_id: String(ownerUserId),
    active_session_id: value.active_session_id,
    sessions,
    config: value.config,
    parameter_enabled: value.parameter_enabled,
    updated_at: value.updated_at,
  }
}

export function preparePublicChatStorage(
  envelope: PublicChatStorageEnvelope
): PreparedPublicChatStorage | null {
  const validated = validateEnvelope(envelope, envelope.owner_user_id, false)
  if (!validated) return null
  const sessions = trimPublicChatSessions(validated.sessions)
  if (!sessions) return null
  const activeSessionId = sessions.some(
    (session) => session.id === validated.active_session_id
  )
    ? validated.active_session_id
    : sessions[0]?.id
  if (!activeSessionId) return null

  let prepared: PublicChatStorageEnvelope = {
    ...validated,
    active_session_id: activeSessionId,
    sessions,
  }
  while (getPublicChatEnvelopeBytes(prepared) > PUBLIC_CHAT_MAX_BYTES) {
    const nextSessions = removeOldestCompletePair(prepared.sessions)
    if (!nextSessions) return null
    prepared = { ...prepared, sessions: nextSessions }
  }

  const inputMessageCount = envelope.sessions.reduce(
    (sum, session) => sum + session.messages.length,
    0
  )
  const preparedMessageCount = prepared.sessions.reduce(
    (sum, session) => sum + session.messages.length,
    0
  )
  return {
    envelope: prepared,
    trimmed:
      prepared.sessions.length !== envelope.sessions.length ||
      preparedMessageCount !== inputMessageCount ||
      prepared.active_session_id !== envelope.active_session_id,
  }
}

export function isPublicChatSerializedSizeAllowed(serialized: string): boolean {
  return (
    new TextEncoder().encode(serialized).byteLength <= PUBLIC_CHAT_MAX_BYTES
  )
}

export function getPublicChatStorageKey(): string {
  return PUBLIC_CHAT_STORAGE_KEY
}

export function clearPublicChatStorage(): boolean {
  const storage = getStorage()
  if (!storage) return false
  try {
    storage.removeItem(PUBLIC_CHAT_STORAGE_KEY)
    return true
  } catch {
    return false
  }
}

export function readPublicChatStorage(
  ownerUserId: number | string
): PublicChatStorageResult {
  const storage = getStorage()
  if (!storage) return { status: 'unavailable' }
  try {
    const raw = storage.getItem(PUBLIC_CHAT_STORAGE_KEY)
    if (!raw) return { status: 'missing' }
    if (!isPublicChatSerializedSizeAllowed(raw)) {
      throw new Error('Oversized public chat snapshot')
    }
    const parsed: unknown = JSON.parse(raw)
    const envelope = validateEnvelope(parsed, ownerUserId, true)
    if (!envelope) throw new Error('Invalid public chat snapshot')
    return { status: 'ok', envelope }
  } catch {
    try {
      storage.removeItem(PUBLIC_CHAT_STORAGE_KEY)
    } catch {
      // Memory mode remains safe because identity guards still clear state.
    }
    return { status: 'invalid' }
  }
}

export function writePublicChatStorage(
  envelope: PublicChatStorageEnvelope
): boolean {
  const storage = getStorage()
  if (!storage) return false
  try {
    const prepared = preparePublicChatStorage(envelope)
    if (!prepared) return false
    const serialized = JSON.stringify(prepared.envelope)
    if (!isPublicChatSerializedSizeAllowed(serialized)) {
      return false
    }
    storage.setItem(PUBLIC_CHAT_STORAGE_KEY, serialized)
    return true
  } catch {
    return false
  }
}
