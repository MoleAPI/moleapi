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
import { afterEach, beforeEach, describe, test } from 'node:test'

import { STORAGE_KEYS } from '../../constants'
import type { Message, PlaygroundConversationSession } from '../../types'
import { loadConversationState, saveConversationState } from './storage'

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
})
