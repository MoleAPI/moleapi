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
    'min-h-11 w-full min-w-0 justify-center gap-1.5 rounded-lg px-3 py-2 text-center text-sm font-semibold shadow-sm hover:shadow-md [&>svg]:!text-white',
  bonus:
    'bg-success/10 text-success rounded-md px-1.5 py-0.5 text-xs font-semibold',
} as const
