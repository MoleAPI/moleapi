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
import assert from 'node:assert/strict'
import { test } from 'node:test'

import { rechargeFormLayoutClasses, walletLayoutClasses } from '../lib/layout'

test('wallet stacks on small screens and uses the former 7/5 desktop split', () => {
  assert.ok(walletLayoutClasses.grid.includes('lg:grid-cols-12'))
  assert.ok(walletLayoutClasses.recharge.includes('lg:col-span-7'))
  assert.ok(walletLayoutClasses.referral.includes('lg:col-span-5'))
})

test('payment choices use two neutral buttons per row with app-style icon badges', () => {
  assert.ok(rechargeFormLayoutClasses.paymentMethods.includes('grid-cols-2'))
  assert.ok(rechargeFormLayoutClasses.paymentMethods.includes('max-w-md'))
  assert.ok(rechargeFormLayoutClasses.paymentButton.includes('justify-center'))
  assert.ok(rechargeFormLayoutClasses.paymentButton.includes('text-sm'))
  assert.ok(rechargeFormLayoutClasses.paymentButton.includes('min-h-14'))
  assert.ok(
    rechargeFormLayoutClasses.paymentButton.includes('dark:bg-[#2d3035]')
  )
  assert.ok(rechargeFormLayoutClasses.paymentIconBadge.includes('size-8'))
  assert.ok(
    rechargeFormLayoutClasses.paymentIconBadge.includes('rounded-[10px]')
  )
  assert.ok(rechargeFormLayoutClasses.bonus.includes('text-success'))
  assert.ok(rechargeFormLayoutClasses.bonus.includes('bg-success/10'))
})
