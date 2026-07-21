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
import { useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { CopyButton } from '@/components/copy-button'
import { Badge } from '@/components/ui/badge'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { cn } from '@/lib/utils'

import {
  API_DEMOS,
  PUBLIC_API_BASE_URL,
  type ApiDemoConfig,
} from './hero-terminal-demo-data'

const LANGUAGES = [
  { id: 'curl', label: 'cURL' },
  { id: 'python', label: 'Python' },
  { id: 'node', label: 'Node.js' },
] as const

type Language = (typeof LANGUAGES)[number]['id']

const CYCLE_INTERVAL = 6000

interface HeroTerminalDemoProps {
  className?: string
}

export function HeroTerminalDemo(props: HeroTerminalDemoProps) {
  const { t } = useTranslation()
  const [activeIndex, setActiveIndex] = useState(0)
  const [language, setLanguage] = useState<Language>('curl')
  const intervalRef = useRef<ReturnType<typeof setInterval>>(undefined)

  useEffect(() => {
    if (window.matchMedia('(prefers-reduced-motion: reduce)').matches) return

    intervalRef.current = setInterval(() => {
      setActiveIndex((index) => (index + 1) % API_DEMOS.length)
    }, CYCLE_INTERVAL)

    return () => {
      if (intervalRef.current) clearInterval(intervalRef.current)
    }
  }, [])

  const selectEndpoint = (index: number) => {
    if (intervalRef.current) clearInterval(intervalRef.current)
    setActiveIndex(index)
  }

  const demo = API_DEMOS[activeIndex]

  return (
    <div className={cn('mx-auto w-full max-w-2xl', props.className)}>
      <div className='bg-card overflow-hidden rounded-xl border shadow-sm'>
        <div className='border-border/60 flex items-center gap-3 border-b px-4 py-3 sm:px-5'>
          <span className='text-muted-foreground shrink-0 text-[10px] font-semibold tracking-[0.14em] uppercase'>
            {t('Base URL')}
          </span>
          <code className='text-foreground/80 min-w-0 flex-1 truncate font-mono text-xs sm:text-sm'>
            {PUBLIC_API_BASE_URL}
          </code>
          <CopyButton
            value={PUBLIC_API_BASE_URL}
            size='icon'
            className='size-8'
            tooltip={t('Copy to clipboard')}
          />
        </div>

        <div className='border-border/60 flex min-w-0 items-center gap-2 border-b px-4 py-3 sm:px-5'>
          <Badge variant='secondary' className='shrink-0 font-mono text-[10px]'>
            {demo.method}
          </Badge>
          <code className='text-foreground min-w-0 truncate font-mono text-sm font-medium'>
            {demo.endpoint}
          </code>
        </div>

        <Tabs
          value={language}
          onValueChange={(value) => setLanguage(value as Language)}
          className='gap-0'
        >
          <div className='flex items-center justify-between gap-3 px-4 pt-3 sm:px-5'>
            <span className='text-muted-foreground text-[10px] font-semibold tracking-[0.14em] uppercase'>
              {t('Request')}
            </span>
            <TabsList variant='line' className='h-7'>
              {LANGUAGES.map((item) => (
                <TabsTrigger key={item.id} value={item.id}>
                  {item.label}
                </TabsTrigger>
              ))}
            </TabsList>
          </div>

          <pre className='text-foreground/75 h-[280px] overflow-auto px-4 py-4 font-mono text-[11px] leading-[1.65] sm:px-5 sm:text-xs'>
            <code>{getRequestExample(demo, language)}</code>
          </pre>
        </Tabs>

        <div
          role='tablist'
          aria-label={t('Endpoint')}
          className='border-border/60 bg-muted/20 flex flex-wrap gap-1 border-t p-2.5'
        >
          {API_DEMOS.map((item, index) => {
            const isActive = index === activeIndex
            return (
              <button
                key={item.id}
                type='button'
                role='tab'
                aria-selected={isActive}
                onClick={() => selectEndpoint(index)}
                className={cn(
                  'focus-visible:ring-ring rounded-md px-2.5 py-1.5 text-xs font-medium transition-colors focus-visible:ring-2 focus-visible:outline-none',
                  isActive
                    ? 'bg-background text-foreground shadow-xs'
                    : 'text-muted-foreground hover:bg-background/70 hover:text-foreground'
                )}
              >
                {item.label}
              </button>
            )
          })}
        </div>
      </div>
    </div>
  )
}

function getRequestExample(demo: ApiDemoConfig, language: Language): string {
  const url = `${PUBLIC_API_BASE_URL}${demo.endpoint}`
  const headers = JSON.stringify(demo.headers, null, 2)
  const payload = JSON.stringify(demo.payload, null, 2)

  if (language === 'python') {
    return `import requests

response = requests.post(
    "${url}",
    headers=${indent(headers, 4)},
    json=${indent(payload, 4)},
)

print(response.json())`
  }

  if (language === 'node') {
    return `const response = await fetch("${url}", {
  method: "${demo.method}",
  headers: ${indent(headers, 2)},
  body: JSON.stringify(${indent(payload, 2)}),
})

console.log(await response.json())`
  }

  const headerLines = Object.entries(demo.headers)
    .map(([name, value]) => `  -H "${name}: ${value}" \\`)
    .join('\n')

  return `curl -X ${demo.method} "${url}" \\
${headerLines}
  -H "Content-Type: application/json" \\
  -d '${payload}'`
}

function indent(value: string, spaces: number): string {
  return value.replaceAll('\n', `\n${' '.repeat(spaces)}`)
}
