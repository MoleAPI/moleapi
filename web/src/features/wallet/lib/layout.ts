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
export const walletLayoutClasses = {
  grid: 'grid gap-4 lg:grid-cols-12 lg:items-start',
  recharge: 'scroll-mt-4 lg:col-span-7',
  referral: 'lg:col-span-5',
} as const

export const rechargeFormLayoutClasses = {
  paymentMethods: 'grid max-w-md grid-cols-2 gap-1.5 sm:gap-2',
  paymentButton:
    'h-auto min-h-14 w-full min-w-0 justify-start gap-3 rounded-lg border-border/70 bg-card px-4 py-2 text-left text-sm font-semibold text-card-foreground shadow-sm transition-colors hover:border-foreground/20 hover:bg-muted/60 dark:border-white/10 dark:bg-[#2d3035] dark:text-white dark:hover:bg-[#333841]',
  paymentIconBadge:
    'flex size-8 shrink-0 items-center justify-center rounded-[10px] shadow-sm ring-1 ring-white/20 [&>svg]:size-4',
  paymentLabel: 'flex min-w-0 flex-col items-start gap-0.5 text-left',
  bonus:
    'bg-success/10 text-success rounded-md px-1.5 py-0.5 text-xs font-semibold',
} as const
