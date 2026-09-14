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
import { useTranslation } from 'react-i18next'

import { ModelProviders } from '../model-providers'

export function Stats(_props: { className?: string }) {
  const { t } = useTranslation()
  const commitments = [
    { value: '4', label: t('Years of continuous service') },
    { value: 'ZDR', label: t('Zero conversation retention') },
    { value: '1:1', label: t('One-to-one model forwarding') },
  ]
  return (
    <section
      aria-label={t('Our commitments')}
      className='border-border/40 bg-muted/10 relative z-10 border-y'
    >
      <div className='mx-auto max-w-6xl px-6 py-10 md:py-12'>
        <dl className='grid grid-cols-1 gap-8 sm:grid-cols-3 sm:gap-12'>
          {commitments.map((item) => (
            <div
              key={item.value}
              className='flex flex-col items-center text-center'
            >
              <dd className='text-3xl font-semibold tracking-tight tabular-nums md:text-4xl'>
                {item.value}
              </dd>
              <dt className='text-muted-foreground mt-2 text-sm'>
                {item.label}
              </dt>
            </div>
          ))}
        </dl>
        <ModelProviders />
      </div>
    </section>
  )
}
