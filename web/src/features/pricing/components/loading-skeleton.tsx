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
import { Skeleton } from '@/components/ui/skeleton'

const CARD_SKELETON_IDS = [
  'one',
  'two',
  'three',
  'four',
  'five',
  'six',
  'seven',
  'eight',
]
const CARD_METRIC_WIDTHS = [
  { id: 'context', width: 72 },
  { id: 'max', width: 80 },
  { id: 'input', width: 70 },
  { id: 'output', width: 82 },
  { id: 'cache', width: 78 },
]
const CARD_TAG_WIDTHS = [
  { id: 'endpoint', width: 52 },
  { id: 'vision', width: 64 },
  { id: 'tools', width: 58 },
  { id: 'unit', width: 46 },
]
const FILTER_WIDTHS = [
  { id: 'vendor', width: 80 },
  { id: 'group', width: 90 },
  { id: 'tag', width: 75 },
  { id: 'type', width: 85 },
  { id: 'endpoint', width: 70 },
]
export function LoadingSkeleton() {
  return (
    <div className='space-y-5'>
      <div className='space-y-1.5'>
        <Skeleton className='h-8 w-40' />
        <Skeleton className='h-4 w-52' />
      </div>
      <Skeleton className='h-10 w-full rounded-lg' />
      <FilterBarSkeleton />
      <CardContentSkeleton />
    </div>
  )
}

function CardContentSkeleton() {
  return (
    <div className='space-y-2'>
      {CARD_SKELETON_IDS.map((id) => (
        <div key={id} className='rounded-lg border bg-background p-4'>
          <div className='flex items-start gap-3'>
            <Skeleton className='size-11 shrink-0 rounded-lg' />
            <div className='min-w-0 flex-1'>
              <div className='flex items-start justify-between gap-3'>
                <div className='space-y-2'>
                  <Skeleton className='h-4 w-56' />
                  <Skeleton className='h-3 w-44' />
                </div>
                <Skeleton className='h-7 w-20 rounded-md' />
              </div>
              <Skeleton className='mt-3 h-3 w-4/5' />
              <div className='mt-3 flex flex-wrap gap-3 border-t pt-3'>
                {CARD_METRIC_WIDTHS.map((item) => (
                  <Skeleton
                    key={item.id}
                    className='h-9 rounded-md'
                    style={{ width: item.width }}
                  />
                ))}
              </div>
              <div className='mt-3 flex gap-1.5 border-t pt-2.5'>
                {CARD_TAG_WIDTHS.map((item) => (
                  <Skeleton
                    key={item.id}
                    className='h-5 rounded-md'
                    style={{ width: item.width }}
                  />
                ))}
              </div>
            </div>
          </div>
        </div>
      ))}
    </div>
  )
}

function FilterBarSkeleton() {
  return (
    <div className='space-y-3'>
      <div className='flex flex-wrap items-center justify-between gap-3 rounded-xl border p-3'>
        <div className='flex flex-wrap items-center gap-2'>
          {FILTER_WIDTHS.slice(0, 2).map((item) => (
            <Skeleton
              key={item.id}
              className='h-8 rounded-lg'
              style={{ width: `${item.width}px` }}
            />
          ))}
        </div>
        <Skeleton className='h-8 w-24 rounded-lg' />
      </div>
      <Skeleton className='h-5 w-24' />
    </div>
  )
}
