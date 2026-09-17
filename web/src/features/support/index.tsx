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
  Add01Icon,
  AiChat02Icon,
  ArrowLeft01Icon,
  CustomerSupportIcon,
  File01Icon,
  Message01Icon,
} from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { SectionPageLayout } from '@/components/layout'
import { LoadingState } from '@/components/loading-state'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from '@/components/ui/empty'
import { Field, FieldLabel } from '@/components/ui/field'
import { ScrollArea } from '@/components/ui/scroll-area'
import { Spinner } from '@/components/ui/spinner'
import { Textarea } from '@/components/ui/textarea'
import { TitledCard } from '@/components/ui/titled-card'
import { usePlaygroundState } from '@/features/playground/hooks'
import { useIsAdmin } from '@/hooks/use-admin'
import { handleServerError } from '@/lib/handle-server-error'
import { cn } from '@/lib/utils'
import { useAuthStore } from '@/stores/auth-store'

import {
  downloadSupportAttachment,
  getSupportConfig,
  getSupportTicket,
  getSupportTickets,
  replySupportTicket,
} from './api'
import { CommunityChannels } from './components/community-channels'
import {
  SupportAssistant,
  type AssistantTicketDraft,
} from './components/support-assistant'
import { TicketCreateForm } from './components/ticket-create-form'

export function Support() {
  const { t } = useTranslation()
  const isAdmin = useIsAdmin()
  const accountEmail = useAuthStore((state) => state.auth.user?.email)
  const queryClient = useQueryClient()
  const [selectedTicket, setSelectedTicket] = useState<string | null>(null)
  const [userPanel, setUserPanel] = useState<'overview' | 'ai' | 'create'>(
    'overview'
  )
  const [ticketDraft, setTicketDraft] = useState<AssistantTicketDraft | null>(
    null
  )
  const [reply, setReply] = useState('')
  const config = useQuery({
    queryKey: ['support', 'config'],
    queryFn: getSupportConfig,
  })
  const tickets = useQuery({
    queryKey: ['support', 'tickets'],
    queryFn: getSupportTickets,
    enabled:
      config.data?.enabled === true && (isAdmin || Boolean(accountEmail)),
  })
  const detail = useQuery({
    queryKey: ['support', 'ticket', selectedTicket],
    queryFn: () => {
      if (!selectedTicket) throw new Error('Ticket ID is required')
      return getSupportTicket(selectedTicket)
    },
    enabled: selectedTicket != null,
  })
  const sendReply = useMutation({
    mutationFn: () => {
      if (!selectedTicket) throw new Error('Ticket ID is required')
      return replySupportTicket(selectedTicket, reply)
    },
    onSuccess: async () => {
      setReply('')
      await queryClient.invalidateQueries({
        queryKey: ['support', 'ticket', selectedTicket],
      })
      await queryClient.invalidateQueries({ queryKey: ['support', 'tickets'] })
      toast.success(t('Reply sent'))
    },
    onError: (error) => handleServerError(error),
  })

  return (
    <SectionPageLayout fixedContent>
      <SectionPageLayout.Title>{t('Support center')}</SectionPageLayout.Title>
      <SectionPageLayout.Actions>
        <CommunityChannels links={config.data?.community_links ?? {}} />
      </SectionPageLayout.Actions>
      <SectionPageLayout.Content>
        <div className='flex h-full min-h-0 w-full flex-col gap-4'>
          {config.isLoading && <LoadingState />}
          {!config.isLoading && !config.data?.enabled && (
            <Alert>
              <AlertTitle>{t('Ticket service is being prepared')}</AlertTitle>
              <AlertDescription>
                {isAdmin
                  ? t('Complete the Zoho Desk settings to enable tickets.')
                  : t('Please use the community or support email for now.')}
              </AlertDescription>
            </Alert>
          )}

          {config.data?.enabled && isAdmin && (
            <TitledCard
              className='min-h-0 flex-1'
              title={t('Ticket management')}
              description={t('Replying here also sends an email to the user.')}
              icon={<HugeiconsIcon icon={CustomerSupportIcon} />}
              iconTone='info'
              contentClassName='min-h-0 flex-1 p-0 sm:p-0'
              disableHoverEffect
            >
              <TicketBrowser
                loading={tickets.isLoading}
                tickets={tickets.data ?? []}
                selectedTicket={selectedTicket}
                onSelect={setSelectedTicket}
                detail={detail.data}
                detailLoading={detail.isLoading}
                reply={reply}
                onReplyChange={setReply}
                onSend={() => sendReply.mutate(undefined)}
                sending={sendReply.isPending}
              />
            </TitledCard>
          )}

          {config.data?.enabled && !isAdmin && (
            <TitledCard
              className='min-h-0 flex-1'
              title={t('Support tickets')}
              description={t(
                'Submit a request or continue a conversation with support.'
              )}
              icon={<HugeiconsIcon icon={CustomerSupportIcon} />}
              iconTone='info'
              contentClassName='min-h-0 flex-1 p-0 sm:p-0'
              disableHoverEffect
            >
              <UserSupportWorkspace
                accountEmail={accountEmail}
                loading={tickets.isLoading}
                tickets={tickets.data ?? []}
                selectedTicket={selectedTicket}
                onSelectTicket={(id) => {
                  setSelectedTicket(id)
                  setUserPanel('overview')
                }}
                panel={userPanel}
                onPanelChange={(panel) => {
                  setSelectedTicket(null)
                  setUserPanel(panel)
                  if (panel === 'create') setTicketDraft(null)
                }}
                draft={ticketDraft}
                onAssistantDraft={(draft) => {
                  setTicketDraft(draft)
                  setSelectedTicket(null)
                  setUserPanel('create')
                }}
                detail={detail.data}
                detailLoading={detail.isLoading}
                reply={reply}
                onReplyChange={setReply}
                onSend={() => sendReply.mutate(undefined)}
                sending={sendReply.isPending}
              />
            </TitledCard>
          )}
        </div>
      </SectionPageLayout.Content>
    </SectionPageLayout>
  )
}

