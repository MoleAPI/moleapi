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
import AzureAI from '@lobehub/icons/es/AzureAI/components/Color'
import Claude from '@lobehub/icons/es/Claude/components/Color'
import Cohere from '@lobehub/icons/es/Cohere/components/Color'
import DeepSeek from '@lobehub/icons/es/DeepSeek/components/Color'
import Gemini from '@lobehub/icons/es/Gemini/components/Color'
import Grok from '@lobehub/icons/es/Grok/components/Mono'
import Hunyuan from '@lobehub/icons/es/Hunyuan/components/Color'
import Midjourney from '@lobehub/icons/es/Midjourney/components/Mono'
import Minimax from '@lobehub/icons/es/Minimax/components/Color'
import Moonshot from '@lobehub/icons/es/Moonshot/components/Mono'
import OpenAI from '@lobehub/icons/es/OpenAI/components/Mono'
import Qingyan from '@lobehub/icons/es/Qingyan/components/Color'
import Qwen from '@lobehub/icons/es/Qwen/components/Color'
import Spark from '@lobehub/icons/es/Spark/components/Color'
import Suno from '@lobehub/icons/es/Suno/components/Mono'
import Volcengine from '@lobehub/icons/es/Volcengine/components/Color'
import Wenxin from '@lobehub/icons/es/Wenxin/components/Color'
import XAI from '@lobehub/icons/es/XAI/components/Mono'
import Xinference from '@lobehub/icons/es/Xinference/components/Color'
import Zhipu from '@lobehub/icons/es/Zhipu/components/Color'
import { useTranslation } from 'react-i18next'

const MODEL_PROVIDERS = [
  { name: 'Moonshot', icon: <Moonshot size={18} /> },
  { name: 'OpenAI', icon: <OpenAI size={18} /> },
  { name: 'xAI', icon: <XAI size={18} /> },
  { name: 'Zhipu', icon: <Zhipu size={18} /> },
  { name: 'Volcengine', icon: <Volcengine size={18} /> },
  { name: 'Cohere', icon: <Cohere size={18} /> },
  { name: 'Claude', icon: <Claude size={18} /> },
  { name: 'Gemini', icon: <Gemini size={18} /> },
  { name: 'Suno', icon: <Suno size={18} /> },
  { name: 'MiniMax', icon: <Minimax size={18} /> },
  { name: 'Wenxin', icon: <Wenxin size={18} /> },
  { name: 'Spark', icon: <Spark size={18} /> },
  { name: 'Qingyan', icon: <Qingyan size={18} /> },
  { name: 'DeepSeek', icon: <DeepSeek size={18} /> },
  { name: 'Qwen', icon: <Qwen size={18} /> },
  { name: 'Midjourney', icon: <Midjourney size={18} /> },
  { name: 'Grok', icon: <Grok size={18} /> },
  { name: 'Azure AI', icon: <AzureAI size={18} /> },
  { name: 'Hunyuan', icon: <Hunyuan size={18} /> },
  { name: 'Xinference', icon: <Xinference size={18} /> },
] as const

export function ModelProviders() {
  const { t } = useTranslation()

  return (
    <div className='border-border/40 mt-10 border-t pt-8 md:mt-12 md:pt-10'>
      <p className='text-muted-foreground/50 mb-5 text-center text-[10px] font-bold tracking-[0.15em] uppercase'>
        {t('Model Providers')}
      </p>
      <div className='flex flex-wrap items-center justify-center gap-2.5'>
        {MODEL_PROVIDERS.map((provider) => (
          <div
            key={provider.name}
            className='border-border/40 bg-background/60 text-foreground/70 flex items-center gap-2 rounded-full border px-3 py-1.5 text-xs font-medium'
          >
            <span aria-hidden='true' className='flex shrink-0'>
              {provider.icon}
            </span>
            <span>{provider.name}</span>
          </div>
        ))}
        <div className='border-border/40 bg-background/60 text-muted-foreground flex items-center rounded-full border px-3 py-1.5 text-xs font-semibold tabular-nums'>
          30+
        </div>
      </div>
    </div>
  )
}
