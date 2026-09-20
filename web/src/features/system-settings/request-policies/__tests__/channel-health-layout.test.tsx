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
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { cleanup, render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, it } from 'vitest'

import { ChannelHealthSection } from '../channel-health-section'
import { defaultRequestPolicySettings } from '../defaults'

let client: QueryClient

beforeEach(() => {
  client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
})

afterEach(() => {
  cleanup()
  client.clear()
})

function show() {
  render(
    <QueryClientProvider client={client}>
      <ChannelHealthSection defaultValues={defaultRequestPolicySettings} />
    </QueryClientProvider>
  )
}

function formItemOf(control: HTMLElement) {
  return control.closest('[data-slot=form-item]')
}

describe('channel health layout', () => {
  it('preserves local smart checks and exposes custom prompt inputs', async () => {
    render(
      <QueryClientProvider client={client}>
        <ChannelHealthSection
          defaultValues={{
            ...defaultRequestPolicySettings,
            'monitor_setting.channel_test_mode': 'auto_detect',
            'monitor_setting.channel_test_type': 'intelligence',
          }}
        />
      </QueryClientProvider>
    )
    expect(
      screen.getByRole('combobox', { name: 'Channel test mode' })
    ).toHaveTextContent('Auto-detect channels')
    const probeType = screen.getByRole('combobox', { name: 'Probe type' })
    expect(probeType).toHaveTextContent('Intelligence check')
    await userEvent.click(probeType)
    await userEvent.click(
      screen.getByRole('option', { name: 'Custom prompt check' })
    )
    expect(screen.getByRole('textbox', { name: 'Custom prompt' })).toBeVisible()
    expect(
      screen.getByRole('textbox', { name: 'Expected answer' })
    ).toBeVisible()
  })
  it('nests only the test mode and interval under the scheduled channel tests switch', () => {
    show()
    const options = screen.getByRole('group', {
      name: 'Scheduled test options',
    })
    expect(within(options).getByRole('combobox')).toBeVisible()
    expect(
      within(options).getByRole('spinbutton', {
        name: 'Test interval (minutes)',
      })
    ).toBeVisible()
    expect(
      within(options).queryByRole('spinbutton', {
        name: 'Channel test concurrency',
      })
    ).not.toBeInTheDocument()
    expect(within(options).queryByRole('switch')).not.toBeInTheDocument()
  })

  it('marks every switch row as full width so it never shares a grid row with an input', () => {
    show()
    for (const name of [
      'Scheduled channel tests',
      'Re-enable on success',
      'Disable on failure',
    ]) {
      expect(formItemOf(screen.getByRole('switch', { name }))).toHaveAttribute(
        'data-settings-form-span',
        'full'
      )
    }
  })

  it('lays out the auto-disable switch, inputs and keyword list in one shared grid', () => {
    show()
    const grid = formItemOf(
      screen.getByRole('switch', { name: 'Disable on failure' })
    )?.parentElement
    expect(grid).toHaveAttribute('data-settings-form-span', 'full')
    for (const control of [
      screen.getByRole('spinbutton', {
        name: 'Health check timeout threshold (seconds)',
      }),
      screen.getByRole('textbox', { name: 'Auto-disable status codes' }),
      screen.getByRole('textbox', { name: 'Failure keywords' }),
    ]) {
      expect(formItemOf(control)?.parentElement).toBe(grid)
    }
  })
})
