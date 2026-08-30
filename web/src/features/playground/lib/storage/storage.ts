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
import { MESSAGE_STATUS, STORAGE_KEYS } from '../../constants'
import type {
  PlaygroundConfig,
  ParameterEnabled,
  Message,
  PlaygroundConversationSession,
} from '../../types'
import {
  finalizeMessage,
  isAssistantMessagePending,
  sanitizeMessagesOnLoad,
} from '../message/message-streaming-utils'
import { completeAssistantTiming } from '../message/message-timing-utils'
import { getMessageContent, hasMessageContent } from '../message/message-utils'
import {
  MAX_LOADED_MESSAGE_CHARS,
  MAX_LOADED_MESSAGES_CHARS,
  MAX_STORED_CONVERSATIONS,
  MAX_STORED_CONVERSATIONS_BYTES,
  MAX_STORED_MESSAGES,
  MAX_STORED_MESSAGES_BYTES,
  STORAGE_VERSION,
  conversationSessionsSchema,
  messagesSchema,
  parameterEnabledSchema,
  playgroundConfigSchema,
} from './storage-schema'

type StoredEnvelope<T> = {
  version: number
  data: T
}

const TRUNCATED_CONTENT_SUFFIX = '\n\n[...]'
const MIN_PREFIX_COLLAPSE_LENGTH = 2000
const MIN_REPEATED_SECTION_COUNT = 3
const DEFAULT_CONVERSATION_ID = 'default'
const MAX_CONVERSATION_TITLE_LENGTH = 60
const SECTION_HEADING_LINE_PATTERN = /^#{2,6}\s+\d+\.\s+.+$/gm

function readStoredValue(key: string): unknown | null {
  const saved = localStorage.getItem(key)
  if (!saved) return null

  return JSON.parse(saved) as unknown
}

function readStoredMessagesValue(): unknown | null {
  const saved = localStorage.getItem(STORAGE_KEYS.MESSAGES)
  if (!saved) return null

  if (saved.length > MAX_STORED_MESSAGES_BYTES) {
    localStorage.removeItem(STORAGE_KEYS.MESSAGES)
    return null
  }

  return JSON.parse(saved) as unknown
}

function readStoredConversationsValue(): unknown | null {
  const saved = localStorage.getItem(STORAGE_KEYS.CONVERSATIONS)
  if (!saved) return null

  return JSON.parse(saved) as unknown
}

function unwrapStoredValue(value: unknown): unknown {
  if (!value || typeof value !== 'object') {
    return value
  }

  if ('version' in value && 'data' in value) {
    return (value as StoredEnvelope<unknown>).data
  }

  return value
}

function writeStoredValue<T>(key: string, data: T): void {
  const payload: StoredEnvelope<T> = {
    version: STORAGE_VERSION,
    data,
  }

  localStorage.setItem(key, JSON.stringify(payload))
}

function trimMessages(messages: Message[]): Message[] {
  if (messages.length <= MAX_STORED_MESSAGES) {
    return messages
  }

  return messages.slice(-MAX_STORED_MESSAGES)
}

function createConversationId(): string {
  if (globalThis.crypto?.randomUUID) {
    return globalThis.crypto.randomUUID()
  }

  return `conversation-${Date.now()}-${Math.random().toString(36).slice(2)}`
}

export function getConversationTitle(messages: Message[]): string {
  const firstUserMessage = messages.find(
    (message) => message.from === 'user' && hasMessageContent(message)
  )
  const content = firstUserMessage
    ? getMessageContent(firstUserMessage).replaceAll(/\s+/g, ' ').trim()
    : ''

  if (content.length <= MAX_CONVERSATION_TITLE_LENGTH) {
    return content
  }

  return `${content.slice(0, MAX_CONVERSATION_TITLE_LENGTH - 1)}...`
}

function getMessageSize(message: Message): number {
  const versionsSize = message.versions.reduce(
    (total, version) => total + version.content.length,
    0
  )
  const reasoningSize = message.reasoning?.content.length ?? 0

  return versionsSize + reasoningSize
}

function truncateText(text: string, maxLength: number): string {
  if (text.includes('](data:image/')) {
    return text
  }

  if (text.length <= maxLength) {
    return text
  }

  if (maxLength <= TRUNCATED_CONTENT_SUFFIX.length) {
    return text.slice(0, maxLength)
  }

  return `${text.slice(0, maxLength - TRUNCATED_CONTENT_SUFFIX.length)}${TRUNCATED_CONTENT_SUFFIX}`
}

type SectionOccurrence = {
  heading: string
  index: number
}

