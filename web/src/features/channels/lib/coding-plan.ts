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
import type { AdvancedCustomConfig, AdvancedCustomRoute } from '../types'

export const CHANNEL_TYPE_CODING_PLAN = 59

export type CodingPlanProvider = {
  value: string
  label: string
  openAIBaseUrl: string
  anthropicBaseUrl: string
  responsesBaseUrl?: string
  modelListBaseUrl?: string
}

export const CODING_PLAN_PROVIDER_OPTIONS: CodingPlanProvider[] = [
  {
    value: 'glm-coding-plan',
    label: 'GLM Coding Plan',
    openAIBaseUrl: 'https://open.bigmodel.cn/api/coding/paas/v4',
    anthropicBaseUrl: 'https://open.bigmodel.cn/api/anthropic',
    modelListBaseUrl: 'https://open.bigmodel.cn/api/coding/paas/v4',
  },
  {
    value: 'glm-coding-plan-international',
    label: 'Z.ai Coding Plan',
    openAIBaseUrl: 'https://api.z.ai/api/coding/paas/v4',
    anthropicBaseUrl: 'https://api.z.ai/api/anthropic',
    modelListBaseUrl: 'https://api.z.ai/api/coding/paas/v4',
  },
  {
    value: 'kimi-coding-plan',
    label: 'Kimi Code',
    openAIBaseUrl: 'https://api.kimi.com/coding/v1',
    anthropicBaseUrl: 'https://api.kimi.com/coding',
    responsesBaseUrl: 'https://api.kimi.com/coding/v1',
    modelListBaseUrl: 'https://api.kimi.com/coding/v1',
  },
  {
    value: 'doubao-coding-plan',
    label: 'Doubao Coding Plan',
    openAIBaseUrl: 'https://ark.cn-beijing.volces.com/api/coding/v3',
    anthropicBaseUrl: 'https://ark.cn-beijing.volces.com/api/coding',
    responsesBaseUrl: 'https://ark.cn-beijing.volces.com/api/coding/v3',
    modelListBaseUrl: 'https://ark.cn-beijing.volces.com/api/coding/v3',
  },
  {
    value: 'qwen-coding-plan',
    label: 'Qwen Coding Plan',
    openAIBaseUrl: 'https://coding-intl.dashscope.aliyuncs.com/v1',
    anthropicBaseUrl:
      'https://coding-intl.dashscope.aliyuncs.com/apps/anthropic',
    modelListBaseUrl: 'https://coding-intl.dashscope.aliyuncs.com/v1',
  },
  {
    value: 'qwen-token-plan',
    label: 'Qwen Token Plan',
    openAIBaseUrl:
      'https://token-plan.ap-southeast-1.maas.aliyuncs.com/compatible-mode/v1',
    anthropicBaseUrl:
      'https://token-plan.ap-southeast-1.maas.aliyuncs.com/apps/anthropic',
    responsesBaseUrl:
      'https://token-plan.ap-southeast-1.maas.aliyuncs.com/compatible-mode/v1',
    modelListBaseUrl:
      'https://token-plan.ap-southeast-1.maas.aliyuncs.com/compatible-mode/v1',
  },
  {
    value: 'minimax-token-plan',
    label: 'MiniMax Token Plan',
    openAIBaseUrl: 'https://api.minimax.io/v1',
    anthropicBaseUrl: 'https://api.minimax.io/anthropic',
    responsesBaseUrl: 'https://api.minimax.io/v1',
    modelListBaseUrl: 'https://api.minimax.io/v1',
  },
  {
    value: 'opencode-go',
    label: 'OpenCode Go',
    openAIBaseUrl: 'https://opencode.ai/zen/go/v1',
    anthropicBaseUrl: 'https://opencode.ai/zen/go/v1',
    modelListBaseUrl: 'https://opencode.ai/zen/go/v1',
  },
]

export function getDefaultCodingPlanProvider(): string {
  return CODING_PLAN_PROVIDER_OPTIONS[0]?.value || ''
}

export function normalizeCodingPlanProvider(value: string | undefined): string {
  const normalized = String(value || '')
    .trim()
    .replace(/\/+$/, '')
  const provider = CODING_PLAN_PROVIDER_OPTIONS.find(
    (option) =>
      option.value === normalized ||
      option.openAIBaseUrl === normalized ||
      option.anthropicBaseUrl === normalized ||
      option.responsesBaseUrl === normalized
  )
  return provider?.value || normalized
}

export function buildCodingPlanAdvancedCustomConfig(
  providerValue: string | undefined
): AdvancedCustomConfig | null {
  const provider = CODING_PLAN_PROVIDER_OPTIONS.find(
    (option) => option.value === normalizeCodingPlanProvider(providerValue)
  )
  if (!provider) return null

  const routes: AdvancedCustomRoute[] = []
  if (provider.openAIBaseUrl) {
    const chatUrl = joinProviderPath(provider.openAIBaseUrl, 'chat/completions')
    routes.push(
      codingPlanRoute('/v1/chat/completions', chatUrl, 'none'),
      codingPlanRoute(
        '/v1/completions',
        joinProviderPath(provider.openAIBaseUrl, 'completions'),
        'none'
      ),
      codingPlanRoute(
        '/v1beta/models/{model}:generateContent',
        chatUrl,
        'gemini_generate_content_to_openai_chat_completions'
      )
    )
    if (!provider.responsesBaseUrl) {
      routes.push(
        codingPlanRoute(
          '/v1/responses',
          chatUrl,
          'openai_responses_to_openai_chat_completions'
        )
      )
    }
  }
  if (provider.responsesBaseUrl) {
    routes.push(
      codingPlanRoute(
        '/v1/responses',
        joinProviderPath(provider.responsesBaseUrl, 'responses'),
        'none'
      )
    )
  }
  if (provider.anthropicBaseUrl) {
    routes.push(
      codingPlanRoute(
        '/v1/messages',
        joinProviderPath(provider.anthropicBaseUrl, 'v1/messages'),
        'none'
      )
    )
  }
  if (provider.modelListBaseUrl) {
    routes.push(
      codingPlanRoute(
        '/v1/models',
        joinProviderPath(provider.modelListBaseUrl, 'models'),
        'none'
      )
    )
  }

  return { advanced_routes: routes }
}

function codingPlanRoute(
  incomingPath: string,
  upstreamPath: string,
  converter: AdvancedCustomRoute['converter']
): AdvancedCustomRoute {
  return {
    incoming_path: incomingPath,
    upstream_path: upstreamPath,
    converter,
    auth: {
      type: 'header',
      name: 'Authorization',
      value: 'Bearer {api_key}',
    },
  }
}

function joinProviderPath(baseUrl: string, path: string): string {
  return `${baseUrl.trim().replace(/\/+$/, '')}/${path.trim().replace(/^\/+/, '')}`
}
