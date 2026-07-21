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
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'

import { PublicLayout } from '@/components/layout'
import { RichContent } from '@/components/rich-content'
import { Skeleton } from '@/components/ui/skeleton'
import { isHttpUrl, isLikelyHtml } from '@/lib/content-format'

import { getAboutContent } from './api'

function ServiceComplianceNotice() {
  const { t } = useTranslation()

  return (
    <section
      aria-labelledby='service-compliance-title'
      className='border-border bg-card text-card-foreground mx-auto flex max-w-4xl flex-col gap-4 rounded-xl border p-6 text-center shadow-sm'
    >
      <h2 id='service-compliance-title' className='text-xl font-semibold'>
        {t('Service Compliance Notice')}
      </h2>
      <p>{t('MoleAPI is operated outside China and the United States.')}</p>
      <p>
        {t(
          'MoleAPI does not provide services in China, the United States, or any country or region where an applicable upstream provider, including OpenAI or Anthropic, has not made the relevant service available.'
        )}
      </p>
      <p>
        {t(
          'Before using the service, you must confirm that your location and use comply with applicable laws and the policies of relevant service providers. If you do not meet these requirements, stop using the service immediately.'
        )}
      </p>
      <p>
        {t(
          'By continuing to access or use this site, you confirm that you have read, understood, and agreed to this notice and the User Agreement.'
        )}
      </p>
      <p>
        {t(
          'To the extent permitted by applicable law, MoleAPI reserves the right to interpret and update this notice.'
        )}
      </p>
    </section>
  )
}

export function EmptyAboutState() {
  const { t } = useTranslation()
  const currentYear = new Date().getFullYear()

  return (
    <div className='flex min-h-[60vh] items-center justify-center p-8'>
      <div className='flex max-w-4xl flex-col gap-6 text-center'>
        <ServiceComplianceNotice />
        <div className='flex flex-col gap-4 text-sm'>
          <p>
            {t('New API Project Repository:')}{' '}
            <a
              href='https://github.com/QuantumNous/new-api'
              target='_blank'
              rel='noopener noreferrer'
              className='text-primary hover:underline'
            >
              {t('https://github.com/QuantumNous/new-api')}
            </a>
          </p>
          <p className='text-muted-foreground'>
            <a
              href='https://github.com/QuantumNous/new-api'
              target='_blank'
              rel='noopener noreferrer'
              className='text-primary hover:underline'
            >
              {t('NewAPI')}
            </a>{' '}
            © {currentYear}{' '}
            <a
              href='https://github.com/QuantumNous'
              target='_blank'
              rel='noopener noreferrer'
              className='text-primary hover:underline'
            >
              {t('QuantumNous')}
            </a>{' '}
            {t('| Based on')}{' '}
            <a
              href='https://github.com/songquanpeng/one-api'
              target='_blank'
              rel='noopener noreferrer'
              className='text-primary hover:underline'
            >
              {t('One API')}
            </a>{' '}
            © 2023{' '}
            <a
              href='https://github.com/songquanpeng'
              target='_blank'
              rel='noopener noreferrer'
              className='text-primary hover:underline'
            >
              {t('JustSong')}
            </a>
          </p>
          <p className='text-muted-foreground'>
            {t('This project must be used in compliance with the')}{' '}
            <a
              href='https://github.com/QuantumNous/new-api/blob/main/LICENSE'
              target='_blank'
              rel='noopener noreferrer'
              className='text-primary hover:underline'
            >
              {t('AGPL v3.0 License')}
            </a>
            .
          </p>
        </div>
      </div>
    </div>
  )
}

export function About() {
  const { t } = useTranslation()
  const { data, isLoading } = useQuery({
    queryKey: ['about-content'],
    queryFn: getAboutContent,
  })

  const rawContent = data?.data?.trim() ?? ''
  const hasContent = rawContent.length > 0
  const isUrl = hasContent && isHttpUrl(rawContent)
  const contentIsHtml = hasContent && isLikelyHtml(rawContent)

  if (isLoading) {
    return (
      <PublicLayout>
        <div className='mx-auto flex max-w-4xl flex-col gap-4 py-12'>
          <Skeleton className='h-8 w-[45%]' />
          <Skeleton className='h-4 w-full' />
          <Skeleton className='h-4 w-[90%]' />
          <Skeleton className='h-4 w-[80%]' />
        </div>
      </PublicLayout>
    )
  }

  let aboutContent

  if (!hasContent) {
    aboutContent = <EmptyAboutState />
  } else if (isUrl) {
    aboutContent = (
      <iframe
        src={rawContent}
        className='h-[calc(100vh-3.5rem)] w-full border-0'
        title={t('About')}
        sandbox='allow-forms allow-popups allow-popups-to-escape-sandbox allow-scripts'
      />
    )
  } else if (contentIsHtml) {
    aboutContent = (
      <RichContent
        mode='html'
        htmlVariant='isolated'
        content={rawContent}
        className='prose-neutral dark:prose-invert max-w-none'
      />
    )
  } else {
    aboutContent = (
      <div className='mx-auto max-w-6xl px-4 py-8'>
        <RichContent
          mode='markdown'
          content={rawContent}
          className='prose-neutral dark:prose-invert max-w-none'
        />
      </div>
    )
  }

  return (
    <PublicLayout showMainContainer={!isUrl && !contentIsHtml}>
      {aboutContent}
      {hasContent ? (
        <div className='px-4 pb-12'>
          <ServiceComplianceNotice />
        </div>
      ) : null}
    </PublicLayout>
  )
}
