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
import { fireEvent, render, screen } from '@testing-library/react'
import { expect, test, vi } from 'vitest'

import { AccessTokenDialog } from '../access-token-dialog'

const generate = vi.fn(async () => true)

vi.mock('../../../hooks', () => ({
  useAccessToken: () => ({
    token: '',
    generating: false,
    generate,
    clearToken: vi.fn(),
  }),
}))

vi.mock('@/components/dialog', () => ({
  Dialog: (props: {
    open: boolean
    children: React.ReactNode
    footer: React.ReactNode
  }) => (props.open ? <div>{props.children}{props.footer}</div> : null),
}))

vi.mock('@/components/confirm-dialog', () => ({
  ConfirmDialog: (props: {
    open: boolean
    confirmText: React.ReactNode
    handleConfirm: () => void
  }) => props.open ? (
    <button type='button' onClick={props.handleConfirm}>
      {props.confirmText}
    </button>
  ) : null,
}))

test('rotates an access token only after explicit confirmation', async () => {
  render(<AccessTokenDialog open onOpenChange={() => undefined} />)

  expect(generate).not.toHaveBeenCalled()
  fireEvent.click(screen.getByRole('button', { name: 'Regenerate' }))
  expect(generate).not.toHaveBeenCalled()

  fireEvent.click(screen.getByRole('button', { name: 'Regenerate token' }))
  expect(generate).toHaveBeenCalledOnce()
})