function UserSupportWorkspace(props: {
  accountEmail?: string
  loading: boolean
  tickets: Awaited<ReturnType<typeof getSupportTickets>>
  selectedTicket: string | null
  onSelectTicket: (id: string | null) => void
  panel: 'overview' | 'ai' | 'create'
  onPanelChange: (panel: 'overview' | 'ai' | 'create') => void
  draft: AssistantTicketDraft | null
  onAssistantDraft: (draft: AssistantTicketDraft) => void
  detail: Awaited<ReturnType<typeof getSupportTicket>> | undefined
  detailLoading: boolean
  reply: string
  onReplyChange: (value: string) => void
  onSend: () => void
  sending: boolean
}) {
  const { t } = useTranslation()
  const assistantState = usePlaygroundState('support')
  const [assistantFiles, setAssistantFiles] = useState<File[]>([])
  const [assistantBusy, setAssistantBusy] = useState(false)
  const showingPanel =
    Boolean(props.selectedTicket) || props.panel !== 'overview'
  const goBack = () => {
    props.onSelectTicket(null)
    props.onPanelChange('overview')
  }

  return (
    <div className='grid h-full min-h-0 overflow-hidden md:grid-cols-[minmax(16rem,20rem)_minmax(0,1fr)]'>
      <aside
        className={cn(
          'min-h-0 flex-col border-r',
          showingPanel ? 'hidden md:flex' : 'flex'
        )}
      >
        <div className='grid grid-cols-2 gap-2 border-b p-3'>
          <Button
            variant={props.panel === 'ai' ? 'secondary' : 'outline'}
            disabled={assistantBusy}
            onClick={() => props.onPanelChange('ai')}
          >
            <HugeiconsIcon icon={AiChat02Icon} data-icon='inline-start' />
            {t('Ask AI')}
          </Button>
          <Button
            disabled={assistantBusy}
            onClick={() => props.onPanelChange('create')}
          >
            <HugeiconsIcon icon={Add01Icon} data-icon='inline-start' />
            {t('Create ticket')}
          </Button>
        </div>
        <div className='flex h-12 items-center gap-2 border-b px-4 font-medium'>
          {t('My tickets')}
          <Badge variant='secondary'>{props.tickets.length}</Badge>
        </div>
        <ScrollArea className='min-h-0 flex-1'>
          {assistantState.sessions.some(
            (session) => session.messages.length > 0
          ) && (
            <div className='border-b py-2'>
              <p className='text-muted-foreground px-4 py-1 text-xs font-medium'>
                {t('AI conversations')}
              </p>
              {[...assistantState.sessions]
                .filter((session) => session.messages.length > 0)
                .sort((a, b) => b.updatedAt - a.updatedAt)
                .map((session) => (
                  <button
                    key={session.id}
                    type='button'
                    disabled={assistantBusy}
                    aria-current={
                      props.panel === 'ai' &&
                      !props.selectedTicket &&
                      assistantState.activeSessionId === session.id
                        ? 'page'
                        : undefined
                    }
                    className={cn(
                      'hover:bg-muted/60 flex w-full items-center gap-3 px-4 py-3 text-left text-sm',
                      props.panel === 'ai' &&
                        !props.selectedTicket &&
                        assistantState.activeSessionId === session.id &&
                        'bg-muted'
                    )}
                    onClick={() => {
                      assistantState.selectConversation(session.id)
                      setAssistantFiles([])
                      props.onPanelChange('ai')
                    }}
                  >
                    <HugeiconsIcon
                      icon={AiChat02Icon}
                      className='text-muted-foreground size-4 shrink-0'
                      aria-hidden='true'
                    />
                    <span className='truncate'>
                      {session.title || t('New chat')}
                    </span>
                  </button>
                ))}
            </div>
          )}
          {props.loading && <LoadingState />}
          {!props.loading &&
            props.tickets.length === 0 &&
            assistantState.sessions.every(
              (session) => session.messages.length === 0
            ) && (
              <div className='text-muted-foreground px-4 py-8 text-center text-sm'>
                {t('No tickets yet')}
              </div>
            )}
          {!props.loading && props.tickets.length > 0 && (
            <div className='divide-y'>
              {props.tickets.map((ticket) => (
                <button
                  key={ticket.id}
                  type='button'
                  disabled={assistantBusy}
                  className={cn(
                    'hover:bg-muted/60 flex w-full items-start gap-3 px-4 py-3.5 text-left transition-colors',
                    props.selectedTicket === ticket.id && 'bg-muted'
                  )}
                  onClick={() => props.onSelectTicket(ticket.id)}
                >
                  <HugeiconsIcon
                    icon={File01Icon}
                    className='text-muted-foreground mt-0.5 size-4 shrink-0'
                    aria-hidden='true'
                  />
                  <span className='min-w-0 flex-1'>
                    <span className='block truncate text-sm font-medium'>
                      {ticket.subject}
                    </span>
                    <span className='text-muted-foreground mt-1 block truncate text-xs'>
                      #{ticket.ticketNumber} · {t(ticket.category)}
                    </span>
                  </span>
                  <Badge variant='secondary'>{t(ticket.status)}</Badge>
                </button>
              ))}
            </div>
          )}
        </ScrollArea>
      </aside>

      <section
        className={cn(
          'min-h-0 flex-col',
          showingPanel ? 'flex' : 'hidden md:flex'
        )}
      >
        {!showingPanel && <SupportOverview />}
        {!props.selectedTicket && props.panel !== 'overview' && (
          <div className='flex items-center border-b px-3 py-2 md:hidden'>
            <Button
              variant='ghost'
              size='icon-sm'
              disabled={assistantBusy}
              onClick={goBack}
            >
              <HugeiconsIcon icon={ArrowLeft01Icon} />
              <span className='sr-only'>{t('Back to tickets')}</span>
            </Button>
          </div>
        )}
        {props.panel === 'ai' && !props.selectedTicket && (
          <SupportAssistant
            state={assistantState}
            files={assistantFiles}
            onFilesChange={setAssistantFiles}
            onBusyChange={setAssistantBusy}
            onCreateTicket={(draft) => {
              props.onAssistantDraft({ ...draft, files: assistantFiles })
            }}
          />
        )}
        {props.panel === 'create' && !props.selectedTicket && (
          <ScrollArea className='min-h-0 flex-1'>
            <div className='p-5 sm:p-7'>
              <div className='mx-auto mb-6 w-full max-w-3xl'>
                <h2 className='font-semibold'>
                  {t('Create a support ticket')}
                </h2>
                <p className='text-muted-foreground mt-1 text-sm'>
                  {t(
                    'Choose a type and include enough detail for support to investigate.'
                  )}
                </p>
              </div>
              <TicketCreateForm
                key={
                  props.draft
                    ? `${props.draft.type}-${props.draft.subject}`
                    : 'blank'
                }
                accountEmail={props.accountEmail}
                initialValues={props.draft ?? undefined}
                initialFiles={props.draft?.files}
                onCreated={(id) => props.onSelectTicket(id)}
              />
            </div>
          </ScrollArea>
        )}
        {props.selectedTicket && (props.detailLoading || !props.detail) && (
          <LoadingState />
        )}
        {props.selectedTicket && props.detail && (
          <TicketDetail
            data={props.detail}
            reply={props.reply}
            onReplyChange={props.onReplyChange}
            onBack={goBack}
            onSend={props.onSend}
            sending={props.sending}
          />
        )}
      </section>
    </div>
  )
}

