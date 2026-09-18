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
import { ArrowUp02Icon, PlusSignIcon } from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useMutation } from '@tanstack/react-query'
import { useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { ModelGroupSelector } from '@/components/model-group-selector'
import { Button } from '@/components/ui/button'
import {
  InputGroup,
  InputGroupAddon,
  InputGroupTextarea,
} from '@/components/ui/input-group'
import { Spinner } from '@/components/ui/spinner'
import { sendChatCompletion } from '@/features/playground/api'
import { PlaygroundChat } from '@/features/playground/components/chat/playground-chat'
import {
  useConversationTitle,
  usePlaygroundConversation,
  usePlaygroundOptions,
  type usePlaygroundState,
} from '@/features/playground/hooks'
import {
  buildChatCompletionPayload,
  getMessageContent,
} from '@/features/playground/lib'
import type { Message } from '@/features/playground/types'

import {
  TICKET_TYPES,
  type TicketType,
  parseAssistantReply,
} from '../constants'
import { AttachmentPicker } from './attachment-picker'

export type AssistantTicketDraft = {
  type: TicketType
  subject: string
  content: string
  files?: File[]
}

export function SupportAssistant(props: {
  onCreateTicket: (draft: AssistantTicketDraft) => void
  onCreateBlankTicket: () => void
  state: ReturnType<typeof usePlaygroundState>
  files: File[]
  onFilesChange: (files: File[]) => void
  onBusyChange?: (busy: boolean) => void
}) {
  const { t } = useTranslation()
  const onBusyChange = props.onBusyChange
  const [input, setInput] = useState('')
  const [suggestion, setSuggestion] = useState<{
    type: TicketType
    subject: string
    sessionId: string
  } | null>(null)
  const {
    config,
    parameterEnabled,
    messages,
    activeSessionId,
    isLoadingMessages,
    models,
    groups,
    setModels,
    setGroups,
    updateConfig,
    updateMessages,
    createConversation,
    renameConversation,
    sessions,
  } = props.state
  const activeSessionRef = useRef(activeSessionId)
  const pendingFilesRef = useRef<File[]>([])
  const inputRef = useRef<HTMLTextAreaElement | null>(null)
  activeSessionRef.current = activeSessionId
  const { isLoadingModels } = usePlaygroundOptions({
    currentGroup: config.group,
    currentModel: config.model,
    setGroups,
    setModels,
    updateConfig,
  })
  const hasReadableFiles = props.files.some(
    (file) =>
      file.type.startsWith('image/') ||
      file.type === 'text/plain' ||
      file.name.endsWith('.log')
  )

  const ask = useMutation({
    mutationFn: async ({
      conversationMessages,
      files,
    }: {
      conversationMessages: Message[]
      files: File[]
    }) => {
      const questionMessage = [...conversationMessages]
        .reverse()
        .find((message) => message.from === 'user')
      if (!questionMessage) throw new Error('A user message is required.')
      const textFiles = await Promise.all(
        files
          .filter(
            (file) => file.type === 'text/plain' || file.name.endsWith('.log')
          )
          .map(
            async (file) =>
              `${file.name}:\n${(await file.text()).slice(0, 6000)}`
          )
      )
      const content = [getMessageContent(questionMessage), ...textFiles]
        .filter(Boolean)
        .join('\n\n')
      const requestMessages = conversationMessages
        .filter(
          (message) =>
            message.from !== 'assistant' || getMessageContent(message).trim()
        )
        .slice(-10)
        .map((message) =>
          message.key === questionMessage.key
            ? { ...message, versions: [{ ...message.versions[0], content }] }
            : message
        )
      // ponytail: ten recent messages keep support costs bounded without a
      // separate server-side conversation store.
      const payload = buildChatCompletionPayload(
        requestMessages,
        { ...config, stream: false },
        parameterEnabled
      )
      const images = await Promise.all(
        files
          .filter((file) => file.type.startsWith('image/'))
          .map(
            (file) =>
              new Promise<string>((resolve, reject) => {
                const reader = new FileReader()
                reader.addEventListener(
                  'load',
                  () => resolve(String(reader.result)),
                  { once: true }
                )
                reader.addEventListener('error', () => reject(reader.error), {
                  once: true,
                })
                reader.readAsDataURL(file)
              })
          )
      )
      if (images.length > 0) {
        const last = payload.messages.at(-1)
        if (last) {
          last.content = [
            {
              type: 'text',
              text: content || 'Please inspect the attached image.',
            },
            ...images.map((url) => ({
              type: 'image_url' as const,
              image_url: { url },
            })),
          ]
        }
      }
      payload.messages.unshift({
        role: 'system',
        content: `You are MoleAPI support triage. Help users troubleshoot safely and concisely. Never ask for passwords, complete API keys, or other secrets. Ask for request IDs and redacted errors when useful. If the problem is not resolved, recommend creating a support ticket. Reply in the user's language. End every response with exactly one machine-readable line in this format: <ticket>{"type":"TYPE","subject":"SHORT SUBJECT"}</ticket>. TYPE must be one of: ${TICKET_TYPES.map((item) => item.value).join(', ')}.`,
      })
      const response = await sendChatCompletion(payload)
      const answer = response.choices?.[0]?.message?.content?.trim()
      if (!answer) throw new Error('AI support returned an empty response.')
      return {
        assistantKey: conversationMessages.at(-1)?.key ?? '',
        parsed: parseAssistantReply(answer),
        sessionId: activeSessionId,
      }
    },
    onSuccess: ({ assistantKey, parsed, sessionId }) => {
      if (sessionId !== activeSessionRef.current) {
        return
      }
      updateMessages((current) =>
        current.map((message) =>
          message.key === assistantKey
            ? {
                ...message,
                versions: [{ ...message.versions[0], content: parsed.answer }],
                status: 'complete' as const,
                completedAt: Date.now(),
              }
            : message
        )
      )
      setSuggestion({ type: parsed.type, subject: parsed.subject, sessionId })
      setInput('')
    },
    onError: () => {
      updateMessages((current) => {
        const last = current.at(-1)
        if (!last || last.from !== 'assistant') return current
        return current.map((message) =>
          message.key === last.key
            ? { ...message, status: 'error' as const }
            : message
        )
      })
      toast.error(
        t(
          'AI support is temporarily unavailable. You can create a ticket directly.'
        )
      )
    },
  })

  const sendChat = (conversationMessages: Message[]) => {
    ask.mutate({ conversationMessages, files: pendingFilesRef.current })
    pendingFilesRef.current = []
  }
  const {
    editingMessageKey,
    handleSendMessage,
    handleRegenerateMessage,
    handleEditMessage,
    handleEditOpenChange,
    applyEdit,
    handleDeleteMessage,
  } = usePlaygroundConversation({ messages, updateMessages, sendChat })

  useConversationTitle({
    messages,
    sessionId: activeSessionId,
    currentTitle:
      sessions.find((session) => session.id === activeSessionId)?.title ?? '',
    group: config.group,
    onRename: renameConversation,
  })

  useEffect(() => {
    onBusyChange?.(ask.isPending)
    return () => onBusyChange?.(false)
  }, [ask.isPending, onBusyChange])

  const createTicket = () => {
    const firstQuestion = messages.find((message) => message.from === 'user')
    const firstQuestionContent = firstQuestion
      ? getMessageContent(firstQuestion)
      : ''
    const currentSuggestion =
      suggestion?.sessionId === activeSessionId ? suggestion : null
    const subject =
      currentSuggestion?.subject ||
      (firstQuestionContent || input.trim()).slice(0, 200)
    props.onCreateTicket({
      type: currentSuggestion?.type ?? 'Other',
      subject:
        subject.trim().length >= 3 ? subject.trim() : t('AI support request'),
      content: [
        t('AI-assisted support conversation'),
        ...messages.map(
          (message) =>
            `${message.from === 'user' ? t('You') : t('AI support')}: ${getMessageContent(message)}`
        ),
        ...(input.trim() ? [`${t('You')}: ${input.trim()}`] : []),
      ]
        .join('\n\n')
        .slice(0, 10000),
    })
  }

  const submit = () => {
    const question = input.trim()
    if (
      (!question && !hasReadableFiles) ||
      ask.isPending ||
      isLoadingMessages
    ) {
      return
    }
    pendingFilesRef.current = props.files
    props.onFilesChange([])
    handleSendMessage(question || t('Please inspect the attached files.'))
  }

  return (
    <div className='flex min-h-0 flex-1 flex-col'>
      <div className='border-b px-4 py-3 sm:px-5'>
        <div className='flex flex-wrap items-center justify-between gap-3'>
          <div>
            <h2 className='font-semibold'>
              {sessions.find((session) => session.id === activeSessionId)
                ?.title || t('Ask AI')}
            </h2>
            <p className='text-muted-foreground text-xs'>{t('Ask AI')}</p>
          </div>
          <div className='flex flex-wrap items-center gap-2'>
            <Button
              variant='outline'
              size='sm'
              onClick={createTicket}
              disabled={
                (messages.length === 0 &&
                  !input.trim() &&
                  props.files.length === 0) ||
                ask.isPending
              }
            >
              {t('Request human support')}
            </Button>
            <Button
              aria-label={t('New chat')}
              disabled={ask.isPending || isLoadingMessages}
              onClick={() => {
                setSuggestion(null)
                setInput('')
                props.onFilesChange([])
                createConversation()
              }}
              size='icon-sm'
              variant='outline'
            >
              <HugeiconsIcon icon={PlusSignIcon} />
            </Button>
          </div>
        </div>
      </div>

      <div className='min-h-0 flex-1'>
        <PlaygroundChat
          emptyState={
            <div className='flex min-h-[min(520px,calc(100svh-18rem))] items-center justify-center px-4 py-10'>
              <div className='w-full max-w-xl space-y-5 text-center'>
                <div className='space-y-2'>
                  <h3 className='text-xl font-semibold'>
                    {t('Describe the problem you are seeing')}
                  </h3>
                  <p className='text-muted-foreground text-sm leading-6'>
                    {t(
                      'AI can suggest checks and prepare a support request. It cannot confirm that a ticket was submitted until you create one.'
                    )}
                  </p>
                </div>
                <div className='flex flex-wrap justify-center gap-2'>
                  <Button
                    onClick={() => inputRef.current?.focus()}
                    size='sm'
                    variant='outline'
                  >
                    {t('Ask AI')}
                  </Button>
                  <Button onClick={props.onCreateBlankTicket} size='sm'>
                    {t('Create ticket')}
                  </Button>
                </div>
              </div>
            </div>
          }
          fullWidth
          messages={messages}
          isLoadingMessages={isLoadingMessages}
          isGenerating={ask.isPending}
          onSelectPrompt={handleSendMessage}
          editingKey={editingMessageKey}
          onEditMessage={handleEditMessage}
          onRegenerateMessage={handleRegenerateMessage}
          onDeleteMessage={handleDeleteMessage}
          onCancelEdit={handleEditOpenChange}
          onSaveEdit={(content) => applyEdit(content, false)}
          onSaveEditAndSubmit={(content) => applyEdit(content, true)}
          afterMessage={(message) => {
            const answer = getMessageContent(message)
            if (
              message.from !== 'assistant' ||
              message.status !== 'complete' ||
              !/(已|已经).{0,8}(记录|提交|转交|反馈)|\b(logged|submitted|reported|recorded)\b/i.test(
                answer
              )
            ) {
              return null
            }
            return (
              <Button
                className='mt-2'
                size='sm'
                variant='outline'
                onClick={createTicket}
              >
                {t('Report this as a ticket')}
              </Button>
            )
          }}
        />
        {ask.isError && (
          <p className='text-destructive mx-auto max-w-4xl px-4 pb-2 text-sm'>
            {t(
              'AI support is temporarily unavailable. You can create a ticket directly.'
            )}
          </p>
        )}
      </div>

      <div className='shrink-0 p-3 sm:p-4'>
        <form
          className='w-full'
          onSubmit={(event) => {
            event.preventDefault()
            submit()
          }}
        >
          <InputGroup className='bg-background overflow-hidden'>
            <InputGroupTextarea
              aria-label={t('Describe the problem you are seeing')}
              className='min-h-20 px-4 py-3'
              ref={inputRef}
              rows={3}
              disabled={ask.isPending || isLoadingMessages}
              value={input}
              placeholder={t(
                'Describe the error, request ID, and what you already tried...'
              )}
              onChange={(event) => setInput(event.target.value)}
              onKeyDown={(event) => {
                if (
                  event.key === 'Enter' &&
                  !event.shiftKey &&
                  !event.nativeEvent.isComposing
                ) {
                  event.preventDefault()
                  submit()
                }
              }}
            />
            <InputGroupAddon
              align='block-end'
              className='bg-muted/20 flex-wrap justify-between gap-2 border-t px-2 py-2'
            >
              <div className='flex min-w-0 flex-1 flex-wrap items-center gap-2'>
                <ModelGroupSelector
                  selectedModel={config.model}
                  models={models}
                  onModelChange={(value) => updateConfig('model', value)}
                  selectedGroup={config.group}
                  groups={groups}
                  onGroupChange={(value) => updateConfig('group', value)}
                  disabled={ask.isPending || isLoadingModels}
                />
                <AttachmentPicker
                  files={props.files}
                  onFilesChange={props.onFilesChange}
                  disabled={ask.isPending || isLoadingMessages}
                  compact
                  onRejected={(fileName) =>
                    toast.error(
                      t('{{file}} exceeds the 5 MB attachment limit.', {
                        file: fileName,
                      })
                    )
                  }
                />
              </div>
              <Button
                type='submit'
                size='icon-sm'
                className='shrink-0'
                disabled={
                  (!input.trim() && !hasReadableFiles) ||
                  ask.isPending ||
                  isLoadingMessages
                }
              >
                {ask.isPending ? (
                  <Spinner />
                ) : (
                  <HugeiconsIcon icon={ArrowUp02Icon} />
                )}
                <span className='sr-only'>{t('Send')}</span>
              </Button>
            </InputGroupAddon>
          </InputGroup>
        </form>
      </div>
    </div>
  )
}
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
