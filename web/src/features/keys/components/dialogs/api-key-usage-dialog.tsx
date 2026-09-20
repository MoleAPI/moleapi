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
import { ShieldCheck } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import {
  CodeBlock,
  CodeBlockCopyButton,
} from '@/components/ai-elements/code-block'
import { CopyButton } from '@/components/copy-button'
import { Dialog } from '@/components/dialog'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { useStatus } from '@/hooks/use-status'

import { buildApiEndpointOptions } from '../../lib/api-endpoint'

type ApiKeyUsageDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  tokenKey: string
  apiKeyName?: string
}

type Protocol = 'responses' | 'chat' | 'messages' | 'gemini' | 'images'

export function ApiKeyUsageDialog(props: ApiKeyUsageDialogProps) {
  const { t } = useTranslation()
  const { status } = useStatus()
  const [protocol, setProtocol] = useState<Protocol>('responses')
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
  const apiOrigin = baseUrl.replace(/\/v1$/, '')

  let endpoint = `${apiOrigin}/v1/responses`
  let authentication = `Authorization: Bearer ${props.tokenKey}`
  let extraHeaders: string[] = []
  let requestBody: Record<string, unknown> = {
    model: 'gpt-5.6-luna',
    input: 'Hello',
  }

  if (protocol === 'chat') {
    endpoint = `${apiOrigin}/v1/chat/completions`
    requestBody = {
      model: 'gpt-5.6-luna',
      messages: [{ role: 'user', content: 'Hello' }],
    }
  } else if (protocol === 'messages') {
    endpoint = `${apiOrigin}/v1/messages`
    authentication = `x-api-key: ${props.tokenKey}`
    extraHeaders = ['anthropic-version: 2023-06-01']
    requestBody = {
      model: 'claude-sonnet-4-6',
      max_tokens: 1024,
      messages: [{ role: 'user', content: 'Hello' }],
    }
  } else if (protocol === 'gemini') {
    endpoint = `${apiOrigin}/v1beta/models/gemini-2.5-flash:generateContent`
    authentication = `x-goog-api-key: ${props.tokenKey}`
    requestBody = {
      contents: [{ role: 'user', parts: [{ text: 'Hello' }] }],
    }
  } else if (protocol === 'images') {
    endpoint = `${apiOrigin}/v1/images/generations`
    requestBody = {
      model: 'gpt-image-1',
      prompt: 'A friendly mole building an AI gateway',
      size: '1024x1024',
    }
  }

  const curlExample = [
    `curl '${endpoint}' \\`,
    `  -H '${authentication}' \\`,
    ...extraHeaders.map((header) => `  -H '${header}' \\`),
    `  -H 'Content-Type: application/json' \\`,
    `  -d '${JSON.stringify(requestBody)}'`,
  ].join('\n')
  const values = [
    { label: t('API Key'), value: props.tokenKey },
    { label: 'Base URL', value: baseUrl },
    { label: t('Authentication header'), value: authentication },
  ]

  return (
    <Dialog
      open={props.open}
      onOpenChange={props.onOpenChange}
      title={t('API key and endpoint')}
      description={
        props.apiKeyName
          ? t('Connection details and examples for {{name}}', {
              name: props.apiKeyName,
            })
          : t('Copy the connection details or start with a working example.')
      }
      contentClassName='sm:max-w-2xl'
      contentHeight='min(68vh, 42rem)'
      bodyClassName='flex flex-col gap-5'
      footer={
        <Button type='button' onClick={() => props.onOpenChange(false)}>
          {t('Close')}
        </Button>
      }
    >
      <Tabs
        value={protocol}
        onValueChange={(value) => setProtocol(value as Protocol)}
      >
        <TabsList
          aria-label={t('API format')}
          className='h-10 max-w-full justify-start overflow-x-auto p-1'
        >
          <TabsTrigger value='responses' className='px-3'>
            Responses
          </TabsTrigger>
          <TabsTrigger value='chat' className='px-3'>
            Chat
          </TabsTrigger>
          <TabsTrigger value='messages' className='px-3'>
            Messages
          </TabsTrigger>
          <TabsTrigger value='gemini' className='px-3'>
            Gemini
          </TabsTrigger>
          <TabsTrigger value='images' className='px-3'>
            Images
          </TabsTrigger>
        </TabsList>
      </Tabs>

      {endpointOptions.length > 1 && (
        <div className='flex min-w-0 items-center justify-between gap-3'>
          <div className='min-w-0'>
            <p className='text-sm font-medium'>{t('Route')}</p>
            {selectedOption?.description && (
              <p className='text-muted-foreground truncate text-xs'>
                {selectedOption.description}
              </p>
            )}
          </div>
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
              className='bg-background/80 max-w-56 min-w-36'
            >
              <SelectValue />
            </SelectTrigger>
            <SelectContent align='end' alignItemWithTrigger={false}>
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
        </div>
      )}

      <div className='flex flex-col gap-3'>
        {values.map((item) => (
          <div key={item.label} className='flex flex-col gap-1.5'>
            <p className='text-sm font-medium'>{item.label}</p>
            <div className='border-border/70 bg-muted/25 flex min-w-0 items-center gap-2 rounded-lg border px-3 py-2'>
              <code className='min-w-0 flex-1 overflow-x-auto font-mono text-xs whitespace-nowrap sm:text-sm'>
                {item.value}
              </code>
              <CopyButton
                value={item.value}
                size='sm'
                className='gap-1.5'
                tooltip={t('Copy {{label}}', { label: item.label })}
              >
                {t('Copy')}
              </CopyButton>
            </div>
          </div>
        ))}
      </div>

      <div>
        <p className='text-sm font-medium'>{t('cURL example')}</p>
        <CodeBlock
          code={curlExample}
          language='bash'
          title={t('cURL example')}
          showToolbar
          enableCollapse={false}
          maxExpandedLines={8}
          className='my-2'
        >
          <CodeBlockCopyButton />
        </CodeBlock>
      </div>

      <Alert className='bg-muted/20'>
        <ShieldCheck aria-hidden='true' />
        <AlertTitle>{t('Keep this key secret')}</AlertTitle>
        <AlertDescription>
          {t(
            'Anyone with this key can use its quota. Store it in an environment variable and never expose it in browser code.'
          )}
        </AlertDescription>
      </Alert>
    </Dialog>
  )
}
