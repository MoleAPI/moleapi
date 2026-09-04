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
import { vi, describe, expect, test } from 'vitest'

vi.mock('../channels-provider', () => ({
  useChannels: () => ({ sensitiveVisible: true }),
}))
vi.mock('@/components/provider-badge', () => ({ ProviderBadge: () => null }))
vi.mock('@/components/group-badge', () => ({ GroupBadge: () => null }))

const { useChannelsColumns } = await import('../channels-columns')

function ChannelHeaders() {
  const columns = useChannelsColumns({ enableSelection: false })
  return (
    <>
      {columns.map((column) =>
        typeof column.header === 'string' ? (
          <span key={column.id ?? column.header}>{column.header}</span>
        ) : null
      )}
    </>
  )
}

describe('channel table columns', () => {
  test('shows used quota, success rate, and reliability as separate columns', () => {
    render(<ChannelHeaders />)

    expect(screen.getByText('Used')).toBeInTheDocument()
    expect(screen.getByText('Success rate')).toBeInTheDocument()
    expect(screen.getByText('Reliability')).toBeInTheDocument()
    expect(screen.queryByText('Used / Remaining')).not.toBeInTheDocument()
  })
})
