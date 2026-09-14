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
import { ArrowRight } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'

// ponytail: editorial examples verified against production on 2026-09-14.
// Recheck when offers change; the wallet and pricing pages remain authoritative.
const TOP_UP_EXAMPLES = [
  { amount: 1, bonus: 5, credit: '1.05' },
  { amount: 15, bonus: 18, credit: '17.70' },
  { amount: 70, bonus: 26, credit: '88.20' },
  { amount: 280, bonus: 40, credit: '392.00' },
]

export function Offers(props: { isAuthenticated: boolean }) {
  const { t } = useTranslation()

  return (
    <section
      id='offers'
      aria-labelledby='offers-title'
      className='mx-auto max-w-6xl scroll-mt-24 px-6 py-16 md:py-24'
    >
      <div className='mb-10 max-w-2xl'>
        <p className='text-primary mb-3 text-sm font-semibold'>
          {t('Priced in USD. More value with every top-up.')}
        </p>
        <h2
          id='offers-title'
          className='text-3xl leading-tight font-semibold tracking-tight sm:text-4xl'
        >
          {t('Keep the model. Lower the cost.')}
        </h2>
      </div>

      <div className='grid gap-10 lg:grid-cols-[0.85fr_1.15fr] lg:gap-16'>
        <div className='bg-muted/40 flex flex-col items-start rounded-2xl p-6 sm:p-8'>
          <p className='text-muted-foreground text-sm'>
            {t('Selected models in the temp group')}
          </p>
          <h3 className='mt-4 text-4xl leading-tight font-semibold tracking-tight sm:text-5xl'>
            {t('From 10% of standard pricing')}
          </h3>
          <p className='mt-5 text-xl font-medium'>DeepSeek · GPT</p>
          <p className='text-muted-foreground mt-3 text-sm leading-7'>
            {t(
              'The temp group bills eligible models at 0.1 times their MoleAPI standard-group price. Model availability can change; check the current list before use.'
            )}
          </p>
          <Button
            role='link'
            className='mt-6 h-auto min-h-10 py-2 whitespace-normal'
            render={<Link to='/pricing' search={{ group: 'temp' }} />}
          >
            {t('Explore temp-group models')}
            <ArrowRight aria-hidden='true' />
          </Button>
        </div>

        <div className='min-w-0'>
          <h3 className='text-2xl font-semibold tracking-tight'>
            {t('Top up from 1 USD. Receive up to 40% extra.')}
          </h3>
          <p className='text-muted-foreground mt-3 text-sm leading-7'>
            {t(
              'Your top-up and bonus combine into one usage balance, denominated in USD.'
            )}
          </p>
          <table className='mt-6 w-full text-left text-sm tabular-nums'>
            <caption className='sr-only'>{t('Top-up examples in USD')}</caption>
            <thead className='text-muted-foreground border-b'>
              <tr>
                <th scope='col' className='py-3 pr-2 font-medium'>
                  {t('Top-up (USD)')}
                </th>
                <th scope='col' className='px-2 py-3 font-medium'>
                  {t('Bonus')}
                </th>
                <th scope='col' className='py-3 pl-2 text-right font-medium'>
                  {t('Receive (USD)')}
                </th>
              </tr>
            </thead>
            <tbody>
              {TOP_UP_EXAMPLES.map((example) => (
                <tr
                  key={example.amount}
                  className='border-b last:font-semibold'
                >
                  <td className='py-4 pr-2'>{example.amount}</td>
                  <td className='px-2 py-4 text-emerald-700 dark:text-emerald-400'>
                    +{example.bonus}%
                  </td>
                  <td className='py-4 pl-2 text-right'>{example.credit}</td>
                </tr>
              ))}
            </tbody>
          </table>
          <p className='text-muted-foreground mt-4 text-xs leading-6'>
            {t(
              'Current top-up examples. Credits are for API usage; payment conversion and final terms are shown at checkout.'
            )}
          </p>
          <Button
            role='link'
            variant='outline'
            className='mt-4 h-auto min-h-10 py-2 whitespace-normal'
            render={
              <Link to={props.isAuthenticated ? '/wallet' : '/sign-up'} />
            }
          >
            {props.isAuthenticated
              ? t('View all top-up offers')
              : t('Create an account to top up')}
            <ArrowRight aria-hidden='true' />
          </Button>
        </div>
      </div>

      <div className='mt-14 border-t pt-10'>
        <div className='grid gap-8 md:grid-cols-[1fr_1.2fr] md:gap-12'>
          <div>
            <h3 className='text-xl font-semibold'>
              {t('A comparison you can check')}
            </h3>
            <p className='text-muted-foreground mt-3 text-sm leading-7'>
              {t(
                'Claude Sonnet 4.6: 1M input tokens plus 1M output tokens, accumulated across standard text requests.'
              )}
            </p>
            <p className='mt-4 text-sm leading-7'>
              {t(
                'With a 280 USD top-up and 40% bonus, standard-group usage costs about 28.6% less than the official API, or 32.3% less than OpenRouter including its listed fee.'
              )}
            </p>
          </div>
          <dl className='divide-y text-sm'>
            <div className='flex flex-wrap items-baseline justify-between gap-2 py-4'>
              <dt>
                <a
                  className='underline underline-offset-4'
                  href='https://platform.claude.com/docs/en/about-claude/pricing'
                  target='_blank'
                  rel='noopener noreferrer'
                >
                  {t('Official Anthropic API')}
                </a>
              </dt>
              <dd className='text-lg font-medium tabular-nums'>18.00 USD</dd>
            </div>
            <div className='flex flex-wrap items-baseline justify-between gap-2 py-4'>
              <dt>
                <a
                  className='underline underline-offset-4'
                  href='https://openrouter.ai/pricing'
                  target='_blank'
                  rel='noopener noreferrer'
                >
                  {t('OpenRouter + 5.5% fee')}
                </a>
              </dt>
              <dd className='text-lg font-medium tabular-nums'>18.99 USD</dd>
            </div>
            <div className='flex flex-wrap items-baseline justify-between gap-2 py-4'>
              <dt>{t('MoleAPI after the 40% bonus')}</dt>
              <dd className='text-primary text-2xl font-semibold tabular-nums'>
                ≈ 12.86 USD
              </dd>
            </div>
          </dl>
        </div>
        <p className='text-muted-foreground mt-6 text-xs leading-6'>
          {t(
            'This example uses the standard group, not temp. Effective cost assumes all purchased and bonus credits are used: 18 / 1.4 ≈ 12.86 USD; the balance deduction is still 18 USD. Excludes caching, tools, batch discounts, taxes and currency conversion. Prices checked on September 14, 2026.'
          )}
        </p>
        <Link
          className='mt-4 inline-block text-sm underline underline-offset-4'
          to='/pricing'
          search={{ search: 'claude-sonnet-4-6', group: 'default' }}
        >
          {t('Check current model pricing')}
        </Link>
      </div>
    </section>
  )
}
