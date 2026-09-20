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
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from '@testing-library/react'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { afterEach, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'
import { useAuthStore } from '@/stores/auth-store'

import { exportUsageLogs } from '../../lib/export'
import { LogExportDialog } from '../dialogs/log-export-dialog'
import { UsageLogsProvider } from '../usage-logs-provider'

afterEach(() => {
  cleanup()
  useAuthStore.getState().auth.setUser(null)
})

test('export consumes partial network chunks once and requires a completion marker', async () => {
  const first =
    JSON.stringify({
      csv: 'header\n',
      count: 0,
      bytes: 7,
      done: false,
      error: '',
    }) + '\n'
  const last =
    JSON.stringify({
      csv: 'row\n',
      count: 1,
      bytes: 11,
      done: true,
      error: '',
    }) + '\n'
  vi.spyOn(api, 'post').mockImplementation(async (_url, _body, config) => {
    for (const text of [first.slice(0, 4), first, first + last.slice(0, 4)]) {
      config?.onDownloadProgress?.({
        loaded: text.length,
        bytes: text.length,
        lengthComputable: false,
        event: { target: { responseText: text } },
      })
    }
    return { data: first + last }
  })
  const progress = vi.fn()
  const result = await exportUsageLogs(
    { start_timestamp: 100, end_timestamp: 200, all_users: false },
    new AbortController().signal,
    progress
  )
  expect(result.size).toBe(11)
  expect(progress).toHaveBeenLastCalledWith({ count: 1, bytes: 11 })
  vi.mocked(api.post).mockResolvedValue({ data: first })
  await expect(
    exportUsageLogs(
      { start_timestamp: 100, end_timestamp: 200, all_users: false },
      new AbortController().signal,
      progress
    )
  ).rejects.toThrow('Export interrupted')
})

test('export dialog validates range, shows progress, prevents duplicate starts and cancels', async () => {
  useAuthStore
    .getState()
    .auth.setUser({ id: 7, username: 'export-user', role: 1 })
  const i18n = i18next.createInstance()
  await i18n.use(initReactI18next).init({ lng: 'en', resources: {} })
  let signal: AbortSignal | undefined
  const post = vi
    .spyOn(api, 'post')
    .mockImplementation(async (_url, _body, config) => {
      signal = config?.signal as AbortSignal
      const text =
        JSON.stringify({
          csv: 'header\n',
          count: 500,
          bytes: 1048576,
          done: false,
          error: '',
        }) + '\n'
      config?.onDownloadProgress?.({
        loaded: text.length,
        bytes: text.length,
        lengthComputable: false,
        event: { target: { responseText: text } },
      })
      return new Promise((_resolve, reject) =>
        signal?.addEventListener('abort', () => reject(new Error('cancelled')))
      )
    })
  render(
    <I18nextProvider i18n={i18n}>
      <UsageLogsProvider>
        <LogExportDialog
          startTime={new Date('2026-01-01T08:30:15')}
          endTime={new Date('2026-01-01T09:30:25')}
        />
      </UsageLogsProvider>
    </I18nextProvider>
  )
  fireEvent.click(screen.getByRole('button', { name: 'Export logs' }))
  const download = await screen.findByRole('button', { name: 'Download CSV' })
  expect(download).not.toBeDisabled()
  expect(screen.getByLabelText('Start time')).toHaveValue(
    '2026-01-01T08:30:15.000'
  )
  expect(screen.getByLabelText('End time')).toHaveValue(
    '2026-01-01T09:30:25.000'
  )
  expect(
    screen.getByText(/The current log list time range/)
  ).toBeInTheDocument()
  fireEvent.change(screen.getByLabelText('Start time'), {
    target: { value: '2026-01-02T00:00:00' },
  })
  fireEvent.change(screen.getByLabelText('End time'), {
    target: { value: '2026-01-01T00:00:00' },
  })
  expect(download).toBeDisabled()
  expect(screen.getByRole('alert')).toHaveTextContent('Invalid date range')
  fireEvent.change(screen.getByLabelText('End time'), {
    target: { value: '2026-01-03T00:00:00' },
  })
  fireEvent.click(download)
  await waitFor(() =>
    expect(screen.getByRole('status')).toHaveTextContent('500 records')
  )
  expect(download).toBeDisabled()
  expect(post).toHaveBeenCalledTimes(1)
  expect(screen.getByRole('progressbar')).not.toHaveAttribute('aria-valuenow')
  fireEvent.click(screen.getByRole('button', { name: 'Cancel' }))
  await waitFor(() => expect(download).not.toBeDisabled())
  expect(signal?.aborted).toBe(true)
  expect(screen.queryByText('Export complete')).toBeNull()
})
