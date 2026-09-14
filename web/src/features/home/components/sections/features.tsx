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
import { ArrowRight, HeartHandshake, ShieldCheck } from 'lucide-react'
import { useTranslation } from 'react-i18next'

export function Features() {
  const { t } = useTranslation()
  const commitments = [
    {
      icon: HeartHandshake,
      title: t('Four years, built on trust'),
      description: t(
        'For four years, we have focused on reliable AI access and lasting relationships with our users.'
      ),
    },
    {
      icon: ShieldCheck,
      title: t('ZDR from day one'),
      description: t(
        'We do not store your prompts or model replies. Necessary account, billing and usage records do not include conversation content.'
      ),
    },
    {
      icon: ArrowRight,
      title: t('The model you choose'),
      description: t(
        'Each model maps to its corresponding upstream model. We do not secretly substitute models or mix lower-quality responses.'
      ),
    },
  ]
  return (
    <section className='mx-auto max-w-6xl px-4 py-16 sm:px-6 lg:px-8'>
      <h2 className='mb-8 text-2xl font-semibold tracking-tight sm:text-3xl'>
        {t('Trust, built over four years.')}
      </h2>
      <div className='grid gap-6 md:grid-cols-3'>
        {commitments.map(({ icon: Icon, title, description }) => (
          <article key={title} className='bg-card rounded-xl border p-6'>
            <Icon className='text-primary mb-5 size-6' aria-hidden='true' />
            <h3 className='mb-3 text-lg font-semibold'>{title}</h3>
            <p className='text-muted-foreground text-sm leading-7'>
              {description}
            </p>
          </article>
        ))}
      </div>
      <p className='text-muted-foreground mt-8 flex flex-wrap items-center justify-center gap-3 text-sm'>
        <span>{t('Your request')}</span>
        <ArrowRight className='size-4' aria-hidden='true' />
        <span>{t('Your chosen model')}</span>
        <ArrowRight className='size-4' aria-hidden='true' />
        <span>{t('The original reply')}</span>
      </p>
    </section>
  )
}
