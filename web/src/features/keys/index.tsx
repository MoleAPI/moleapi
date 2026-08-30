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
import { ApiIcon } from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { CopyButton } from '@/components/copy-button'
import { SectionPageLayout } from '@/components/layout'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { useStatus } from '@/hooks/use-status'

import { ApiKeysDialogs } from './components/api-keys-dialogs'
import { ApiKeysPrimaryButtons } from './components/api-keys-primary-buttons'
import { ApiKeysProvider } from './components/api-keys-provider'
import { ApiKeysTable } from './components/api-keys-table'
import { buildApiEndpointOptions } from './lib/api-endpoint'

function ApiKeyEndpointGuidance() {
  const { t } = useTranslation()
  const { status } = useStatus()
  const [selectedBaseUrl, setSelectedBaseUrl] = useState('')
  const apiInfoRoutes =
    status?.api_info_enabled !== false && Array.isArray(status?.api_info)
      ? status.api_info
      : []
  const endpointOptions = buildApiEndpointOptions(
    typeof status?.server_address === 'string' ? status.server_address : '',
    typeof window === 'undefined' ? '' : window.location.origin,
    apiInfoRoutes
  )
  const selectedOption =
    endpointOptions.find((option) => option.value === selectedBaseUrl) ||
    endpointOptions[0]
  const baseUrl = selectedOption?.value || '/v1'

  return (
    <div className='bg-card flex min-w-0 shrink-0 flex-col gap-2 rounded-lg border px-3 py-2.5 sm:flex-row sm:items-center sm:gap-3'>
      <div className='flex min-w-0 flex-1 items-center gap-3'>
        <HugeiconsIcon
          icon={ApiIcon}
          className='text-muted-foreground size-4 shrink-0'
          strokeWidth={2}
          aria-hidden='true'
        />
        <div className='min-w-0 flex-1'>
          <p className='text-sm font-medium'>{t('Base URL')}</p>
          <code
            className='text-muted-foreground block truncate text-xs'
            title={baseUrl}
          >
            {baseUrl}
          </code>
          {selectedOption?.description && (
            <p className='text-muted-foreground/70 truncate text-xs'>
              {selectedOption.description}
            </p>
          )}
        </div>
      </div>
      <div className='flex min-w-0 shrink-0 items-center gap-2'>
        {endpointOptions.length > 1 && (
          <Select
            items={endpointOptions.map((option) => ({
              value: option.value,
              label: option.label,
            }))}
            value={baseUrl}
            onValueChange={(value) => setSelectedBaseUrl(value || '')}
          >
            <SelectTrigger
              aria-label={t('Route')}
              className='bg-background/80 min-w-0 flex-1 sm:w-40'
              size='sm'
            >
              <SelectValue />
            </SelectTrigger>
            <SelectContent
              align='end'
              alignItemWithTrigger={false}
              className='w-56'
            >
              <SelectGroup>
                {endpointOptions.map((option) => (
                  <SelectItem key={option.value} value={option.value}>
                    <span className='flex min-w-0 flex-col'>
                      <span className='truncate font-medium'>
                        {option.label}
                      </span>
                      {option.description && (
                        <span className='text-muted-foreground truncate text-xs'>
                          {option.description}
                        </span>
                      )}
                    </span>
                  </SelectItem>
                ))}
              </SelectGroup>
            </SelectContent>
          </Select>
        )}
        <CopyButton
          value={baseUrl}
          size='icon'
          className='size-8'
          tooltip={t('Copy URL')}
        />
      </div>
    </div>
  )
}

export function ApiKeys() {
  const { t } = useTranslation()
  return (
    <ApiKeysProvider>
      <SectionPageLayout fixedContent>
        <SectionPageLayout.Title>{t('API Keys')}</SectionPageLayout.Title>
        <SectionPageLayout.Actions>
          <ApiKeysPrimaryButtons />
        </SectionPageLayout.Actions>
        <SectionPageLayout.Content>
          <div className='flex h-full min-h-0 flex-col gap-2.5 sm:gap-3'>
            <ApiKeyEndpointGuidance />
            <div className='min-h-0 flex-1'>
              <ApiKeysTable />
            </div>
          </div>
        </SectionPageLayout.Content>
      </SectionPageLayout>

      <ApiKeysDialogs />
    </ApiKeysProvider>
  )
}
