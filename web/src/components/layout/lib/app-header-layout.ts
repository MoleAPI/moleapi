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
export const appHeaderLayoutClasses = {
  content: 'flex h-full items-center gap-1.5 px-2 pt-2 sm:gap-2 sm:px-3',
  brandLink:
    'text-foreground inline-flex h-9 items-center gap-2 rounded-md px-2 text-base font-medium transition-colors outline-none select-none',
  brandLogo:
    'flex size-7 items-center justify-center overflow-hidden rounded-md',
  brandName: 'max-w-[12rem] truncate text-base',
} as const