function SupportOverview() {
  const { t } = useTranslation()
  return (
    <div className='flex flex-1 items-center justify-center p-6'>
      <div className='w-full max-w-lg'>
        <div className='bg-primary/10 text-primary mb-4 flex size-10 items-center justify-center rounded-md'>
          <HugeiconsIcon icon={CustomerSupportIcon} className='size-6' />
        </div>
        <h2 className='text-xl font-semibold'>{t('How can support help?')}</h2>
        <div className='mt-6 divide-y border-y'>
          {[
            t('Troubleshoot errors and connect faster with AI'),
            t('Send error details or bug reports to support'),
            t('Request invoices or discuss a partnership'),
          ].map((item, index) => (
            <div key={item} className='flex items-center gap-4 py-4 text-sm'>
              <span className='text-muted-foreground w-5 shrink-0 tabular-nums'>
                0{index + 1}
              </span>
              <p className='font-medium'>{item}</p>
            </div>
          ))}
        </div>
      </div>
    </div>
  )
}

function TicketBrowser(props: {
  loading: boolean
  tickets: Awaited<ReturnType<typeof getSupportTickets>>
  selectedTicket: string | null
  onSelect: (id: string | null) => void
  detail: Awaited<ReturnType<typeof getSupportTicket>> | undefined
  detailLoading: boolean
  reply: string
  onReplyChange: (value: string) => void
  onSend: () => void
  sending: boolean
}) {
  const { t } = useTranslation()
  if (props.loading) return <LoadingState />
  if (props.tickets.length === 0) {
    return (
      <Empty className='py-16'>
        <EmptyHeader>
          <EmptyMedia variant='icon'>
            <HugeiconsIcon icon={Message01Icon} />
          </EmptyMedia>
          <EmptyTitle>{t('No tickets yet')}</EmptyTitle>
          <EmptyDescription>
            {t('New tickets will appear here.')}
          </EmptyDescription>
        </EmptyHeader>
      </Empty>
    )
  }

  return (
    <div className='grid h-full min-h-0 overflow-hidden md:grid-cols-[minmax(16rem,22rem)_minmax(0,1fr)]'>
      <aside
        className={cn(
          'min-h-0 flex-col border-r',
          props.selectedTicket ? 'hidden md:flex' : 'flex'
        )}
      >
        <div className='flex h-12 items-center gap-2 border-b px-4 font-medium'>
          {t('Tickets')}
          <Badge variant='secondary'>{props.tickets.length}</Badge>
        </div>
        <ScrollArea className='min-h-0 flex-1'>
          <div className='divide-y'>
            {props.tickets.map((ticket) => (
              <button
                key={ticket.id}
                type='button'
                className={cn(
                  'hover:bg-muted/60 flex w-full items-start gap-3 px-4 py-3.5 text-left transition-colors',
                  props.selectedTicket === ticket.id && 'bg-muted'
                )}
                onClick={() => props.onSelect(ticket.id)}
              >
                <HugeiconsIcon
                  icon={File01Icon}
                  className='text-muted-foreground mt-0.5 size-4 shrink-0'
                  aria-hidden='true'
                />
                <span className='min-w-0 flex-1'>
                  <span className='block truncate text-sm font-medium'>
                    {ticket.subject}
                  </span>
                  <span className='text-muted-foreground mt-1 block truncate text-xs'>
                    #{ticket.ticketNumber} · {t(ticket.category)}
                  </span>
                </span>
                <Badge variant='secondary'>{t(ticket.status)}</Badge>
              </button>
            ))}
          </div>
        </ScrollArea>
      </aside>

      <section
        className={cn(
          'min-h-0 flex-col',
          props.selectedTicket ? 'flex' : 'hidden md:flex'
        )}
      >
        {!props.selectedTicket && (
          <Empty className='flex-1'>
            <EmptyHeader>
              <EmptyMedia variant='icon'>
                <HugeiconsIcon icon={Message01Icon} />
              </EmptyMedia>
              <EmptyTitle>{t('Select a ticket')}</EmptyTitle>
              <EmptyDescription>
                {t(
                  'Open a ticket to view messages and continue the conversation.'
                )}
              </EmptyDescription>
            </EmptyHeader>
          </Empty>
        )}
        {props.selectedTicket && (props.detailLoading || !props.detail) && (
          <LoadingState />
        )}
        {props.selectedTicket && props.detail && (
          <TicketDetail
            data={props.detail}
            reply={props.reply}
            onReplyChange={props.onReplyChange}
            onBack={() => props.onSelect(null)}
            onSend={props.onSend}
            sending={props.sending}
          />
        )}
      </section>
    </div>
  )
}