function getSectionOccurrences(text: string): SectionOccurrence[] {
  const occurrences: SectionOccurrence[] = []
  const matches = text.matchAll(SECTION_HEADING_LINE_PATTERN)
  for (const match of matches) {
    const index = match.index
    if (index === undefined) {
      continue
    }

    occurrences.push({
      heading: match[0],
      index,
    })
  }

  return occurrences
}

function getHeadingCounts(
  occurrences: SectionOccurrence[]
): Map<string, number> {
  const counts = new Map<string, number>()

  for (const occurrence of occurrences) {
    counts.set(occurrence.heading, (counts.get(occurrence.heading) ?? 0) + 1)
  }

  return counts
}

function findLastRepeatedSectionRunStart(text: string): number {
  const occurrences = getSectionOccurrences(text)
  const headingCounts = getHeadingCounts(occurrences)
  const lastRepeatedIndexes: number[] = []
  const seenHeadings = new Set<string>()

  for (let index = occurrences.length - 1; index >= 0; index--) {
    const occurrence = occurrences[index]
    const count = headingCounts.get(occurrence.heading) ?? 0

    if (
      count < MIN_REPEATED_SECTION_COUNT ||
      seenHeadings.has(occurrence.heading)
    ) {
      continue
    }

    seenHeadings.add(occurrence.heading)
    lastRepeatedIndexes.push(occurrence.index)
  }

  if (lastRepeatedIndexes.length === 0) {
    return -1
  }

  return Math.min(...lastRepeatedIndexes)
}

function collapseRepeatedSectionSnapshots(text: string): string {
  if (text.length < MIN_PREFIX_COLLAPSE_LENGTH) {
    return text
  }

  const lastRepeatedRunStart = findLastRepeatedSectionRunStart(text)
  if (lastRepeatedRunStart === -1) {
    return text
  }

  return text.slice(lastRepeatedRunStart)
}

function normalizeStoredMessageForLoad(message: Message): Message {
  let changed = false
  const versions = message.versions.map((version) => {
    const collapsedContent = collapseRepeatedSectionSnapshots(version.content)
    const content = truncateText(collapsedContent, MAX_LOADED_MESSAGE_CHARS)

    if (content === version.content && collapsedContent === version.content) {
      return version
    }

    changed = true
    return {
      ...version,
      content,
    }
  })

  const reasoning = message.reasoning
    ? {
        ...message.reasoning,
        content: truncateText(
          message.reasoning.content,
          MAX_LOADED_MESSAGE_CHARS
        ),
      }
    : undefined

  if (reasoning?.content !== message.reasoning?.content) {
    changed = true
  }

  const normalized = changed ? { ...message, versions, reasoning } : message

  if (!isAssistantMessagePending(normalized)) {
    return normalized
  }

  const hasContent = hasMessageContent(normalized)
  const hasReasoning = normalized.reasoning?.content.trim()

  if (!hasContent && !hasReasoning) {
    return normalized
  }

  const completedAt =
    normalized.completedAt ??
    normalized.reasoning?.completedAt ??
    normalized.startedAt ??
    normalized.createdAt ??
    Date.now()

  return completeAssistantTiming(
    {
      ...finalizeMessage(normalized),
      status: MESSAGE_STATUS.COMPLETE,
      isReasoningStreaming: false,
    },
    completedAt
  )
}

function trimMessagesByContentSize(messages: Message[]): Message[] {
  let totalSize = 0
  const result: Message[] = []

  for (let index = messages.length - 1; index >= 0; index--) {
    const message = messages[index]
    const messageSize = getMessageSize(message)

    if (
      result.length > 0 &&
      totalSize + messageSize > MAX_LOADED_MESSAGES_CHARS
    ) {
      break
    }

    totalSize += messageSize
    result.push(message)
  }

  return result.reverse()
}

function normalizeLoadedMessages(messages: Message[]): Message[] {
  const normalized = messages.map(normalizeStoredMessageForLoad)
  const trimmed = trimMessages(normalized)
  const sizeTrimmed = trimMessagesByContentSize(trimmed)

  return sanitizeMessagesOnLoad(sizeTrimmed)
}

function normalizeConversationSession(
  session: PlaygroundConversationSession
): PlaygroundConversationSession {
  const messages = normalizeLoadedMessages(session.messages)

  return {
    ...session,
    messages,
    title: getConversationTitle(messages) || session.title,
    updatedAt: Number.isFinite(session.updatedAt)
      ? session.updatedAt
      : Date.now(),
  }
}

