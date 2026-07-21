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
export interface ApiDemoConfig {
  id: string
  label: string
  method: 'POST'
  endpoint: string
  headers: Record<string, string>
  payload: Record<string, unknown>
}

export const PUBLIC_API_BASE_URL = 'https://api.moleapi.com'

const bearerHeader = { Authorization: 'Bearer sk-••••' }

export const API_DEMOS: ApiDemoConfig[] = [
  {
    id: 'gpt-chat',
    label: 'Chat',
    method: 'POST',
    endpoint: '/v1/chat/completions',
    headers: bearerHeader,
    payload: {
      model: 'your-model',
      messages: [{ role: 'user', content: 'Hello, MoleAPI!' }],
    },
  },
  {
    id: 'responses',
    label: 'Responses',
    method: 'POST',
    endpoint: '/v1/responses',
    headers: bearerHeader,
    payload: { model: 'your-model', input: 'Hello, MoleAPI!' },
  },
  {
    id: 'embeddings',
    label: 'Embeddings',
    method: 'POST',
    endpoint: '/v1/embeddings',
    headers: bearerHeader,
    payload: { model: 'your-model', input: 'Text to embed' },
  },
  {
    id: 'rerank',
    label: 'Rerank',
    method: 'POST',
    endpoint: '/v1/rerank',
    headers: bearerHeader,
    payload: {
      model: 'your-model',
      query: 'Best matching document',
      documents: ['Document A', 'Document B'],
    },
  },
  {
    id: 'moderation',
    label: 'Moderation',
    method: 'POST',
    endpoint: '/v1/moderations',
    headers: bearerHeader,
    payload: { model: 'your-model', input: 'Content to review' },
  },
  {
    id: 'images',
    label: 'Images',
    method: 'POST',
    endpoint: '/v1/images/generations',
    headers: bearerHeader,
    payload: {
      model: 'your-model',
      prompt: 'A friendly mole building an AI gateway',
      size: '1024x1024',
    },
  },
  {
    id: 'claude',
    label: 'Messages',
    method: 'POST',
    endpoint: '/v1/messages',
    headers: {
      'x-api-key': 'sk-••••',
      'anthropic-version': '2023-06-01',
    },
    payload: {
      model: 'your-model',
      max_tokens: 1024,
      messages: [{ role: 'user', content: 'Hello, MoleAPI!' }],
    },
  },
  {
    id: 'gemini',
    label: 'Gemini',
    method: 'POST',
    endpoint: '/v1beta/models/{model}:generateContent',
    headers: { 'x-goog-api-key': 'sk-••••' },
    payload: {
      contents: [{ role: 'user', parts: [{ text: 'Hello, MoleAPI!' }] }],
    },
  },
]
