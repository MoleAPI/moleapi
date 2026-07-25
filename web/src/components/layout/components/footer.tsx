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
import { Link } from '@tanstack/react-router'
import { Fragment, useMemo } from 'react'
import { useTranslation } from 'react-i18next'

import { useStatus } from '@/hooks/use-status'
import { useSystemConfig } from '@/hooks/use-system-config'
import { cn } from '@/lib/utils'

import { BrandName } from './brand-name'

interface FooterLink {
  text: string
  href: string
}

interface FooterColumnProps {
  title: string
  links: FooterLink[]
}

interface FooterProps {
  logo?: string
  name?: string
  columns?: FooterColumnProps[]
  copyright?: string
  className?: string
}

const NEW_API_FOOTER_ATTRIBUTION_KEY = [
  'footer',
  'new' + 'api',
  'projectAttributionSuffix',
].join('.')
const SUPPORT_EMAIL = 'support@moleapi.com'

function formatFooterVersion(value: unknown, stripSuffix = false) {
  if (typeof value !== 'string' || value.trim() === '') {
    return ''
  }
  const normalized = value.trim().replace(/^v/i, '')
  const display = stripSuffix ? normalized.split('-')[0] : normalized
  return display ? `v${display}` : ''
}

function FooterLinkItem(props: { link: FooterLink }) {
  const { t } = useTranslation()
  const isExternal = props.link.href.startsWith('http')
  const label = t(props.link.text)

  if (isExternal) {
    return (
      <a
        href={props.link.href}
        target='_blank'
        rel='noopener noreferrer'
        className='text-muted-foreground hover:text-foreground text-sm transition-colors duration-200'
      >
        {label}
      </a>
    )
  }

  return (
    <Link
      to={props.link.href}
      className='text-muted-foreground hover:text-foreground text-sm transition-colors duration-200'
    >
      {label}
    </Link>
  )
}

// Renders User Agreement / Privacy Policy links inline with the parent's
// copyright row when either is configured in System Settings → Site. Emits
// fragmented siblings so the parent flex container's gap controls spacing.
function LegalLinks(props: { leadingSeparator?: boolean }) {
  const { t } = useTranslation()
  const { status } = useStatus()
  const items: { key: string; label: string; href: string }[] = []
  if (status?.user_agreement_enabled) {
    items.push({
      key: 'user-agreement',
      label: t('User Agreement'),
      href: '/user-agreement',
    })
  }
  if (status?.privacy_policy_enabled) {
    items.push({
      key: 'privacy-policy',
      label: t('Privacy Policy'),
      href: '/privacy-policy',
    })
  }
  if (items.length === 0) {
    return null
  }
  return (
    <>
      {items.map((item, index) => (
        <Fragment key={item.key}>
          {(props.leadingSeparator || index > 0) && (
            <span aria-hidden='true' className='text-muted-foreground/30'>
              ·
            </span>
          )}
          <Link
            to={item.href}
            className='hover:text-foreground transition-colors duration-200'
          >
            {item.label}
          </Link>
        </Fragment>
      ))}
    </>
  )
}

function SupportEmailLink(props: { leadingSeparator?: boolean }) {
  return (
    <>
      {props.leadingSeparator && (
        <span aria-hidden='true' className='text-muted-foreground/30'>
          ·
        </span>
      )}
      <a
        href={`mailto:${SUPPORT_EMAIL}`}
        className='hover:text-foreground transition-colors duration-200'
      >
        {SUPPORT_EMAIL}
      </a>
    </>
  )
}

// inline=true returns just the inner span for composition in a parent flex
// row. inline=false wraps in a centered/right-aligned div (default).
function ProjectAttribution(props: { currentYear: number; inline?: boolean }) {
  const { t } = useTranslation()
  const content = (
    <span className='text-muted-foreground/45'>
      &copy; {props.currentYear}{' '}
      <a
        href='https://github.com/QuantumNous/new-api'
        target='_blank'
        rel='noopener noreferrer'
        className='text-foreground/70 hover:text-foreground font-medium transition-colors'
      >
        {t('New API')}
      </a>
      . {t(NEW_API_FOOTER_ATTRIBUTION_KEY)}
    </span>
  )
  if (props.inline) {
    return content
  }
  return (
    <div className='text-muted-foreground/45 text-center text-xs sm:text-right'>
      {content}
    </div>
  )
}

