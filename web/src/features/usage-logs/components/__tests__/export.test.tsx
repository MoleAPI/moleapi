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

import { usageLogSchema } from '../../data/schema'
import { estimateLogExportBytes, exportUsageLogs } from '../../lib/export'
import { LogExportDialog } from '../dialogs/log-export-dialog'
import { UsageLogsProvider } from '../usage-logs-provider'

afterEach(() => {
  cleanup()
  useAuthStore.getState().auth.setUser(null)
})

const sampleLog = usageLogSchema.parse({
  id: 1,
  user_id: 7,
  username: 'localuser',
  created_at: 1789862400,
  type: 2,
  content: 'start-boundary',
  model_name: 'export-fixture',
  group: 'default',
  token_name: 'export-token',
  quota: 7,
})

test('size estimates use only exported fields and distinguish empty results from missing samples', () => {
  expect(estimateLogExportBytes([sampleLog], 1, false)).toBe(211)
  expect(
    estimateLogExportBytes(
      [{ ...sampleLog, other: 'private metadata'.repeat(100) }],
      1,
      false
    )
  ).toBe(211)
  expect(estimateLogExportBytes([sampleLog], 1, true)).toBeGreaterThan(211)
  const headerBytes = estimateLogExportBytes([], 0, false)!
  expect(estimateLogExportBytes([sampleLog], 10, false)).toBe(
    headerBytes + (211 - headerBytes) * 10
  )
  expect(estimateLogExportBytes([], 1, false)).toBeUndefined()
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
  const get = vi
    .spyOn(api, 'get')
    .mockResolvedValue({
      data: { success: true, data: { total: 1, items: [sampleLog] } },
    })
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
    <QueryClientProvider
      client={
        new QueryClient({ defaultOptions: { queries: { retry: false } } })
      }
    >
      <I18nextProvider i18n={i18n}>
        <UsageLogsProvider>
          <LogExportDialog
            startTime={new Date('2026-01-01T08:30:15')}
            endTime={new Date('2026-01-01T09:30:25')}
            filters={{
              type: 2,
              model_name: 'deepseek-%',
              group: 'vip',
              token_name: 'my token',
              request_id: 'request-1',
              username: 'foreign-user',
              channel: 99,
              p: 8,
              page_size: 20,
            }}
          />
        </UsageLogsProvider>
      </I18nextProvider>
    </QueryClientProvider>
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
  expect(screen.getByText(/The current log list filters/)).toBeInTheDocument()
  for (const value of [
    'Consume',
    'deepseek-%',
    'vip',
    'my token',
    'request-1',
  ]) {
    expect(screen.getByText(value, { exact: true })).toBeInTheDocument()
  }
  expect(screen.queryByText('foreign-user')).toBeNull()
  expect(await screen.findByText(/Estimated: 1 records/)).toBeInTheDocument()
  const estimateUrl = new URL(
    String(get.mock.lastCall?.[0]),
    'https://test.invalid'
  )
  expect(estimateUrl.pathname).toBe('/api/log/self')
  expect(estimateUrl.searchParams.get('model_name')).toBe('deepseek-%')
  expect(estimateUrl.searchParams.get('type')).toBe('2')
  expect(estimateUrl.searchParams.get('group')).toBe('vip')
  expect(estimateUrl.searchParams.get('token_name')).toBe('my token')
  expect(estimateUrl.searchParams.has('username')).toBe(false)
  expect(post).not.toHaveBeenCalled()
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
  expect(post.mock.lastCall?.[1]).toEqual({
    start_timestamp: Math.floor(
      new Date('2026-01-02T00:00:00').getTime() / 1000
    ),
    end_timestamp: Math.floor(new Date('2026-01-03T00:00:00').getTime() / 1000),
    all_users: false,
    type: 2,
    model_name: 'deepseek-%',
    group: 'vip',
    token_name: 'my token',
    request_id: 'request-1',
    upstream_request_id: undefined,
  })
  expect(screen.getByRole('progressbar')).not.toHaveAttribute('aria-valuenow')
  fireEvent.click(screen.getByRole('button', { name: 'Cancel' }))
  await waitFor(() => expect(download).not.toBeDisabled())
  expect(signal?.aborted).toBe(true)
  expect(screen.queryByText('Export complete')).toBeNull()
  get.mockRejectedValue(new Error('unavailable'))
  fireEvent.change(screen.getByLabelText('End time'), {
    target: { value: '2026-01-04T00:00:00' },
  })
  expect(
    await screen.findByText('Estimate unavailable. You can still export.')
  ).toBeInTheDocument()
  expect(download).not.toBeDisabled()
})
