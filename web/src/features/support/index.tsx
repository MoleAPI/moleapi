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
import { ExternalLink, FileText, LifeBuoy, MessageSquare } from 'lucide-react'
import { useState } from 'react'
import { Controller, useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { EmptyState } from '@/components/empty-state'
import { SectionPageLayout } from '@/components/layout'
import { LoadingState } from '@/components/loading-state'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Textarea } from '@/components/ui/textarea'
import { useIsAdmin } from '@/hooks/use-admin'
import { handleServerError } from '@/lib/handle-server-error'

import {
  createSupportTicket,
  getSupportConfig,
  getSupportTicket,
  getSupportTickets,
  replySupportTicket,
  ticketSchema,
} from './api'

type TicketForm = import('zod').infer<typeof ticketSchema>

export function Support() {
  const { t } = useTranslation()
  const isAdmin = useIsAdmin()
  const queryClient = useQueryClient()
  const [selectedTicket, setSelectedTicket] = useState<string | null>(null)
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
  const form = useForm<TicketForm>({
    resolver: zodResolver(ticketSchema),
    defaultValues: { type: 'Support', subject: '', content: '' },
  })
  const createTicket = useMutation({
    mutationFn: createSupportTicket,
    onSuccess: async (ticket) => {
      form.reset()
      setSelectedTicket(ticket.id)
      await queryClient.invalidateQueries({ queryKey: ['support', 'tickets'] })
      toast.success(t('Ticket created'))
    },
    onError: (error) => handleServerError(error),
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
    ([, href]) => href
  )

  return (
    <SectionPageLayout>
      <SectionPageLayout.Title>
        {t('Support & Community')}
      </SectionPageLayout.Title>
      <SectionPageLayout.Content>
        <div className='mx-auto grid w-full max-w-7xl gap-4 lg:grid-cols-[minmax(0,1fr)_minmax(20rem,0.8fr)]'>
          <div className='space-y-4'>
            <Card>
              <CardHeader>
                <CardTitle>{t('Community')}</CardTitle>
                <CardDescription>
                  {t('Join a community for announcements and peer support.')}
                </CardDescription>
              </CardHeader>
              <CardContent className='flex flex-wrap gap-2'>
                {links.length === 0 ? (
                  <span className='text-muted-foreground text-sm'>
                    {t('Community links are coming soon.')}
                  </span>
                ) : (
                  links.map(([name, href]) => (
                    <Button
                      key={name}
                      variant='outline'
                      render={
                        <a href={href} target='_blank' rel='noreferrer' />
                      }
                    >
                      {name === 'qq'
                        ? 'QQ'
                        : name[0].toUpperCase() + name.slice(1)}
                      <ExternalLink aria-hidden='true' />
                    </Button>
                  ))
                )}
              </CardContent>
            </Card>

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
            {config.data?.enabled && !isAdmin && (
              <Card>
                <CardHeader>
                  <CardTitle>{t('Create a ticket')}</CardTitle>
                  <CardDescription>
                    {t('Use billing and invoice for invoice-related requests.')}
                  </CardDescription>
                </CardHeader>
                <CardContent>
                  <form
                    className='space-y-4'
                    onSubmit={form.handleSubmit((values) =>
                      createTicket.mutate(values)
                    )}
                  >
                    <div className='space-y-2'>
                      <Label htmlFor='ticket-type'>{t('Ticket type')}</Label>
                      <Controller
                        control={form.control}
                        name='type'
                        render={({ field }) => (
                          <Select
                            value={field.value}
                            onValueChange={field.onChange}
                          >
                            <SelectTrigger id='ticket-type' className='w-full'>
                              <SelectValue />
                            </SelectTrigger>
                            <SelectContent>
                              <SelectItem value='Support'>
                                {t('Support')}
                              </SelectItem>
                              <SelectItem value='Billing & Invoice'>
                                {t('Billing & Invoice')}
                              </SelectItem>
                            </SelectContent>
                          </Select>
                        )}
                      />
                    </div>
                    <div className='space-y-2'>
                      <Label htmlFor='ticket-subject'>{t('Subject')}</Label>
                      <Input
                        id='ticket-subject'
                        {...form.register('subject')}
                      />
                      {form.formState.errors.subject && (
                        <p className='text-destructive text-xs'>
                          {t('Enter a subject between 3 and 200 characters.')}
                        </p>
                      )}
                    </div>
                    <div className='space-y-2'>
                      <Label htmlFor='ticket-content'>{t('Description')}</Label>
                      <Textarea
                        id='ticket-content'
                        rows={6}
                        {...form.register('content')}
                      />
                      {form.formState.errors.content && (
                        <p className='text-destructive text-xs'>
                          {t('Enter at least 10 characters.')}
                        </p>
                      )}
                    </div>
                    <Button type='submit' disabled={createTicket.isPending}>
                      {createTicket.isPending
                        ? t('Submitting...')
                        : t('Submit ticket')}
                    </Button>
                  </form>
                </CardContent>
              </Card>
            )}
          </div>

          {config.data?.enabled && (
            <Card className='min-h-[32rem]'>
              <CardHeader>
                <CardTitle>
                  {isAdmin ? t('Ticket management') : t('My tickets')}
                </CardTitle>
                <CardDescription>
                  {isAdmin
                    ? t('Replying here also sends an email to the user.')
                    : t(
                        'Open a ticket to view messages and continue the conversation.'
                      )}
                </CardDescription>
              </CardHeader>
              <CardContent>
                <TicketListContent
                  loading={tickets.isLoading}
                  tickets={tickets.data ?? []}
                  selectedTicket={selectedTicket}
                  detail={detail.data}
                  reply={reply}
                  onSelect={setSelectedTicket}
                  onReplyChange={setReply}
                  onSend={() => sendReply.mutate(undefined)}
                  sending={sendReply.isPending}
                />
              </CardContent>
            </Card>
          )}
        </div>
      </SectionPageLayout.Content>
    </SectionPageLayout>
  )
}

function TicketListContent(props: {
  loading: boolean
  tickets: Awaited<ReturnType<typeof getSupportTickets>>
  selectedTicket: string | null
  detail: Awaited<ReturnType<typeof getSupportTicket>> | undefined
  reply: string
  onSelect: (id: string | null) => void
  onReplyChange: (value: string) => void
  onSend: () => void
  sending: boolean
}) {
  const { t } = useTranslation()
  if (props.loading) return <LoadingState />
  if (props.selectedTicket && props.detail) {
    return (
      <TicketDetail
        data={props.detail}
        reply={props.reply}
        onReplyChange={props.onReplyChange}
        onBack={() => props.onSelect(null)}
        onSend={props.onSend}
        sending={props.sending}
      />
    )
  }
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
    <div className='divide-y'>
      {props.tickets.map((ticket) => (
        <button
          key={ticket.id}
          type='button'
          className='hover:bg-muted/60 flex w-full items-start gap-3 rounded-lg px-2 py-3 text-left transition-colors'
          onClick={() => props.onSelect(ticket.id)}
        >
          <FileText className='text-muted-foreground mt-0.5 size-4 shrink-0' />
          <span className='min-w-0 flex-1'>
            <span className='block truncate font-medium'>{ticket.subject}</span>
            <span className='text-muted-foreground text-xs'>
              #{ticket.ticketNumber} · {ticket.category}
            </span>
          </span>
          <Badge variant='secondary'>{ticket.status}</Badge>
        </button>
      ))}
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
    <div className='space-y-4'>
      <Button variant='ghost' size='sm' onClick={props.onBack}>
        {t('Back to tickets')}
      </Button>
      <div>
        <div className='flex items-start justify-between gap-2'>
          <h2 className='font-medium'>{props.data.ticket.subject}</h2>
          <Badge variant='secondary'>{props.data.ticket.status}</Badge>
        </div>
        <p className='text-muted-foreground mt-1 text-xs'>
          #{props.data.ticket.ticketNumber} · {props.data.ticket.category}
        </p>
      </div>
      <div className='max-h-80 space-y-3 overflow-y-auto pr-1'>
        <div className='bg-muted rounded-lg p-3 text-sm whitespace-pre-wrap'>
          {props.data.ticket.description}
        </div>
        {props.data.conversations.map((message) => (
          <div key={message.id} className='border-border rounded-lg border p-3'>
            <p className='text-muted-foreground mb-1 text-xs'>
              {message.fromEmailAddress || message.direction || message.type}
            </p>
            <p className='text-sm whitespace-pre-wrap'>
              {message.content || message.summary}
            </p>
          </div>
        ))}
      </div>
      <div className='space-y-2'>
        <Label htmlFor='ticket-reply'>{t('Reply')}</Label>
        <Textarea
          id='ticket-reply'
          rows={4}
          value={props.reply}
          onChange={(event) => props.onReplyChange(event.target.value)}
        />
        <Button
          onClick={props.onSend}
          disabled={!props.reply.trim() || props.sending}
        >
          {props.sending ? t('Sending...') : t('Send reply')}
        </Button>
      </div>
    </div>
  )
}