export function Footer(props: FooterProps) {
  const { t } = useTranslation()
  const { status } = useStatus()
  const { systemName, footerHtml, demoSiteEnabled } = useSystemConfig()

  const displayName = systemName || props.name || 'New API'
  const isDemoSiteMode = Boolean(demoSiteEnabled)
  const currentYear = new Date().getFullYear()
  const projectVersion = formatFooterVersion(status?.project_version, true)
  const upstreamVersion = formatFooterVersion(
    status?.upstream_version || status?.version
  )

  const fallbackColumns = useMemo<FooterColumnProps[]>(
    () => [
      {
        title: t('footer.columns.about.title'),
        links: [
          {
            text: t('footer.columns.about.links.aboutProject'),
            href: 'https://docs.newapi.pro/wiki/project-introduction/',
          },
          {
            text: t('footer.columns.about.links.contact'),
            href: 'https://docs.newapi.pro/support/community-interaction/',
          },
          {
            text: t('footer.columns.about.links.features'),
            href: 'https://docs.newapi.pro/wiki/features-introduction/',
          },
        ],
      },
      {
        title: t('footer.columns.docs.title'),
        links: [
          {
            text: t('footer.columns.docs.links.quickStart'),
            href: 'https://docs.newapi.pro/getting-started/',
          },
          {
            text: t('footer.columns.docs.links.installation'),
            href: 'https://docs.newapi.pro/installation/',
          },
          {
            text: t('footer.columns.docs.links.apiDocs'),
            href: 'https://docs.newapi.pro/api/',
          },
        ],
      },
      {
        title: t('footer.columns.related.title'),
        links: [
          {
            text: t('footer.columns.related.links.oneApi'),
            href: 'https://github.com/songquanpeng/one-api',
          },
          {
            text: t('footer.columns.related.links.midjourney'),
            href: 'https://github.com/novicezk/midjourney-proxy',
          },
          {
            text: t('footer.columns.related.links.newApiKeyTool'),
            href: 'https://github.com/Calcium-Ion/new-api-key-tool',
          },
        ],
      },
    ],
    [t]
  )

  const displayColumns = props.columns ?? fallbackColumns

  if (footerHtml) {
    return (
      <footer
        className={cn(
          'border-border/40 relative z-10 border-t',
          props.className
        )}
      >
        <div className='mx-auto w-full max-w-6xl px-6 py-5'>
          <div className='bg-muted/20 border-border/50 flex flex-col items-center justify-between gap-4 rounded-2xl border px-4 py-4 backdrop-blur-sm sm:flex-row sm:px-5'>
            <div
              className='custom-footer text-muted-foreground min-w-0 text-center text-sm sm:text-left'
              dangerouslySetInnerHTML={{ __html: footerHtml }}
            />
            <div className='border-border/60 text-muted-foreground/45 flex w-full flex-wrap items-center justify-center gap-x-3 gap-y-1 border-t pt-4 text-xs sm:w-auto sm:justify-end sm:border-t-0 sm:border-l sm:pt-0 sm:pl-5'>
              <LegalLinks />
              <SupportEmailLink />
              <ProjectAttribution currentYear={currentYear} inline />
            </div>
          </div>
        </div>
      </footer>
    )
  }

  return (
    <footer
      className={cn('border-border/40 relative z-10 border-t', props.className)}
    >
      <div className='mx-auto max-w-6xl px-6 py-12 md:py-16'>
        {isDemoSiteMode && (
          <div className='grid grid-cols-3 gap-8 md:gap-16'>
            {displayColumns.map((column) => (
              <div key={column.title}>
                <p className='text-muted-foreground/50 mb-3 text-xs font-medium tracking-wider uppercase'>
                  {t(column.title)}
                </p>
                <ul className='space-y-2.5'>
                  {column.links.map((link) => (
                    <li key={`${link.text}-${link.href}`}>
                      <FooterLinkItem link={link} />
                    </li>
                  ))}
                </ul>
              </div>
            ))}
          </div>
        )}

        <div
          className={cn(
            'text-muted-foreground/55 flex flex-wrap items-center justify-center gap-x-1.5 gap-y-1 text-center text-xs',
            isDemoSiteMode && 'border-border/30 mt-12 border-t pt-6'
          )}
        >
          <span>&copy; {currentYear}</span>
          <BrandName name={displayName} className='text-xs' />
          {projectVersion && <span>{projectVersion}</span>}
          <span>{t('Designed and Developed by')}</span>
          <a
            href='https://github.com/ClarenceDan'
            target='_blank'
            rel='noopener noreferrer'
            className='text-foreground/70 hover:text-foreground font-medium transition-colors'
          >
            ClarenceDan
          </a>
          <span>{t('| Based on')}</span>
          <a
            href='https://github.com/QuantumNous/new-api'
            target='_blank'
            rel='noopener noreferrer'
            className='text-foreground/70 hover:text-foreground font-medium transition-colors'
          >
            New API {upstreamVersion}
          </a>
          <span>{t('by')}</span>
          <a
            href='https://github.com/QuantumNous'
            target='_blank'
            rel='noopener noreferrer'
            className='text-foreground/70 hover:text-foreground font-medium transition-colors'
          >
            QuantumNous
          </a>
          <span>·</span>
          <a
            href='https://github.com/songquanpeng/one-api'
            target='_blank'
            rel='noopener noreferrer'
            className='text-foreground/70 hover:text-foreground font-medium transition-colors'
          >
            One API
          </a>
          <span>{t('by')}</span>
          <a
            href='https://github.com/songquanpeng'
            target='_blank'
            rel='noopener noreferrer'
            className='text-foreground/70 hover:text-foreground font-medium transition-colors'
          >
            JustSong
          </a>
          {props.copyright && (
            <>
              <span aria-hidden='true'>·</span>
              <span>{props.copyright}</span>
            </>
          )}
          <LegalLinks leadingSeparator />
          <SupportEmailLink leadingSeparator />
        </div>
      </div>
    </footer>
  )
}
