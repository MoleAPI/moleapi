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
import CherryStudio from '@lobehub/icons/es/CherryStudio/components/Color'
import Codex from '@lobehub/icons/es/Codex/components/Color'
import Dify from '@lobehub/icons/es/Dify/components/Color'
import LobeHub from '@lobehub/icons/es/LobeHub/components/Color'

export const SUPPORTED_APPS = [
  {
    name: 'Cherry Studio',
    href: 'https://docs.moleapi.com/zh-CN/docs/apps/cherry-studio',
    icon: <CherryStudio size={22} />,
  },
  {
    name: 'CC Switch',
    href: 'https://docs.moleapi.com/zh-CN/docs/apps/cc-switch',
    icon: (
      <span className='bg-primary/10 text-primary flex size-[22px] items-center justify-center rounded-md text-[9px] font-bold'>
        CC
      </span>
    ),
  },
  {
    name: 'NextChat',
    href: 'https://docs.moleapi.com/zh-CN/docs/apps/nextchat',
    icon: (
      <span className='bg-primary/10 text-primary flex size-[22px] items-center justify-center rounded-md text-[9px] font-bold'>
        NC
      </span>
    ),
  },
  {
    name: 'Dify',
    href: 'https://docs.moleapi.com/zh-CN/docs/apps/dify',
    icon: <Dify size={22} />,
  },
  {
    name: 'LobeHub',
    href: 'https://docs.moleapi.com/zh-CN/docs/apps/lobechat',
    icon: <LobeHub size={22} />,
  },
  {
    name: 'AionUI',
    href: 'https://docs.moleapi.com/zh-CN/docs/apps/aionui',
    icon: (
      <span className='bg-primary/10 text-primary flex size-[22px] items-center justify-center rounded-md text-[9px] font-bold'>
        AI
      </span>
    ),
  },
  {
    name: 'Codex',
    href: 'https://docs.moleapi.com/zh-CN/docs/apps/codex-app',
    icon: <Codex size={22} />,
  },
] as const
