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
import {
  AiAudioIcon,
  AiBrain01Icon,
  AiChat01Icon,
  AiImageIcon,
  AiSearchIcon,
  AiVideoIcon,
  BracesIcon,
  CodeIcon,
  DatabaseSearchIcon,
  DatabaseSyncIcon,
  EyeIcon,
  File02Icon,
  FileCodeIcon,
  FunctionIcon,
  HeadphonesIcon,
  MagicWand01Icon,
  Pdf01Icon,
  RankingIcon,
  SecurityCheckIcon,
  Wrench01Icon,
} from '@hugeicons/core-free-icons'
import type { IconSvgElement } from '@hugeicons/react'

const MODEL_TAG_LABEL_KEYS: Record<string, string> = {
  对话: 'Chat',
  图片生成: 'Image generation',
  视频生成: 'Video generation',
  嵌入: 'Embeddings',
  重排序: 'Reranking',
  音频: 'Audio',
  安全审核: 'Safety',
  代码: 'Coding',
  推理: 'Reasoning',
  工具调用: 'Tools',
  缓存: 'Prompt caching',
  视觉: 'Vision',
  文生图: 'Text to image',
  图生图: 'Image to image',
  高质量: 'High quality',
  文生视频: 'Text to video',
  多模态: 'Multimodal',
  向量检索: 'Vector search',
  语义搜索: 'Semantic search',
  检索增强: 'Retrieval',
  排序优化: 'Ranking',
  语音: 'Speech',
  实时: 'Realtime',
  内容安全: 'Content safety',
  策略审核: 'Policy review',
  代码生成: 'Code generation',
  仓库理解: 'Repository context',
  开源权重: 'Open weights',
  长上下文: 'Long context',
}

const MODEL_TAG_ICONS: Partial<Record<string, IconSvgElement>> = {
  Chat: AiChat01Icon,
  'Image generation': AiImageIcon,
  'Video generation': AiVideoIcon,
  Embeddings: DatabaseSearchIcon,
  Reranking: RankingIcon,
  Audio: AiAudioIcon,
  Safety: SecurityCheckIcon,
  Coding: CodeIcon,
  Reasoning: AiBrain01Icon,
  Tools: Wrench01Icon,
  'Prompt caching': DatabaseSyncIcon,
  Vision: EyeIcon,
  'Text to image': AiImageIcon,
  'Image to image': AiImageIcon,
  'High quality': MagicWand01Icon,
  'Text to video': AiVideoIcon,
  Multimodal: BracesIcon,
  'Vector search': DatabaseSearchIcon,
  'Semantic search': AiSearchIcon,
  Retrieval: AiSearchIcon,
  Ranking: RankingIcon,
  Speech: AiAudioIcon,
  Realtime: HeadphonesIcon,
  'Content safety': SecurityCheckIcon,
  'Policy review': SecurityCheckIcon,
  'Code generation': CodeIcon,
  'Repository context': FileCodeIcon,
  'Open weights': BracesIcon,
  'Long context': File02Icon,
  'Function calling': FunctionIcon,
  Streaming: HeadphonesIcon,
  'JSON mode': BracesIcon,
  'Structured output': BracesIcon,
  'System prompt': File02Icon,
  'Web search': AiSearchIcon,
  'Code interpreter': CodeIcon,
  PDF: Pdf01Icon,
}

export function getModelTagLabelKey(tag: string): string {
  return MODEL_TAG_LABEL_KEYS[tag] ?? tag
}

export function getModelTagIcon(tag: string): IconSvgElement | undefined {
  return MODEL_TAG_ICONS[getModelTagLabelKey(tag)]
}
