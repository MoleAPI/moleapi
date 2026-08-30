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

import { afterEach, beforeEach, describe, test } from 'vitest'

import { STORAGE_KEYS } from '../../constants'
import type { Message, PlaygroundConversationSession } from '../../types'
import {
  deleteConversationSession,
  getConversationStorageUsage,
  loadConversationState,
  saveConversationState,
} from './storage'
import { MAX_STORED_CONVERSATIONS_BYTES } from './storage-schema'

const originalLocalStorageDescriptor = Object.getOwnPropertyDescriptor(
  globalThis,
  'localStorage'
)

function installLocalStorageMock() {
  const store = new Map<string, string>()
  const storage = {
    get length() {
      return store.size
    },
    clear: () => store.clear(),
    getItem: (key: string) => store.get(key) ?? null,
    key: (index: number) => [...store.keys()][index] ?? null,
    removeItem: (key: string) => store.delete(key),
    setItem: (key: string, value: string) => store.set(key, String(value)),
  } as Storage

  Object.defineProperty(globalThis, 'localStorage', {
    configurable: true,
    value: storage,
  })
}

function createUserMessage(content: string): Message {
  return {
    key: `message-${content}`,
    from: 'user',
    versions: [{ id: `version-${content}`, content }],
  }
}

function createSession(
  id: string,
  updatedAt: number
): PlaygroundConversationSession {
  return {
    id,
    title: id,
    updatedAt,
    messages: [createUserMessage(id)],
  }
}

function createLargeSession(
  id: string,
  updatedAt: number,
  size: number
): PlaygroundConversationSession {
  return {
    id,
    title: id,
    updatedAt,
    messages: [
      {
        key: `message-${id}`,
        from: 'user',
        versions: [{ id: `version-${id}`, content: 'x'.repeat(size) }],
      },
    ],
  }
}

beforeEach(() => {
  installLocalStorageMock()
})

afterEach(() => {
  if (originalLocalStorageDescriptor) {
    Object.defineProperty(
      globalThis,
      'localStorage',
      originalLocalStorageDescriptor
    )
    return
  }

  delete (globalThis as { localStorage?: Storage }).localStorage
})

describe('playground conversation storage', () => {
  test('reports remaining history space against the automatic cleanup cap', () => {
    const usage = getConversationStorageUsage([createSession('active', 1)])

    assert.equal(usage.capacityBytes, MAX_STORED_CONVERSATIONS_BYTES)
    assert.ok(usage.usedBytes > 0)
    assert.equal(usage.remainingBytes, usage.capacityBytes - usage.usedBytes)
  })

  test('selects the newest remaining conversation after deleting the active one', () => {
    const nextState = deleteConversationSession(
      [
        createSession('active', 3),
        createSession('old', 1),
        createSession('newest', 2),
      ],
      'active',
      'active'
    )

    assert.equal(nextState.activeSessionId, 'newest')
    assert.deepEqual(
      nextState.sessions.map((session) => session.id),
      ['old', 'newest']
    )
  })

  test('creates an empty conversation after deleting the final record', () => {
    const nextState = deleteConversationSession(
      [createSession('active', 1)],
      'active',
      'active'
    )

    assert.equal(nextState.sessions.length, 1)
    assert.equal(nextState.sessions[0]?.id, nextState.activeSessionId)
    assert.deepEqual(nextState.sessions[0]?.messages, [])
  })

  test('loads legacy single-message storage as the default conversation', () => {
    localStorage.setItem(
      STORAGE_KEYS.MESSAGES,
      JSON.stringify({
        version: 1,
        data: [createUserMessage('hello from old storage')],
      })
    )

    const state = loadConversationState()

    assert.equal(state.activeSessionId, 'default')
    assert.equal(state.sessions.length, 1)
    assert.equal(state.sessions[0]?.title, 'hello from old storage')
    assert.equal(
      state.sessions[0]?.messages[0]?.versions[0]?.content,
      'hello from old storage'
    )
  })

  test('keeps the active conversation when saved history exceeds the local cap', () => {
    const oldActiveSession = createSession('active-old', 1)
    const newerSessions = Array.from({ length: 12 }, (_, index) =>
      createSession(`new-${index}`, 100 + index)
    )

    const saved = saveConversationState(
      [oldActiveSession, ...newerSessions],
      oldActiveSession.id
    )

    assert.equal(saved.length, 12)
    assert.ok(saved.some((session) => session.id === oldActiveSession.id))
  })

  test('keeps stored base64 image markdown intact when reloading', () => {
    const imageMarkdown = `![generated image](data:image/png;base64,${'A'.repeat(45_000)})`
    localStorage.setItem(
      STORAGE_KEYS.CONVERSATIONS,
      JSON.stringify({
        version: 1,
        data: [
          {
            id: 'image-session',
            title: 'image',
            updatedAt: 1,
            messages: [
              {
                key: 'assistant-image',
                from: 'assistant',
                versions: [{ id: 'v1', content: imageMarkdown }],
              },
            ],
          },
        ],
      })
    )

    const state = loadConversationState()

    assert.equal(
      state.sessions[0]?.messages[0]?.versions[0]?.content,
      imageMarkdown
    )
  })

  test('does not delete oversized saved conversations on load', () => {
    const largeContent = `![generated image](data:image/png;base64,${'A'.repeat(MAX_STORED_CONVERSATIONS_BYTES)})`
    localStorage.setItem(
      STORAGE_KEYS.CONVERSATIONS,
      JSON.stringify({
        version: 1,
        data: [
          {
            id: 'large-image-session',
            title: 'large image',
            updatedAt: 1,
            messages: [
              {
                key: 'assistant-large-image',
                from: 'assistant',
                versions: [{ id: 'v1', content: largeContent }],
              },
            ],
          },
        ],
      })
    )

    const state = loadConversationState()

    assert.equal(state.sessions[0]?.id, 'large-image-session')
    assert.ok(localStorage.getItem(STORAGE_KEYS.CONVERSATIONS))
  })

  test('trims old conversations before saving oversized history', () => {
    const activeSession = createLargeSession('active', 1, 1000)
    const oldSessions = Array.from({ length: 12 }, (_, index) =>
      createLargeSession(
        `old-${index}`,
        index + 2,
        Math.ceil(MAX_STORED_CONVERSATIONS_BYTES / 4)
      )
    )

    const saved = saveConversationState(
      [activeSession, ...oldSessions],
      activeSession.id
    )
    const raw = localStorage.getItem(STORAGE_KEYS.CONVERSATIONS)

    assert.ok(raw)
    assert.ok(raw.length <= MAX_STORED_CONVERSATIONS_BYTES)
    assert.ok(saved.some((session) => session.id === activeSession.id))
    assert.ok(saved.length < 13)
  })
})
