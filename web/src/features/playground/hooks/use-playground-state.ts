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
import { useCallback, useEffect, useRef, useState } from 'react'

import { DEFAULT_CONFIG, DEFAULT_PARAMETER_ENABLED } from '../constants'
import {
  saveConfig,
  saveParameterEnabled,
  createConversationSession,
  getConversationTitle,
  saveConversationState,
  applyMessageStateUpdate,
  deleteConversationSession,
  getConversationStorageUsage,
  getInitialParameterEnabled,
  getInitialPlaygroundConfig,
  loadConversationState,
  type MessageStateUpdater,
} from '../lib'
import type {
  Message,
  PlaygroundConfig,
  ParameterEnabled,
  ModelOption,
  GroupOption,
  PlaygroundConversationSession,
} from '../types'

const MESSAGE_SAVE_DEBOUNCE_MS = 500

/**
 * Main state management hook for playground
 */
export function usePlaygroundState() {
  // Load initial state from localStorage
  const [config, setConfig] = useState<PlaygroundConfig>(
    getInitialPlaygroundConfig
  )

  const [parameterEnabled, setParameterEnabled] = useState<ParameterEnabled>(
    getInitialParameterEnabled
  )

  const [messages, setMessages] = useState<Message[]>([])
  const [sessions, setSessions] = useState<PlaygroundConversationSession[]>([])
  const [storageUsage, setStorageUsage] = useState(() =>
    getConversationStorageUsage([])
  )
  const [activeSessionId, setActiveSessionId] = useState('')
  const [isLoadingMessages, setIsLoadingMessages] = useState(true)
  const messagesSaveTimerRef = useRef<number | null>(null)
  const latestMessagesRef = useRef<Message[]>(messages)
  const latestSessionsRef = useRef<PlaygroundConversationSession[]>(sessions)
  const activeSessionIdRef = useRef(activeSessionId)
  const hasLoadedMessagesRef = useRef(false)

  const [models, setModels] = useState<ModelOption[]>([])
  const [groups, setGroups] = useState<GroupOption[]>([])

  const persistSessions = useCallback(
    (sessionsToSave: PlaygroundConversationSession[], activeId: string) => {
      if (!hasLoadedMessagesRef.current) {
        return
      }

      if (messagesSaveTimerRef.current !== null) {
        window.clearTimeout(messagesSaveTimerRef.current)
      }

      messagesSaveTimerRef.current = window.setTimeout(() => {
        messagesSaveTimerRef.current = null
        const savedSessions = saveConversationState(sessionsToSave, activeId)
        latestSessionsRef.current = savedSessions
        setStorageUsage(getConversationStorageUsage(savedSessions))
        if (savedSessions.length !== sessionsToSave.length) {
          setSessions(savedSessions)
        }
      }, MESSAGE_SAVE_DEBOUNCE_MS)
    },
    []
  )

  const setActiveMessages = useCallback(
    (messagesToSave: Message[]) => {
      latestMessagesRef.current = messagesToSave
      setMessages(messagesToSave)

      const activeId = activeSessionIdRef.current
      if (!activeId) {
        return
      }

      setSessions((previousSessions) => {
        const now = Date.now()
        const nextSessions = previousSessions.map((session) =>
          session.id === activeId
            ? {
                ...session,
                messages: messagesToSave,
                title: getConversationTitle(messagesToSave),
                updatedAt: now,
              }
            : session
        )

        latestSessionsRef.current = nextSessions
        persistSessions(nextSessions, activeId)
        return nextSessions
      })
    },
    [persistSessions]
  )

  useEffect(() => {
    let cancelled = false

    window.setTimeout(() => {
      const loadedState = loadConversationState()
      const activeSession =
        loadedState.sessions.find(
          (session) => session.id === loadedState.activeSessionId
        ) ?? loadedState.sessions[0]
      const loadedMessages = activeSession?.messages ?? []
      if (cancelled) {
        return
      }

      latestMessagesRef.current = loadedMessages
      latestSessionsRef.current = loadedState.sessions
      activeSessionIdRef.current = loadedState.activeSessionId
      hasLoadedMessagesRef.current = true
      setSessions(loadedState.sessions)
      setStorageUsage(getConversationStorageUsage(loadedState.sessions))
      setActiveSessionId(loadedState.activeSessionId)
      setMessages(loadedMessages)
      setIsLoadingMessages(false)
    }, 0)

    return () => {
      cancelled = true
    }
  }, [])

  useEffect(
    () => () => {
      if (messagesSaveTimerRef.current !== null) {
        window.clearTimeout(messagesSaveTimerRef.current)
        saveConversationState(
          latestSessionsRef.current,
          activeSessionIdRef.current
        )
      }
    },
    []
  )

  // Update config with automatic save
  const updateConfig = useCallback(
    <K extends keyof PlaygroundConfig>(key: K, value: PlaygroundConfig[K]) => {
      setConfig((prev) => {
        const updated = { ...prev, [key]: value }
        saveConfig(updated)
        return updated
      })
    },
    []
  )

  // Update parameter enabled with automatic save
  const updateParameterEnabled = useCallback(
    (key: keyof ParameterEnabled, value: boolean) => {
      setParameterEnabled((prev) => {
        const updated = { ...prev, [key]: value }
        saveParameterEnabled(updated)
        return updated
      })
    },
    []
  )

  // Update messages with automatic save
  const updateMessages = useCallback(
    (updater: MessageStateUpdater) => {
      setActiveMessages(
        applyMessageStateUpdate(latestMessagesRef.current, updater)
      )
    },
    [setActiveMessages]
  )

  // Clear all messages
  const clearMessages = useCallback(() => {
    updateMessages([])
  }, [updateMessages])

  const selectConversation = useCallback((sessionId: string) => {
    if (sessionId === activeSessionIdRef.current) {
      return
    }

    const nextSession = latestSessionsRef.current.find(
      (session) => session.id === sessionId
    )
    if (!nextSession) {
      return
    }

    if (messagesSaveTimerRef.current !== null) {
      window.clearTimeout(messagesSaveTimerRef.current)
      messagesSaveTimerRef.current = null
    }

    const previousSessions = latestSessionsRef.current
    const savedSessions = saveConversationState(
      previousSessions,
      nextSession.id
    )
    latestSessionsRef.current = savedSessions
    activeSessionIdRef.current = nextSession.id
    latestMessagesRef.current = nextSession.messages
    if (savedSessions.length !== previousSessions.length) {
      setSessions(savedSessions)
    }
    setStorageUsage(getConversationStorageUsage(savedSessions))
    setActiveSessionId(nextSession.id)
    setMessages(nextSession.messages)
  }, [])

  const createConversation = useCallback(() => {
    const nextSession = createConversationSession()
    const nextSessions = saveConversationState(
      [nextSession, ...latestSessionsRef.current],
      nextSession.id
    )

    latestSessionsRef.current = nextSessions
    activeSessionIdRef.current = nextSession.id
    latestMessagesRef.current = []
    setSessions(nextSessions)
    setStorageUsage(getConversationStorageUsage(nextSessions))
    setActiveSessionId(nextSession.id)
    setMessages([])
  }, [])

  const deleteConversation = useCallback((sessionId: string) => {
    const nextState = deleteConversationSession(
      latestSessionsRef.current,
      activeSessionIdRef.current,
      sessionId
    )
    if (nextState.sessions === latestSessionsRef.current) {
      return
    }

    if (messagesSaveTimerRef.current !== null) {
      window.clearTimeout(messagesSaveTimerRef.current)
      messagesSaveTimerRef.current = null
    }

    const nextSessions = saveConversationState(
      nextState.sessions,
      nextState.activeSessionId
    )
    const nextActiveSession =
      nextSessions.find(
        (session) => session.id === nextState.activeSessionId
      ) ?? nextSessions[0]

    latestSessionsRef.current = nextSessions
    activeSessionIdRef.current = nextActiveSession.id
    latestMessagesRef.current = nextActiveSession.messages
    setSessions(nextSessions)
    setStorageUsage(getConversationStorageUsage(nextSessions))
    setActiveSessionId(nextActiveSession.id)
    setMessages(nextActiveSession.messages)
  }, [])

  // Reset config to defaults
  const resetConfig = useCallback(() => {
    setConfig(DEFAULT_CONFIG)
    setParameterEnabled(DEFAULT_PARAMETER_ENABLED)
    saveConfig(DEFAULT_CONFIG)
    saveParameterEnabled(DEFAULT_PARAMETER_ENABLED)
  }, [])

  return {
    // State
    config,
    parameterEnabled,
    messages,
    sessions,
    storageUsage,
    activeSessionId,
    isLoadingMessages,
    models,
    groups,

    // Setters
    setModels,
    setGroups,

    // Actions
    updateConfig,
    updateParameterEnabled,
    updateMessages,
    clearMessages,
    createConversation,
    deleteConversation,
    selectConversation,
    resetConfig,
  }
}
