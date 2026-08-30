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
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import i18next from 'i18next'
import { beforeAll, expect, test, vi } from 'vitest'

import { getConversationStorageUsage } from '../../../lib'
import { MAX_STORED_CONVERSATIONS_BYTES } from '../../../lib/storage/storage-schema'
import type { PlaygroundConversationSession } from '../../../types'
import { PlaygroundHistorySidebar } from '../playground-history-sidebar'

const sessions: PlaygroundConversationSession[] = [
  {
    id: 'first',
    title: 'First conversation',
    updatedAt: 2,
    messages: [],
  },
  {
    id: 'second',
    title: 'Second conversation',
    updatedAt: 1,
    messages: [],
  },
]

beforeAll(() => {
  i18next.addResourceBundle('en', 'translation', {
    Cancel: 'Cancel',
    Delete: 'Delete',
    'Delete conversation?': 'Delete conversation?',
    'Delete "{{title}}" from local history? This action cannot be undone.':
      'Delete "{{title}}" from local history? This action cannot be undone.',
    'Local storage remaining: {{size}}': 'Local storage remaining: {{size}}',
    'Storage is running low. Oldest records will be deleted automatically.':
      'Storage is running low. Oldest records will be deleted automatically.',
  })
})

test('requires confirmation before deleting a hovered history record', async () => {
  const user = userEvent.setup()
  const onDeleteConversation = vi.fn()

  render(
    <PlaygroundHistorySidebar
      activeSessionId='first'
      onDeleteConversation={onDeleteConversation}
      onNewConversation={() => undefined}
      onSelectConversation={() => undefined}
      sessions={sessions}
      storageUsage={getConversationStorageUsage(sessions)}
    />
  )

  const deleteButton = screen.getByRole('button', {
    name: 'Delete: First conversation',
  })
  expect(deleteButton).toHaveClass('opacity-0')
  expect(deleteButton).toHaveClass('group-hover:opacity-100')
  expect(deleteButton).toHaveClass('group-focus-within:opacity-100')

  await user.click(deleteButton)

  expect(onDeleteConversation).not.toHaveBeenCalled()
  expect(screen.getByRole('alertdialog')).toHaveTextContent(
    'Delete "First conversation" from local history? This action cannot be undone.'
  )

  await user.click(screen.getByRole('button', { name: /^Delete$/ }))

  expect(onDeleteConversation).toHaveBeenCalledWith('first')
})

test('shows the remaining local history storage in a progress bar', () => {
  render(
    <PlaygroundHistorySidebar
      activeSessionId='first'
      onDeleteConversation={() => undefined}
      onNewConversation={() => undefined}
      onSelectConversation={() => undefined}
      sessions={sessions}
      storageUsage={getConversationStorageUsage(sessions)}
    />
  )

  expect(screen.getByRole('progressbar')).toBeInTheDocument()
  expect(screen.getByText(/Local storage remaining: .* MB/)).toBeVisible()
})

test('uses a dark red warning when local history storage is running low', () => {
  const nearCapacitySession: PlaygroundConversationSession = {
    id: 'large',
    title: 'Large conversation',
    updatedAt: 1,
    messages: [
      {
        key: 'large-message',
        from: 'user',
        versions: [
          {
            id: 'large-version',
            content: 'x'.repeat(
              Math.ceil(MAX_STORED_CONVERSATIONS_BYTES * 0.81)
            ),
          },
        ],
      },
    ],
  }

  render(
    <PlaygroundHistorySidebar
      activeSessionId='large'
      onDeleteConversation={() => undefined}
      onNewConversation={() => undefined}
      onSelectConversation={() => undefined}
      sessions={[nearCapacitySession]}
      storageUsage={getConversationStorageUsage([nearCapacitySession])}
    />
  )

  const warning = screen.getByText(
    'Storage is running low. Oldest records will be deleted automatically.'
  )
  expect(warning.parentElement).toHaveClass('bg-red-950/10', 'text-red-900')
})
