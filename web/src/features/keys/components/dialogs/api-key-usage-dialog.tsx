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
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { useStatus } from '@/hooks/use-status'

import { buildApiBaseUrl } from '../../lib/api-endpoint'

type ApiKeyUsageDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  tokenKey: string
  apiKeyName?: string
}

type Protocol = 'responses' | 'chat'

export function ApiKeyUsageDialog(props: ApiKeyUsageDialogProps) {
  const { t } = useTranslation()
  const { status } = useStatus()
  const [protocol, setProtocol] = useState<Protocol>('responses')
  const baseUrl = buildApiBaseUrl(
    typeof status?.server_address === 'string' ? status.server_address : '',
    typeof window === 'undefined' ? '' : window.location.origin
  )
  const authorization = `Authorization: Bearer ${props.tokenKey}`
  const endpoint =
    protocol === 'responses'
      ? `${baseUrl}/responses`
      : `${baseUrl}/chat/completions`
  const requestBody =
    protocol === 'responses'
      ? { model: 'gpt-5.6-luna', input: 'Hello' }
      : {
          model: 'gpt-5.6-luna',
          messages: [{ role: 'user', content: 'Hello' }],
        }
  const curlExample = [
    `curl '${endpoint}' \\`,
    `  -H '${authorization}' \\`,
    `  -H 'Content-Type: application/json' \\`,
    `  -d '${JSON.stringify(requestBody)}'`,
  ].join('\n')
  const values = [
    { label: t('API Key'), value: props.tokenKey },
    { label: t('SDK Base URL'), value: baseUrl },
    { label: t('Authentication header'), value: authorization },
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
      bodyClassName='space-y-5'
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
        <TabsList aria-label={t('API format')} className='h-10 p-1'>
          <TabsTrigger value='responses' className='px-4'>
            {t('Responses')}
          </TabsTrigger>
          <TabsTrigger value='chat' className='px-4'>
            {t('Chat')}
          </TabsTrigger>
        </TabsList>
      </Tabs>

      <div className='space-y-3'>
        {values.map((item) => (
          <div key={item.label} className='space-y-1.5'>
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