function TicketDetail(props: {
  data: Awaited<ReturnType<typeof getSupportTicket>>
  reply: string
  onReplyChange: (value: string) => void
  onBack: () => void
  onSend: () => void
  sending: boolean
}) {
  const { t } = useTranslation()
  const downloadAttachment = async (
    attachment: (typeof props.data.attachments)[number]
  ) => {
    try {
      const file = await downloadSupportAttachment(
        props.data.ticket.id,
        attachment
      )
      const link = document.createElement('a')
      link.href = file.url
      link.download = file.filename
      link.click()
      URL.revokeObjectURL(file.url)
    } catch (error) {
      handleServerError(error)
    }
  }
  return (
    <div className='flex min-h-0 flex-1 flex-col'>
      <div className='flex min-h-14 shrink-0 items-center gap-2 border-b px-4 py-2.5'>
        <Button variant='ghost' size='icon-sm' onClick={props.onBack}>
          <HugeiconsIcon icon={ArrowLeft01Icon} />
          <span className='sr-only'>{t('Back to tickets')}</span>
        </Button>
        <div className='min-w-0 flex-1'>
          <h2 className='truncate font-semibold'>
            {props.data.ticket.subject}
          </h2>
          <p className='text-muted-foreground truncate text-xs'>
            #{props.data.ticket.ticketNumber} · {t(props.data.ticket.category)}
          </p>
        </div>
        <Badge variant='secondary'>{t(props.data.ticket.status)}</Badge>
      </div>

      <ScrollArea className='min-h-0 flex-1'>
        <div className='mx-auto w-full max-w-3xl px-5'>
          <Message
            sender={props.data.ticket.email}
            content={props.data.ticket.description}
          />
          {props.data.conversations.map((message) => (
            <Message
              key={message.id}
              sender={
                message.fromEmailAddress || message.direction || message.type
              }
              content={message.content || message.summary}
            />
          ))}
          {props.data.attachments.length > 0 && (
            <div className='flex flex-wrap gap-2 border-b py-5'>
              <p className='w-full text-sm font-medium'>{t('Attachments')}</p>
              {props.data.attachments.map((attachment) => (
                <Button
                  key={attachment.id}
                  variant='outline'
                  size='sm'
                  onClick={() => downloadAttachment(attachment)}
                >
                  <HugeiconsIcon icon={File01Icon} data-icon='inline-start' />
                  {attachment.name}
                </Button>
              ))}
            </div>
          )}
        </div>
      </ScrollArea>

      <Field className='shrink-0 gap-2 border-t p-4'>
        <FieldLabel htmlFor='ticket-reply'>{t('Reply')}</FieldLabel>
        <Textarea
          id='ticket-reply'
          rows={4}
          value={props.reply}
          onChange={(event) => props.onReplyChange(event.target.value)}
        />
        <div className='flex justify-end'>
          <Button
            onClick={props.onSend}
            disabled={!props.reply.trim() || props.sending}
          >
            {props.sending && <Spinner data-icon='inline-start' />}
            {props.sending ? t('Sending...') : t('Send reply')}
          </Button>
        </div>
      </Field>
    </div>
  )
}

function Message(props: { sender: string; content: string }) {
  return (
    <article className='border-b py-5 last:border-b-0'>
      <p className='text-muted-foreground mb-2 text-xs'>{props.sender}</p>
      <p className='text-sm leading-relaxed whitespace-pre-wrap'>
        {props.content}
      </p>
    </article>
  )
}