function trimConversations(
  sessions: PlaygroundConversationSession[],
  activeSessionId: string
): PlaygroundConversationSession[] {
  const seen = new Set<string>()
  const uniqueSessions = sessions.filter((session) => {
    if (seen.has(session.id)) {
      return false
    }

    seen.add(session.id)
    return true
  })
  const sorted = [...uniqueSessions].sort((a, b) => b.updatedAt - a.updatedAt)
  const limited = sorted.slice(0, MAX_STORED_CONVERSATIONS)

  if (limited.some((session) => session.id === activeSessionId)) {
    return limited
  }

  const activeSession = uniqueSessions.find(
    (session) => session.id === activeSessionId
  )
  if (!activeSession) {
    return limited
  }

  return [activeSession, ...limited.slice(0, MAX_STORED_CONVERSATIONS - 1)]
}

function getStoredConversationsSize(
  sessions: PlaygroundConversationSession[]
): number {
  return JSON.stringify({
    version: STORAGE_VERSION,
    data: sessions,
  }).length
}

function trimConversationsByStorageSize(
  sessions: PlaygroundConversationSession[],
  activeSessionId: string
): PlaygroundConversationSession[] {
  let result = sessions

  while (
    result.length > 1 &&
    getStoredConversationsSize(result) > MAX_STORED_CONVERSATIONS_BYTES
  ) {
    let oldestIndex = -1
    for (let index = 0; index < result.length; index += 1) {
      if (result[index].id === activeSessionId) continue
      if (
        oldestIndex === -1 ||
        result[index].updatedAt < result[oldestIndex].updatedAt
      ) {
        oldestIndex = index
      }
    }
    if (oldestIndex === -1) break
    result = result.filter((_, index) => index !== oldestIndex)
  }

  return result
}

function createDefaultConversation(
  messages: Message[] = []
): PlaygroundConversationSession {
  return {
    id: DEFAULT_CONVERSATION_ID,
    title: getConversationTitle(messages),
    updatedAt: Date.now(),
    messages: trimMessages(messages),
  }
}

export function createConversationSession(): PlaygroundConversationSession {
  return {
    id: createConversationId(),
    title: '',
    updatedAt: Date.now(),
    messages: [],
  }
}

export type ConversationStorageUsage = {
  capacityBytes: number
  remainingBytes: number
  usedBytes: number
}

export function getConversationStorageUsage(
  sessions: PlaygroundConversationSession[]
): ConversationStorageUsage {
  const usedBytes = getStoredConversationsSize(sessions)

  return {
    capacityBytes: MAX_STORED_CONVERSATIONS_BYTES,
    remainingBytes: Math.max(MAX_STORED_CONVERSATIONS_BYTES - usedBytes, 0),
    usedBytes,
  }
}

export function deleteConversationSession(
  sessions: PlaygroundConversationSession[],
  activeSessionId: string,
  sessionId: string
): {
  activeSessionId: string
  sessions: PlaygroundConversationSession[]
} {
  const remainingSessions = sessions.filter(
    (session) => session.id !== sessionId
  )
  if (remainingSessions.length === sessions.length) {
    return { activeSessionId, sessions }
  }

  if (remainingSessions.length === 0) {
    const session = createConversationSession()
    return { activeSessionId: session.id, sessions: [session] }
  }

  if (remainingSessions.some((session) => session.id === activeSessionId)) {
    return { activeSessionId, sessions: remainingSessions }
  }

  const newestSession = remainingSessions.reduce((newest, session) =>
    session.updatedAt > newest.updatedAt ? session : newest
  )
  return {
    activeSessionId: newestSession.id,
    sessions: remainingSessions,
  }
}

/**
 * Load playground config from localStorage
 */
export function loadConfig(): Partial<PlaygroundConfig> {
  try {
    const saved = readStoredValue(STORAGE_KEYS.CONFIG)
    if (!saved) return {}

    return playgroundConfigSchema.parse(unwrapStoredValue(saved))
  } catch (error) {
    // eslint-disable-next-line no-console
    console.error('Failed to load config:', error)
  }
  return {}
}

/**
 * Save playground config to localStorage
 */
export function saveConfig(config: Partial<PlaygroundConfig>): void {
  try {
    const parsed = playgroundConfigSchema.parse(config)
    writeStoredValue(STORAGE_KEYS.CONFIG, parsed)
  } catch (error) {
    // eslint-disable-next-line no-console
    console.error('Failed to save config:', error)
  }
}

/**
 * Load parameter enabled state from localStorage
 */
export function loadParameterEnabled(): Partial<ParameterEnabled> {
  try {
    const saved = readStoredValue(STORAGE_KEYS.PARAMETER_ENABLED)
    if (!saved) return {}

    return parameterEnabledSchema.parse(unwrapStoredValue(saved))
  } catch (error) {
    // eslint-disable-next-line no-console
    console.error('Failed to load parameter enabled:', error)
  }
  return {}
}

