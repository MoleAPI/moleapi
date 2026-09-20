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

import type { UsageLog } from '../data/schema'
import type { GetLogsParams } from '../types'

export type ExportProgress = { count: number; bytes: number }

// ponytail: estimate from one existing list page, not another full export scan.
// Highly variable row lengths can differ from the sample; only the completed export is exact.
export function estimateLogExportBytes(
  rows: UsageLog[],
  count: number,
  allUsers: boolean
) {
  if (count > 0 && rows.length === 0) return undefined
  let header =
    'time_utc,type,model,token_name,input_tokens,output_tokens,quota_units,duration_seconds,stream,group,request_id,content'
  if (allUsers) header += ',user_id,username,channel_id'
  let sampleBytes = 0
  for (const row of rows) {
    const values = [
      new Date(row.created_at * 1000).toISOString().replace('.000Z', 'Z'),
      row.type,
      row.model_name,
      row.token_name,
      row.prompt_tokens ?? 0,
      row.completion_tokens ?? 0,
      row.quota ?? 0,
      row.use_time ?? 0,
      row.is_stream ?? false,
      row.group,
      row.request_id,
      row.content,
      ...(allUsers ? [row.user_id, row.username, row.channel ?? 0] : []),
    ]
    const line = values
      .map((value) => {
        let text = String(value ?? '')
        if (/^ *[=+@\-\t\r\n]/.test(text)) text = `'${text}`
        return /^[\s]|[,"\r\n]/.test(text)
          ? `"${text.replaceAll('"', '""')}"`
          : text
      })
      .join(',')
    sampleBytes += new Blob([line, '\n']).size
  }
  return (
    new Blob(['\uFEFF', header, '\n']).size +
    Math.ceil(rows.length ? (sampleBytes / rows.length) * count : 0)
  )
}

export async function exportUsageLogs(
  range: Omit<GetLogsParams, 'p' | 'page_size'> & {
    start_timestamp: number
    end_timestamp: number
    all_users: boolean
  },
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
