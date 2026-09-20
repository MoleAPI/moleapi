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
  Delete02Icon,
  DocumentAttachmentIcon,
  FolderOpenIcon,
} from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import {
  Item,
  ItemActions,
  ItemContent,
  ItemGroup,
  ItemMedia,
  ItemTitle,
} from '@/components/ui/item'
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import { cn } from '@/lib/utils'

import {
  SUPPORT_ATTACHMENT_ACCEPT,
  SUPPORT_ATTACHMENT_LIMIT,
  mergeSupportFiles,
} from '../constants'

export function AttachmentPicker(props: {
  files: File[]
  onFilesChange: (files: File[]) => void
  onRejected: (fileName: string) => void
  compact?: boolean
  disabled?: boolean
}) {
  const { t } = useTranslation()
  const inputRef = useRef<HTMLInputElement>(null)
  const [dragging, setDragging] = useState(false)

  const addFiles = (incoming: File[]) => {
    if (props.disabled) return
    const result = mergeSupportFiles(props.files, incoming)
    if (result.oversized) props.onRejected(result.oversized.name)
    props.onFilesChange(result.files)
  }

  return (
    <div className={cn(props.compact ? 'contents' : 'flex flex-col gap-2')}>
      {!props.compact && (
        <div
          className={cn(
            'flex flex-col items-center gap-2 rounded-lg border border-dashed px-4 py-5 text-center transition-colors',
            dragging ? 'border-primary bg-primary/5' : 'bg-muted/20'
          )}
          onDragOver={(event) => {
            event.preventDefault()
            setDragging(true)
          }}
          onDragLeave={() => setDragging(false)}
          onDrop={(event) => {
            event.preventDefault()
            setDragging(false)
            addFiles([...event.dataTransfer.files])
          }}
        >
          <HugeiconsIcon
            icon={DocumentAttachmentIcon}
            className='text-muted-foreground size-5'
            aria-hidden='true'
          />
          <div>
            <p className='text-sm font-medium'>
              {t('Attach files or screenshots')}
            </p>
            <p className='text-muted-foreground mt-0.5 text-xs'>
              {t(
                'Drop files here, choose files, or paste a screenshot into the detailed description.'
              )}
            </p>
          </div>
          <Button
            type='button'
            variant='outline'
            size='sm'
            disabled={
              props.disabled || props.files.length >= SUPPORT_ATTACHMENT_LIMIT
            }
            onClick={() => inputRef.current?.click()}
          >
            <HugeiconsIcon icon={FolderOpenIcon} data-icon='inline-start' />
            {t('Choose files')}
          </Button>
          <label className='sr-only' htmlFor='support-attachments'>
            {t('Attachments')}
          </label>
          <input
            ref={inputRef}
            id='support-attachments'
            type='file'
            multiple
            accept={SUPPORT_ATTACHMENT_ACCEPT}
            className='sr-only'
            onChange={(event) => {
              addFiles([...(event.target.files ?? [])])
              event.target.value = ''
            }}
          />
        </div>
      )}

      {props.compact && (
        <Tooltip>
          <TooltipTrigger
            render={
              <Button
                type='button'
                variant='ghost'
                size='sm'
                className='shrink-0'
                aria-label={t('Attach files or screenshots')}
                disabled={
                  props.disabled ||
                  props.files.length >= SUPPORT_ATTACHMENT_LIMIT
                }
                onClick={() => inputRef.current?.click()}
              />
            }
          >
            <HugeiconsIcon
              icon={DocumentAttachmentIcon}
              data-icon='inline-start'
            />
            <span>{t('Attach files or screenshots')}</span>
          </TooltipTrigger>
          <TooltipContent>
            {t('Attach files or screenshots')} · {t('Up to 3 files, 5 MB each')}
          </TooltipContent>
        </Tooltip>
      )}
      {props.compact && (
        <input
          ref={inputRef}
          type='file'
          multiple
          accept={SUPPORT_ATTACHMENT_ACCEPT}
          aria-label={t('Attachments')}
          disabled={props.disabled}
          className='sr-only'
          onChange={(event) => {
            addFiles([...(event.target.files ?? [])])
            event.target.value = ''
          }}
        />
      )}

      {props.files.length > 0 && (
        <ItemGroup
          className={cn(
            'gap-2',
            props.compact && 'order-last min-w-0 basis-full flex-row flex-wrap'
          )}
        >
          {props.files.map((file) => (
            <Item
              key={`${file.name}-${file.size}`}
              variant='outline'
              size='xs'
              className={cn(
                props.compact &&
                  'w-auto max-w-44 flex-nowrap gap-1 rounded-md px-2 py-0.5'
              )}
            >
              <ItemMedia variant='icon'>
                <HugeiconsIcon icon={DocumentAttachmentIcon} />
              </ItemMedia>
              <ItemContent className='min-w-0'>
                <ItemTitle className='block w-full truncate' title={file.name}>
                  {file.name}
                </ItemTitle>
              </ItemContent>
              <ItemActions className='shrink-0'>
                <Button
                  type='button'
                  variant='ghost'
                  size='icon-sm'
                  disabled={props.disabled}
                  onClick={() =>
                    props.onFilesChange(
                      props.files.filter((item) => item !== file)
                    )
                  }
                >
                  <HugeiconsIcon icon={Delete02Icon} />
                  <span className='sr-only'>{t('Remove attachment')}</span>
                </Button>
              </ItemActions>
            </Item>
          ))}
        </ItemGroup>
      )}
    </div>
  )
}