/**
 * Save parameter enabled state to localStorage
 */
export function saveParameterEnabled(
  parameterEnabled: Partial<ParameterEnabled>
): void {
  try {
    const parsed = parameterEnabledSchema.parse(parameterEnabled)
    writeStoredValue(STORAGE_KEYS.PARAMETER_ENABLED, parsed)
  } catch (error) {
    // eslint-disable-next-line no-console
    console.error('Failed to save parameter enabled:', error)
  }
}

/**
 * Load messages from localStorage
 */
export function loadMessages(): Message[] | null {
  try {
    const saved = readStoredMessagesValue()
    if (!saved) return null

    const parsed = messagesSchema.parse(unwrapStoredValue(saved)) as Message[]
    const sanitized = normalizeLoadedMessages(parsed)

    if (sanitized !== parsed) {
      saveMessages(sanitized)
    }

    return sanitized
  } catch (error) {
    // eslint-disable-next-line no-console
    console.error('Failed to load messages:', error)
  }
  return null
}

/**
 * Save messages to localStorage
 */
export function saveMessages(messages: Message[]): void {
  try {
    const trimmed = trimMessages(messages)
    const parsed = messagesSchema.parse(trimmed) as Message[]
    writeStoredValue(STORAGE_KEYS.MESSAGES, parsed)
  } catch (error) {
    // eslint-disable-next-line no-console
    console.error('Failed to save messages:', error)
  }
}

export function loadConversationState(): {
  activeSessionId: string
  sessions: PlaygroundConversationSession[]
} {
  try {
    const saved = readStoredConversationsValue()
    const savedActiveSessionId = readStoredValue(
      STORAGE_KEYS.ACTIVE_CONVERSATION_ID
    )
    const activeSessionId =
      typeof unwrapStoredValue(savedActiveSessionId) === 'string'
        ? (unwrapStoredValue(savedActiveSessionId) as string)
        : ''

    if (saved) {
      const parsed = conversationSessionsSchema.parse(
        unwrapStoredValue(saved)
      ) as PlaygroundConversationSession[]
      const normalized = parsed.map(normalizeConversationSession)
      const fallbackSession = normalized[0] ?? createDefaultConversation()
      const resolvedActiveSessionId =
        normalized.find((session) => session.id === activeSessionId)?.id ??
        fallbackSession.id
      const sessions = trimConversations(normalized, resolvedActiveSessionId)

      return {
        activeSessionId: resolvedActiveSessionId,
        sessions,
      }
    }

    const legacyMessages = loadMessages() ?? []
    const session = createDefaultConversation(legacyMessages)

    return {
      activeSessionId: session.id,
      sessions: [session],
    }
  } catch (error) {
    // eslint-disable-next-line no-console
    console.error('Failed to load playground conversations:', error)
  }

  const session = createDefaultConversation()
  return {
    activeSessionId: session.id,
    sessions: [session],
  }
}

export function saveConversationState(
  sessions: PlaygroundConversationSession[],
  activeSessionId: string
): PlaygroundConversationSession[] {
  try {
    const normalized = sessions.map((session) => ({
      ...session,
      messages: trimMessages(session.messages),
      title: getConversationTitle(session.messages),
    }))
    const trimmed = trimConversationsByStorageSize(
      trimConversations(normalized, activeSessionId),
      activeSessionId
    )
    const parsed = conversationSessionsSchema.parse(
      trimmed
    ) as PlaygroundConversationSession[]
    writeStoredValue(STORAGE_KEYS.CONVERSATIONS, parsed)
    writeStoredValue(STORAGE_KEYS.ACTIVE_CONVERSATION_ID, activeSessionId)

    return parsed
  } catch (error) {
    // eslint-disable-next-line no-console
    console.error('Failed to save playground conversations:', error)
  }

  return sessions
}

/**
 * Clear all playground data
 */
export function clearPlaygroundData(): void {
  try {
    localStorage.removeItem(STORAGE_KEYS.ACTIVE_CONVERSATION_ID)
    localStorage.removeItem(STORAGE_KEYS.CONFIG)
    localStorage.removeItem(STORAGE_KEYS.CONVERSATIONS)
    localStorage.removeItem(STORAGE_KEYS.PARAMETER_ENABLED)
    localStorage.removeItem(STORAGE_KEYS.MESSAGES)
  } catch (error) {
    // eslint-disable-next-line no-console
    console.error('Failed to clear playground data:', error)
  }
}
