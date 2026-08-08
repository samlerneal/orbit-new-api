/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import { useCallback, useEffect, useMemo, useRef, useState } from 'react'

import { DEFAULT_CONFIG, DEFAULT_PARAMETER_ENABLED } from '../constants'
import {
  createPublicChatSession,
  setPublicChatSessionMessages,
  updatePublicChatSessionMessages,
  type PublicChatSession,
} from '../lib/public-chat-sessions'
import {
  clearPublicChatStorage,
  preparePublicChatStorage,
  PUBLIC_CHAT_MAX_SESSIONS,
  readPublicChatStorage,
  writePublicChatStorage,
} from '../lib/public-chat-storage'
import type { Message, ParameterEnabled, PlaygroundConfig } from '../types'

type StorageNotice = 'limit' | 'reset' | 'unavailable' | 'trimmed' | null

type PublicChatSnapshot = {
  ownerUserId: number | undefined
  activeSessionId: string
  config: PlaygroundConfig
  parameterEnabled: ParameterEnabled
  sessions: PublicChatSession[]
}

function createInitialSessions(): PublicChatSession[] {
  const session = createPublicChatSession()
  return session ? [session] : []
}

function createEmptySnapshot(): PublicChatSnapshot {
  return {
    ownerUserId: undefined,
    activeSessionId: '',
    config: { ...DEFAULT_CONFIG },
    parameterEnabled: { ...DEFAULT_PARAMETER_ENABLED },
    sessions: [],
  }
}

// eslint-disable-next-line react/only-export-components
export function shouldReloadPublicChatStorage(
  previousUserId: number | undefined,
  nextUserId: number | undefined
): boolean {
  return previousUserId !== nextUserId
}

// eslint-disable-next-line react/only-export-components
export function getPublicChatPersistDecision(
  saved: boolean,
  trimmed: boolean
): { notice: StorageNotice; replaceState: boolean } {
  if (!saved) return { notice: 'unavailable', replaceState: false }
  if (trimmed) return { notice: 'trimmed', replaceState: true }
  return { notice: null, replaceState: false }
}

// eslint-disable-next-line react/only-export-components
export function canMutatePublicChatSession(isGenerating: boolean): boolean {
  return !isGenerating
}

// eslint-disable-next-line react/only-export-components
export function getPublicChatIdentityState(
  requestedUserId: number | undefined,
  loadedOwnerUserId: number | undefined,
  authReady = true
): { identityReady: boolean } {
  return {
    identityReady:
      authReady &&
      requestedUserId !== undefined &&
      requestedUserId === loadedOwnerUserId,
  }
}

