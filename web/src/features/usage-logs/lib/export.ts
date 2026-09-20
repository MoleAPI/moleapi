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
import { api } from '@/lib/api'

export type ExportProgress = { count: number; bytes: number }

export async function exportUsageLogs(
  range: { start_timestamp: number; end_timestamp: number; all_users: boolean },
  signal: AbortSignal,
  onProgress: (progress: ExportProgress) => void
) {
  let offset = 0
  let complete = false
  let failure = ''
  const parts: string[] = []
  const consume = (text: string) => {
    let newline = text.indexOf('\n', offset)
    while (newline >= 0) {
      const event = JSON.parse(
        text.slice(offset, newline)
      ) as ExportProgress & {
        csv: string
        done: boolean
        error: string
      }
      offset = newline + 1
      if (event.error) failure = event.error
      if (event.csv) parts.push(event.csv)
      onProgress({ count: event.count, bytes: event.bytes })
      complete = event.done
      newline = text.indexOf('\n', offset)
    }
  }
  const response = await api.post<string>('/api/log/export', range, {
    responseType: 'text',
    timeout: 0,
    signal,
    onDownloadProgress: (progress) => {
      const xhr = progress.event?.target as XMLHttpRequest | undefined
      if (typeof xhr?.responseText === 'string') {
        try {
          consume(xhr.responseText)
        } catch {
          failure = 'Export interrupted; select a shorter range and retry'
        }
      }
    },
  })
  consume(response.data)
  if (failure || !complete) {
    throw new Error(
      failure || 'Export interrupted; select a shorter range and retry'
    )
  }
  return new Blob(parts, { type: 'text/csv;charset=utf-8' })
}
