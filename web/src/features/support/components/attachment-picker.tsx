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
}) {
  const { t } = useTranslation()
  const inputRef = useRef<HTMLInputElement>(null)
  const [dragging, setDragging] = useState(false)

  const addFiles = (incoming: File[]) => {
    const result = mergeSupportFiles(props.files, incoming)
    if (result.oversized) props.onRejected(result.oversized.name)
    props.onFilesChange(result.files)
  }

  return (
    <div className='flex flex-col gap-3'>
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
            disabled={props.files.length >= SUPPORT_ATTACHMENT_LIMIT}
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
        <Button
          type='button'
          variant='outline'
          size='sm'
          className='w-fit'
          disabled={props.files.length >= SUPPORT_ATTACHMENT_LIMIT}
          onClick={() => inputRef.current?.click()}
        >
          <HugeiconsIcon
            icon={DocumentAttachmentIcon}
            data-icon='inline-start'
          />
          {t('Choose files')}
        </Button>
      )}
      {props.compact && (
        <input
          ref={inputRef}
          type='file'
          multiple
          accept={SUPPORT_ATTACHMENT_ACCEPT}
          aria-label={t('Attachments')}
          className='sr-only'
          onChange={(event) => {
            addFiles([...(event.target.files ?? [])])
            event.target.value = ''
          }}
        />
      )}

      {props.files.length > 0 && (
        <ItemGroup className='gap-2'>
          {props.files.map((file) => (
            <Item key={`${file.name}-${file.size}`} variant='outline' size='xs'>
              <ItemMedia variant='icon'>
                <HugeiconsIcon icon={DocumentAttachmentIcon} />
              </ItemMedia>
              <ItemContent>
                <ItemTitle>{file.name}</ItemTitle>
              </ItemContent>
              <ItemActions>
                <Button
                  type='button'
                  variant='ghost'
                  size='icon-sm'
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
