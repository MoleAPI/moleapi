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
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { Textarea } from '@/components/ui/textarea'
import { TitledCard } from '@/components/ui/titled-card'
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
import { TicketCreateForm } from './components/ticket-create-form'

export function Support() {
  const { t } = useTranslation()
  const isAdmin = useIsAdmin()
  const accountEmail = useAuthStore((state) => state.auth.user?.email)
  const queryClient = useQueryClient()
  const [selectedTicket, setSelectedTicket] = useState<string | null>(null)
  const [activeTab, setActiveTab] = useState('create')
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
    <SectionPageLayout>
      <SectionPageLayout.Title>
        {t('Support & Community')}
      </SectionPageLayout.Title>
      <SectionPageLayout.Content>
        <div className='mx-auto flex w-full max-w-7xl flex-col gap-6'>
          <CommunityChannels links={config.data?.community_links ?? {}} />

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
              title={t('Ticket management')}
              description={t('Replying here also sends an email to the user.')}
              icon={<HugeiconsIcon icon={CustomerSupportIcon} />}
              iconTone='info'
              contentClassName='p-0 sm:p-0'
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
              title={t('Support tickets')}
              description={t(
                'Submit a request or continue a conversation with support.'
              )}
              icon={<HugeiconsIcon icon={CustomerSupportIcon} />}
              iconTone='info'
              disableHoverEffect
            >
              <Tabs value={activeTab} onValueChange={setActiveTab}>
                <TabsList>
                  <TabsTrigger value='create'>
                    {t('Create a ticket')}
                  </TabsTrigger>
                  <TabsTrigger value='tickets'>{t('My tickets')}</TabsTrigger>
                </TabsList>
                <TabsContent value='create' className='pt-4'>
                  <TicketCreateForm
                    accountEmail={accountEmail}
                    onCreated={(id) => {
                      setSelectedTicket(id)
                      setActiveTab('tickets')
                    }}
                  />
                </TabsContent>
                <TabsContent value='tickets' className='pt-4'>
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
                </TabsContent>
              </Tabs>
            </TitledCard>
          )}
        </div>
      </SectionPageLayout.Content>
    </SectionPageLayout>
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
    <div className='grid min-h-[32rem] overflow-hidden rounded-lg border md:grid-cols-[minmax(16rem,22rem)_minmax(0,1fr)]'>
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
                    #{ticket.ticketNumber} · {ticket.category}
                  </span>
                </span>
                <Badge variant='secondary'>{ticket.status}</Badge>
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
            #{props.data.ticket.ticketNumber} · {props.data.ticket.category}
          </p>
        </div>
        <Badge variant='secondary'>{props.data.ticket.status}</Badge>
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