export function usePublicChatState(
  userId: number | undefined,
  authReady = true
) {
  const [sessions, setSessions] = useState<PublicChatSession[]>([])
  const [activeSessionId, setActiveSessionId] = useState('')
  const [config, setConfig] = useState<PlaygroundConfig>({ ...DEFAULT_CONFIG })
  const [parameterEnabled, setParameterEnabled] = useState<ParameterEnabled>({
    ...DEFAULT_PARAMETER_ENABLED,
  })
  const [notice, setNotice] = useState<StorageNotice>(null)
  const [loadedOwnerUserId, setLoadedOwnerUserId] = useState<
    number | undefined
  >(undefined)
  const activeUserRef = useRef<number | undefined>(undefined)
  const requestedUserRef = useRef<number | undefined>(userId)
  const authReadyRef = useRef(authReady)
  const stateRef = useRef<PublicChatSnapshot>(createEmptySnapshot())

  requestedUserRef.current = userId
  authReadyRef.current = authReady
  stateRef.current = {
    ...stateRef.current,
    activeSessionId,
    config,
    parameterEnabled,
    sessions,
  }

  const publishSnapshot = useCallback((snapshot: PublicChatSnapshot) => {
    stateRef.current = snapshot
    setSessions(snapshot.sessions)
    setActiveSessionId(snapshot.activeSessionId)
    setConfig(snapshot.config)
    setParameterEnabled(snapshot.parameterEnabled)
  }, [])

  const publishEmptySnapshot = useCallback(() => {
    const emptySnapshot = createEmptySnapshot()
    publishSnapshot(emptySnapshot)
    setLoadedOwnerUserId(undefined)
    setNotice(null)
  }, [publishSnapshot])

  useEffect(() => {
    if (!authReady) {
      publishEmptySnapshot()
      return
    }

    const previousUserId = activeUserRef.current
    const identityChanged = shouldReloadPublicChatStorage(
      previousUserId,
      userId
    )
    if (!identityChanged && loadedOwnerUserId === userId) return

    activeUserRef.current = userId
    publishEmptySnapshot()

    if (userId === undefined) {
      clearPublicChatStorage()
      return
    }
    if (previousUserId !== undefined && previousUserId !== userId) {
      clearPublicChatStorage()
    }

    const result = readPublicChatStorage(userId)
    if (result.status === 'ok') {
      const restoredSessions = result.envelope.sessions
      const restoredActiveId = restoredSessions.some(
        (session) => session.id === result.envelope.active_session_id
      )
        ? result.envelope.active_session_id
        : (restoredSessions[0]?.id ?? '')
      publishSnapshot({
        ownerUserId: userId,
        activeSessionId: restoredActiveId,
        config: result.envelope.config,
        parameterEnabled: result.envelope.parameter_enabled,
        sessions: restoredSessions,
      })
      setNotice(null)
      setLoadedOwnerUserId(userId)
      return
    }

    const freshSessions = createInitialSessions()
    publishSnapshot({
      ownerUserId: userId,
      activeSessionId: freshSessions[0]?.id ?? '',
      config: { ...DEFAULT_CONFIG },
      parameterEnabled: { ...DEFAULT_PARAMETER_ENABLED },
      sessions: freshSessions,
    })
    let nextNotice: StorageNotice = null
    if (result.status === 'invalid') nextNotice = 'reset'
    if (result.status === 'unavailable') nextNotice = 'unavailable'
    setNotice(nextNotice)
    setLoadedOwnerUserId(userId)
  }, [
    authReady,
    loadedOwnerUserId,
    publishEmptySnapshot,
    publishSnapshot,
    userId,
  ])

  const { identityReady } = getPublicChatIdentityState(
    userId,
    loadedOwnerUserId,
    authReady
  )

  const hasCurrentIdentity = useCallback(
    () =>
      identityReady &&
      authReadyRef.current &&
      requestedUserRef.current === userId &&
      activeUserRef.current === userId &&
      stateRef.current.ownerUserId === userId,
    [identityReady, userId]
  )

  const activeSession = useMemo(
    () =>
      sessions.find((session) => session.id === activeSessionId) ?? sessions[0],
    [activeSessionId, sessions]
  )

  const persistSnapshot = useCallback(
    (snapshot = stateRef.current) => {
      if (!userId || !hasCurrentIdentity()) return false
      const prepared = preparePublicChatStorage({
        schema_version: 2,
        owner_user_id: String(userId),
        active_session_id: snapshot.activeSessionId,
        sessions: snapshot.sessions,
        config: snapshot.config,
        parameter_enabled: snapshot.parameterEnabled,
        updated_at: Date.now(),
      })
      if (!prepared) {
        setNotice('unavailable')
        return false
      }

      const saved = writePublicChatStorage(prepared.envelope)
      const decision = getPublicChatPersistDecision(saved, prepared.trimmed)
      if (decision.replaceState) {
        publishSnapshot({
          ...snapshot,
          activeSessionId: prepared.envelope.active_session_id,
          sessions: prepared.envelope.sessions,
        })
      }
      setNotice(decision.notice)
      return saved
    },
    [hasCurrentIdentity, publishSnapshot, userId]
  )

  useEffect(() => {
    if (identityReady) persistSnapshot()
  }, [
    activeSessionId,
    config,
    identityReady,
    parameterEnabled,
    persistSnapshot,
    sessions,
  ])

  useEffect(() => {
    const flush = () => {
      if (identityReady) persistSnapshot()
    }
    window.addEventListener('pagehide', flush)
    return () => window.removeEventListener('pagehide', flush)
  }, [identityReady, persistSnapshot])

  const updateConfig = useCallback(
    <K extends keyof PlaygroundConfig>(key: K, value: PlaygroundConfig[K]) => {
      if (!hasCurrentIdentity()) return
      setConfig((current) => ({ ...current, [key]: value }))
    },
    [hasCurrentIdentity]
  )

  const updateParameterEnabled = useCallback(
    (key: keyof ParameterEnabled, value: boolean) => {
      if (!hasCurrentIdentity()) return
      setParameterEnabled((current) => ({ ...current, [key]: value }))
    },
    [hasCurrentIdentity]
  )

  const updateMessages = useCallback(
    (updater: (messages: Message[]) => Message[]) => {
      if (!hasCurrentIdentity()) return
      const targetSessionId =
        stateRef.current.activeSessionId || activeSession?.id
      if (!targetSessionId) return

      const nextSessions = updatePublicChatSessionMessages(
        stateRef.current.sessions,
        targetSessionId,
        updater
      )
      stateRef.current = { ...stateRef.current, sessions: nextSessions }
      setSessions(nextSessions)
    },
    [activeSession, hasCurrentIdentity]
  )

  const submitMessages = useCallback(
    (messages: Message[], firstPrompt: string): boolean => {
      if (!hasCurrentIdentity()) return false
      const targetSessionId =
        stateRef.current.activeSessionId || activeSession?.id
      if (!targetSessionId) return false

      const nextSessions = setPublicChatSessionMessages(
        stateRef.current.sessions,
        targetSessionId,
        messages,
        firstPrompt
      )
      const snapshot = {
        ...stateRef.current,
        activeSessionId: targetSessionId,
        sessions: nextSessions,
      }
      stateRef.current = snapshot
      setSessions(nextSessions)
      persistSnapshot(snapshot)
      return true
    },
    [activeSession, hasCurrentIdentity, persistSnapshot]
  )

  const createSession = useCallback((): boolean => {
    if (!hasCurrentIdentity()) return false
    if (stateRef.current.sessions.length >= PUBLIC_CHAT_MAX_SESSIONS) {
      setNotice('limit')
      return false
    }
    const session = createPublicChatSession()
    if (!session) {
      setNotice('unavailable')
      return false
    }

    const snapshot = {
      ...stateRef.current,
      activeSessionId: session.id,
      sessions: [...stateRef.current.sessions, session],
    }
    publishSnapshot(snapshot)
    persistSnapshot(snapshot)
    return true
  }, [hasCurrentIdentity, persistSnapshot, publishSnapshot])

  const deleteSession = useCallback(
    (sessionId: string) => {
      if (!hasCurrentIdentity()) return
      const current = stateRef.current.sessions
      const index = current.findIndex((session) => session.id === sessionId)
      if (index === -1) return
      const remaining = current.filter((session) => session.id !== sessionId)
      const replacement =
        remaining.length === 0 ? createPublicChatSession() : null
      const nextSessions = replacement ? [replacement] : remaining
      const nextActiveId =
        sessionId === stateRef.current.activeSessionId
          ? (nextSessions[Math.min(index, nextSessions.length - 1)]?.id ?? '')
          : stateRef.current.activeSessionId
      const snapshot = {
        ...stateRef.current,
        activeSessionId: nextActiveId,
        sessions: nextSessions,
      }
      publishSnapshot(snapshot)
      persistSnapshot(snapshot)
    },
    [hasCurrentIdentity, persistSnapshot, publishSnapshot]
  )

  const clearSessions = useCallback(() => {
    if (!hasCurrentIdentity()) return
    const session = createPublicChatSession()
    const snapshot = {
      ...stateRef.current,
      activeSessionId: session?.id ?? '',
      sessions: session ? [session] : [],
    }
    publishSnapshot(snapshot)
    persistSnapshot(snapshot)
  }, [hasCurrentIdentity, persistSnapshot, publishSnapshot])

  const selectSession = useCallback(
    (sessionId: string) => {
      if (
        !hasCurrentIdentity() ||
        !stateRef.current.sessions.some((session) => session.id === sessionId)
      ) {
        return
      }
      const snapshot = {
        ...stateRef.current,
        activeSessionId: sessionId,
      }
      stateRef.current = snapshot
      setActiveSessionId(sessionId)
      persistSnapshot(snapshot)
    },
    [hasCurrentIdentity, persistSnapshot]
  )

  if (!identityReady) {
    return {
      activeSession: undefined,
      activeSessionId: '',
      clearSessions,
      config: { ...DEFAULT_CONFIG },
      createSession,
      deleteSession,
      identityReady: false,
      notice: null,
      parameterEnabled: { ...DEFAULT_PARAMETER_ENABLED },
      sessions: [],
      setActiveSessionId: selectSession,
      submitMessages,
      updateConfig,
      updateMessages,
      updateParameterEnabled,
    }
  }

  return {
    activeSession,
    activeSessionId,
    clearSessions,
    config,
    createSession,
    deleteSession,
    identityReady: true,
    notice,
    parameterEnabled,
    sessions,
    setActiveSessionId: selectSession,
    submitMessages,
    updateConfig,
    updateMessages,
    updateParameterEnabled,
  }
}
