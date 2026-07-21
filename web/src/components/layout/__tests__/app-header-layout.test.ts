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

import { appHeaderLayoutClasses } from '../lib/app-header-layout'

test('console header sits lower and uses an enlarged brand', () => {
  assert.ok(appHeaderLayoutClasses.content.includes('pt-2'))
  assert.ok(appHeaderLayoutClasses.brandLink.includes('h-9'))
  assert.ok(appHeaderLayoutClasses.brandLogo.includes('size-7'))
  assert.ok(appHeaderLayoutClasses.brandName.includes('text-base'))
})
