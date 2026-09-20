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
import { useEffect, useRef } from 'react'
import { useTranslation } from 'react-i18next'

import { Checkbox } from '@/components/ui/checkbox'
import { Label } from '@/components/ui/label'
import { cn } from '@/lib/utils'

import type { SystemStatus } from '../types'

interface LegalConsentProps {
  status: SystemStatus | null
  checked: boolean
  onCheckedChange: (nextValue: boolean) => void
  className?: string
  error?: string
  attentionKey?: number
}

export function LegalConsent({
  status,
  checked,
  onCheckedChange,
  className,
  error,
  attentionKey = 0,
}: LegalConsentProps) {
  const { t } = useTranslation()
  const checkboxRef = useRef<HTMLButtonElement>(null)
  const hasUserAgreement = Boolean(status?.user_agreement_enabled)
  const hasPrivacyPolicy = Boolean(status?.privacy_policy_enabled)

  useEffect(() => {
    if (error) checkboxRef.current?.focus()
  }, [attentionKey, error])

  if (!hasUserAgreement && !hasPrivacyPolicy) {
    return null
  }

  const handleChange = (value: boolean) => {
    onCheckedChange(value === true)
  }

  return (
    <div className={className}>
      <div
        key={attentionKey}
        className={cn(
          'border-border/60 bg-muted/40 flex items-start gap-3 rounded-md border p-3',
          error &&
            'legal-consent-attention border-destructive/60 bg-destructive/5'
        )}
      >
        <Checkbox
          id='legal-consent'
          checked={checked}
          onCheckedChange={handleChange}
          className='mt-0.5'
          aria-invalid={Boolean(error)}
          aria-describedby={error ? 'legal-consent-error' : undefined}
          ref={checkboxRef}
        />
        <Label
          htmlFor='legal-consent'
          className='text-muted-foreground items-start gap-1 text-left text-xs leading-5 font-normal'
        >
          <span>
            {t('I have read and agree to the')}{' '}
            {hasUserAgreement && (
              <a
                href='/user-agreement'
                target='_blank'
                rel='noopener noreferrer'
                className='text-primary hover:underline'
              >
                {t('User Agreement')}
              </a>
            )}
            {hasUserAgreement && hasPrivacyPolicy ? <> {t('and')} </> : null}
            {hasPrivacyPolicy && (
              <a
                href='/privacy-policy'
                target='_blank'
                rel='noopener noreferrer'
                className='text-primary hover:underline'
              >
                {t('Privacy Policy')}
              </a>
            )}
            .
          </span>
        </Label>
      </div>
      {error && (
        <p
          id='legal-consent-error'
          className='text-destructive mt-1 text-xs'
          role='alert'
        >
          {error}
        </p>
      )}
    </div>
  )
}
