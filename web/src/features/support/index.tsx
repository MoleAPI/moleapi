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
import { zodResolver } from '@hookform/resolvers/zod'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import {
  ArrowLeft,
  ExternalLink,
  FileText,
  LifeBuoy,
  MessageSquare,
  Plus,
} from 'lucide-react'
import { useState } from 'react'
import { Controller, useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { EmptyState } from '@/components/empty-state'
import { SectionPageLayout } from '@/components/layout'
import { LoadingState } from '@/components/loading-state'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { ScrollArea } from '@/components/ui/scroll-area'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Textarea } from '@/components/ui/textarea'
import { useIsAdmin } from '@/hooks/use-admin'
import { handleServerError } from '@/lib/handle-server-error'
import { cn } from '@/lib/utils'

import {
  createSupportTicket,
  getSupportConfig,
  getSupportTicket,
  getSupportTickets,
  replySupportTicket,
  ticketSchema,
} from './api'

type TicketForm = import('zod').infer<typeof ticketSchema>

const communityNames: Record<string, string> = {
  qq: 'QQ',
  wechat: 'WeChat',
  telegram: 'Telegram',
  discord: 'Discord',
}

export function Support() {
  const { t } = useTranslation()
  const isAdmin = useIsAdmin()
  const queryClient = useQueryClient()
  const [selectedTicket, setSelectedTicket] = useState<string | null>(null)
  const [creating, setCreating] = useState(false)
  const [reply, setReply] = useState('')
  const config = useQuery({
    queryKey: ['support', 'config'],
    queryFn: getSupportConfig,
  })
  const tickets = useQuery({
    queryKey: ['support', 'tickets'],
    queryFn: getSupportTickets,
    enabled: config.data?.enabled === true,
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

  const links = Object.entries(config.data?.community_links ?? {}).filter(
    ([, href]) => href,
  )
  const detailOpen = creating || selectedTicket != null

  const showTickets = () => {
    setCreating(false)
    setSelectedTicket(null)
  }

  let detailPane = (
    <EmptyState
      icon={MessageSquare}
      title={
        tickets.data?.length
          ? t('Open a ticket to view messages and continue the conversation.')
          : t('No tickets yet')
      }
      description={t('New tickets will appear here.')}
    />
  )
  if (creating) {
    detailPane = (
      <CreateTicketPane
        onBack={showTickets}
        onCreated={(id) => {
          setCreating(false)
          setSelectedTicket(id)
        }}
      />
    )
  } else if (selectedTicket) {
    detailPane =
      detail.isLoading || !detail.data ? (
        <LoadingState />
      ) : (
        <TicketDetail
          data={detail.data}
          reply={reply}
          onReplyChange={setReply}
          onBack={showTickets}
          onSend={() => sendReply.mutate(undefined)}
          sending={sendReply.isPending}
        />
      )
  }

  return (
    <SectionPageLayout fixedContent>
      <SectionPageLayout.Title>
        {t('Support & Community')}
      </SectionPageLayout.Title>
      <SectionPageLayout.Actions>
        {links.length === 0 ? (
          <span className="text-muted-foreground text-xs">
            {t('Community links are coming soon.')}
          </span>
        ) : (
          links.map(([name, href]) => (
            <Button
              key={name}
              size="sm"
              variant="outline"
              render={<a href={href} target="_blank" rel="noreferrer" />}
            >
              {communityNames[name] ?? name}
              <ExternalLink aria-hidden="true" />
            </Button>
          ))
        )}
      </SectionPageLayout.Actions>
      <SectionPageLayout.Content>
        <div className="mx-auto flex size-full max-w-7xl flex-col">
          {!config.data?.enabled &&
            (config.isLoading ? (
              <LoadingState />
            ) : (
              <EmptyState
                icon={LifeBuoy}
                title={t('Ticket service is being prepared')}
                description={
                  isAdmin
                    ? t('Complete the Zoho Desk settings to enable tickets.')
                    : t('Please use the community or support email for now.')
                }
                bordered
              />
            ))}

          {config.data?.enabled && (
            <div className="border-border bg-card grid min-h-0 flex-1 overflow-hidden rounded-xl border md:grid-cols-[minmax(16rem,22rem)_minmax(0,1fr)]">
              <aside
                className={cn(
                  'min-h-0 flex-col border-r',
                  detailOpen ? 'hidden md:flex' : 'flex',
                )}
              >
                <div className="flex h-14 shrink-0 items-center justify-between gap-3 border-b px-4">
                  <div className="flex min-w-0 items-center gap-2">
                    <h2 className="truncate font-semibold">
                      {isAdmin ? t('Ticket management') : t('My tickets')}
                    </h2>
                    <Badge variant="secondary">
                      {tickets.data?.length ?? 0}
                    </Badge>
                  </div>
                  {!isAdmin && (
                    <Button
                      size="sm"
                      onClick={() => {
                        setSelectedTicket(null)
                        setCreating(true)
                      }}
                    >
                      <Plus aria-hidden="true" />
                      {t('Create a ticket')}
                    </Button>
                  )}
                </div>
                <TicketList
                  loading={tickets.isLoading}
                  tickets={tickets.data ?? []}
                  selectedTicket={selectedTicket}
                  onSelect={(id) => {
                    setCreating(false)
                    setSelectedTicket(id)
                  }}
                />
              </aside>

              <section
                className={cn(
                  'min-h-0 flex-col',
                  detailOpen ? 'flex' : 'hidden md:flex',
                )}
              >
                {detailPane}
              </section>
            </div>
          )}
        </div>
      </SectionPageLayout.Content>
    </SectionPageLayout>
  )
}

function TicketList(props: {
  loading: boolean
  tickets: Awaited<ReturnType<typeof getSupportTickets>>
  selectedTicket: string | null
  onSelect: (id: string) => void
}) {
  const { t } = useTranslation()
  if (props.loading) return <LoadingState />
  if (props.tickets.length === 0) {
    return (
      <EmptyState
        icon={MessageSquare}
        title={t('No tickets yet')}
        description={t('New tickets will appear here.')}
      />
    )
  }
  return (
    <ScrollArea className="min-h-0 flex-1">
      <div className="divide-y">
        {props.tickets.map((ticket) => (
          <button
            key={ticket.id}
            type="button"
            className={cn(
              'hover:bg-muted/60 flex w-full items-start gap-3 px-4 py-3.5 text-left transition-colors',
              props.selectedTicket === ticket.id && 'bg-muted',
            )}
            onClick={() => props.onSelect(ticket.id)}
          >
            <FileText className="text-muted-foreground mt-0.5 size-4 shrink-0" />
            <span className="min-w-0 flex-1">
              <span className="block truncate text-sm font-medium">
                {ticket.subject}
              </span>
              <span className="text-muted-foreground mt-1 block truncate text-xs">
                #{ticket.ticketNumber} · {ticket.category}
              </span>
            </span>
            <Badge variant="secondary">{ticket.status}</Badge>
          </button>
        ))}
      </div>
    </ScrollArea>
  )
}

function CreateTicketPane(props: {
  onBack: () => void
  onCreated: (id: string) => void
}) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const form = useForm<TicketForm>({
    resolver: zodResolver(ticketSchema),
    defaultValues: { type: 'Support', subject: '', content: '' },
  })
  const createTicket = useMutation({
    mutationFn: createSupportTicket,
    onSuccess: async (ticket) => {
      form.reset()
      await queryClient.invalidateQueries({ queryKey: ['support', 'tickets'] })
      props.onCreated(ticket.id)
      toast.success(t('Ticket created'))
    },
    onError: (error) => handleServerError(error),
  })

  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <div className="flex h-14 shrink-0 items-center gap-2 border-b px-4">
        <Button variant="ghost" size="icon-sm" onClick={props.onBack}>
          <ArrowLeft aria-hidden="true" />
          <span className="sr-only">{t('Back to tickets')}</span>
        </Button>
        <div>
          <h2 className="font-semibold">{t('Create a ticket')}</h2>
          <p className="text-muted-foreground text-xs">
            {t('Use billing and invoice for invoice-related requests.')}
          </p>
        </div>
      </div>
      <ScrollArea className="min-h-0 flex-1">
        <form
          className="mx-auto flex w-full max-w-2xl flex-col gap-5 p-5 sm:p-8"
          onSubmit={form.handleSubmit((values) => createTicket.mutate(values))}
        >
          <div className="flex flex-col gap-2">
            <Label htmlFor="ticket-type">{t('Ticket type')}</Label>
            <Controller
              control={form.control}
              name="type"
              render={({ field }) => (
                <Select value={field.value} onValueChange={field.onChange}>
                  <SelectTrigger id="ticket-type" className="w-full">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectGroup>
                      <SelectItem value="Support">{t('Support')}</SelectItem>
                      <SelectItem value="Billing & Invoice">
                        {t('Billing & Invoice')}
                      </SelectItem>
                    </SelectGroup>
                  </SelectContent>
                </Select>
              )}
            />
          </div>
          <div className="flex flex-col gap-2">
            <Label htmlFor="ticket-subject">{t('Subject')}</Label>
            <Input id="ticket-subject" {...form.register('subject')} />
            {form.formState.errors.subject && (
              <p className="text-destructive text-xs">
                {t('Enter a subject between 3 and 200 characters.')}
              </p>
            )}
          </div>
          <div className="flex flex-col gap-2">
            <Label htmlFor="ticket-content">{t('Description')}</Label>
            <Textarea
              id="ticket-content"
              rows={10}
              {...form.register('content')}
            />
            {form.formState.errors.content && (
              <p className="text-destructive text-xs">
                {t('Enter at least 10 characters.')}
              </p>
            )}
          </div>
          <div className="flex justify-end">
            <Button type="submit" disabled={createTicket.isPending}>
              {createTicket.isPending ? t('Submitting...') : t('Submit ticket')}
            </Button>
          </div>
        </form>
      </ScrollArea>
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
  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <div className="flex min-h-14 shrink-0 items-center gap-2 border-b px-4 py-2.5">
        <Button variant="ghost" size="icon-sm" onClick={props.onBack}>
          <ArrowLeft aria-hidden="true" />
          <span className="sr-only">{t('Back to tickets')}</span>
        </Button>
        <div className="min-w-0 flex-1">
          <h2 className="truncate font-semibold">
            {props.data.ticket.subject}
          </h2>
          <p className="text-muted-foreground truncate text-xs">
            #{props.data.ticket.ticketNumber} · {props.data.ticket.category}
          </p>
        </div>
        <Badge variant="secondary">{props.data.ticket.status}</Badge>
      </div>

      <ScrollArea className="min-h-0 flex-1">
        <div className="mx-auto w-full max-w-3xl px-5">
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
        </div>
      </ScrollArea>

      <div className="shrink-0 border-t p-4">
        <div className="mx-auto flex w-full max-w-3xl flex-col gap-2">
          <Label htmlFor="ticket-reply">{t('Reply')}</Label>
          <Textarea
            id="ticket-reply"
            rows={4}
            value={props.reply}
            onChange={(event) => props.onReplyChange(event.target.value)}
          />
          <div className="flex justify-end">
            <Button
              onClick={props.onSend}
              disabled={!props.reply.trim() || props.sending}
            >
              {props.sending ? t('Sending...') : t('Send reply')}
            </Button>
          </div>
        </div>
      </div>
    </div>
  )
}

function Message(props: { sender: string; content: string }) {
  return (
    <article className="border-b py-5 last:border-b-0">
      <p className="text-muted-foreground mb-2 text-xs">{props.sender}</p>
      <p className="text-sm leading-relaxed whitespace-pre-wrap">
        {props.content}
      </p>
    </article>
  )
}
