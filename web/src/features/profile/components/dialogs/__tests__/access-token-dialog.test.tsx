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

test('shows the generated token until the user closes it', () => {
  const close = vi.fn()
  render(<AccessTokenDialog token='test-token' onClose={close} />)
  expect(screen.getByLabelText('Token')).toHaveValue('test-token')
  expect(screen.getByLabelText('Token')).toHaveAttribute('readonly')
  expect(
    screen.queryByRole('button', { name: 'Regenerate' })
  ).not.toBeInTheDocument()
  fireEvent.click(screen.getAllByRole('button', { name: 'Close' })[0])
  expect(close).toHaveBeenCalledOnce()
})
