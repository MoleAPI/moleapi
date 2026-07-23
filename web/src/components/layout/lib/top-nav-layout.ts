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
export const topNavLayoutClasses = {
  mobileMenu: 'xl:hidden',
  desktopNav: 'hidden flex-nowrap items-center gap-1 xl:flex xl:gap-1.5',
  link: 'hover:bg-accent/70 hover:text-primary focus-visible:bg-accent/70 shrink-0 rounded-lg px-2.5 py-1.5 text-sm font-medium whitespace-nowrap motion-safe:transition-[color,background-color,transform] motion-safe:duration-200 motion-safe:ease-[cubic-bezier(0.25,1,0.5,1)] motion-safe:hover:-translate-y-px xl:px-3',
} as const
