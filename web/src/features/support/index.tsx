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
  RefreshIcon,
  LinkSquare02Icon,
  Tick02Icon,
  RotateLeft01Icon,
} from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import {
  useInfiniteQuery,
  useMutation,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query'
import DOMPurify from 'dompurify'
import { useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { ConfirmDialog } from '@/components/confirm-dialog'
import { ErrorState } from '@/components/error-state'
import { HtmlContent } from '@/components/html-content'
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
import { Input } from '@/components/ui/input'
import {
  InputGroup,
  InputGroupAddon,
  InputGroupTextarea,
} from '@/components/ui/input-group'
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { ScrollArea } from '@/components/ui/scroll-area'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Spinner } from '@/components/ui/spinner'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'
import {
  Tooltip,
  TooltipTrigger,
  TooltipContent,
} from '@/components/ui/tooltip'
import { usePlaygroundState } from '@/features/playground/hooks'
import { useIsAdmin } from '@/hooks/use-admin'
import { toIntlLocale } from '@/i18n/languages'
import { handleServerError } from '@/lib/handle-server-error'
import { cn } from '@/lib/utils'
import { useAuthStore } from '@/stores/auth-store'

import {
  downloadSupportAttachment,
  getSupportConfig,
  getSupportTicket,
  getSupportTickets,
  replySupportTicket,
  uploadSupportAttachments,
  updateSupportTicketStatus,
  type SupportTicket,
} from './api'
import { AttachmentPicker } from './components/attachment-picker'
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
  const [replies, setReplies] = useState<Record<string, string>>({})
  const [replyFiles, setReplyFiles] = useState<Record<string, File[]>>({})
  const reply = selectedTicket ? (replies[selectedTicket] ?? '') : ''
  const setReply = (value: string) => {
    if (selectedTicket) {
      setReplies((current) => ({ ...current, [selectedTicket]: value }))
    }
  }
  const files = selectedTicket ? (replyFiles[selectedTicket] ?? []) : []
  const setFiles = (value: File[]) => {
    if (selectedTicket) {
      setReplyFiles((current) => ({ ...current, [selectedTicket]: value }))
    }
  }
  const config = useQuery({
    queryKey: ['support', 'config'],
    queryFn: getSupportConfig,
  })
  const tickets = useInfiniteQuery({
    queryKey: ['support', 'tickets'],
    queryFn: ({ pageParam }) => getSupportTickets(pageParam),
    initialPageParam: 0,
    getNextPageParam: (page) => (page.has_more ? page.next_from : undefined),
    staleTime: 60_000,
    enabled:
      config.data?.enabled === true && (isAdmin || Boolean(accountEmail)),
  })
  const [showRefreshLabel, setShowRefreshLabel] = useState(false)
  useEffect(() => {
    setShowRefreshLabel(false)
    const timer = window.setTimeout(() => setShowRefreshLabel(true), 60_000)
    return () => window.clearTimeout(timer)
  }, [tickets.dataUpdatedAt])
  const ticketList = tickets.data?.pages.flatMap((page) => page.tickets) ?? []
  const detail = useQuery({
    queryKey: ['support', 'ticket', selectedTicket],
    queryFn: () => {
      if (!selectedTicket) throw new Error('Ticket ID is required')
      return getSupportTicket(selectedTicket)
    },
    enabled: selectedTicket != null,
    staleTime: 30_000,
  })
  const sendReply = useMutation({
    mutationFn: async ({
      id,
      content,
      files,
    }: {
      id: string
      content: string
      files: File[]
    }) => {
      await replySupportTicket(id, content)
      let attachmentsFailed = false
      if (files.length > 0) {
        try {
          await uploadSupportAttachments(id, files)
        } catch {
          attachmentsFailed = true
        }
      }
      return { attachmentsFailed }
    },
    onSuccess: async ({ attachmentsFailed }, { id, content }) => {
      setReplies((current) =>
        current[id] === content ? { ...current, [id]: '' } : current
      )
      setReplyFiles((current) => ({ ...current, [id]: [] }))
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ['support', 'ticket', id] }),
        queryClient.invalidateQueries({ queryKey: ['support', 'tickets'] }),
      ])
      toast[attachmentsFailed ? 'warning' : 'success'](
        attachmentsFailed
          ? t('Reply sent, but attachments could not be uploaded.')
          : t('Reply sent')
      )
    },
    onError: (error) => handleServerError(error),
  })

  return (
    <SectionPageLayout fixedContent>
      <SectionPageLayout.Title>{t('Support center')}</SectionPageLayout.Title>
      <SectionPageLayout.Actions>
        <Tooltip>
          <TooltipTrigger
            render={
              <Button
                variant='outline'
                size={showRefreshLabel ? 'sm' : 'icon-sm'}
                disabled={tickets.isFetching || detail.isFetching}
                onClick={() => {
                  setShowRefreshLabel(false)
                  void queryClient.invalidateQueries({ queryKey: ['support'] })
                }}
                aria-label={t('Refresh')}
              />
            }
          >
            <HugeiconsIcon icon={RefreshIcon} />
            {showRefreshLabel && <span>{t('Refresh replies')}</span>}
          </TooltipTrigger>
          <TooltipContent>
            {t('Refresh to check for support replies')}
          </TooltipContent>
        </Tooltip>
        <CommunityChannels links={config.data?.community_links ?? {}} />
      </SectionPageLayout.Actions>
      <SectionPageLayout.Content>
        <div className='flex h-full min-h-0 w-full flex-col gap-4'>
          {config.isLoading && <LoadingState />}
          {config.isError && (
            <ErrorState onRetry={() => void config.refetch()} />
          )}
          {!config.isLoading && !config.isError && !config.data?.enabled && (
            <Alert>
              <AlertTitle>{t('Ticket service is being prepared')}</AlertTitle>
              <AlertDescription>
                {isAdmin
                  ? t('Complete the Zoho Desk settings to enable tickets.')
                  : t('Please use the community or support email for now.')}
              </AlertDescription>
            </Alert>
          )}

          {config.data?.enabled && tickets.isError && (
            <ErrorState onRetry={() => void tickets.refetch()} />
          )}
          {config.data?.enabled && isAdmin && !tickets.isError && (
            <div className='min-h-0 flex-1 overflow-hidden border-y'>
              <TicketBrowser
                loading={tickets.isLoading}
                tickets={ticketList}
                selectedTicket={selectedTicket}
                onSelect={setSelectedTicket}
                detail={detail.data}
                detailLoading={detail.isLoading}
                detailError={detail.isError}
                onRetry={() => void detail.refetch()}
                reply={reply}
                onReplyChange={setReply}
                onSend={() =>
                  selectedTicket &&
                  sendReply.mutate({
                    id: selectedTicket,
                    content: reply,
                    files,
                  })
                }
                files={files}
                onFilesChange={setFiles}
                sending={sendReply.isPending}
              />
            </div>
          )}

          {config.data?.enabled && !isAdmin && !tickets.isError && (
            <div className='min-h-0 flex-1 overflow-hidden border-y'>
              <UserSupportWorkspace
                accountEmail={accountEmail}
                loading={tickets.isLoading}
                tickets={ticketList}
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
                detailError={detail.isError}
                onRetry={() => void detail.refetch()}
                reply={reply}
                onReplyChange={setReply}
                onSend={() =>
                  selectedTicket &&
                  sendReply.mutate({
                    id: selectedTicket,
                    content: reply,
                    files,
                  })
                }
                files={files}
                onFilesChange={setFiles}
                sending={sendReply.isPending}
              />
            </div>
          )}
          {tickets.hasNextPage && (
            <Button
              variant='ghost'
              size='sm'
              disabled={tickets.isFetchingNextPage}
              onClick={() => void tickets.fetchNextPage()}
            >
              {t('Load more tickets')}
            </Button>
          )}
        </div>
      </SectionPageLayout.Content>
    </SectionPageLayout>
  )
}

function UserSupportWorkspace(props: {
  accountEmail?: string
  loading: boolean
  tickets: SupportTicket[]
  selectedTicket: string | null
  onSelectTicket: (id: string | null) => void
  panel: 'overview' | 'ai' | 'create'
  onPanelChange: (panel: 'overview' | 'ai' | 'create') => void
  draft: AssistantTicketDraft | null
  onAssistantDraft: (draft: AssistantTicketDraft) => void
  detail: Awaited<ReturnType<typeof getSupportTicket>> | undefined
  detailLoading: boolean
  detailError: boolean
  onRetry: () => void
  reply: string
  onReplyChange: (value: string) => void
  files?: File[]
  onFilesChange?: (files: File[]) => void
  onSend: () => void
  sending: boolean
}) {
  const { t } = useTranslation()
  const assistantState = usePlaygroundState('support')
  const [assistantFiles, setAssistantFiles] = useState<File[]>([])
  const [assistantBusy, setAssistantBusy] = useState(false)
  const [sidebarView, setSidebarView] = useState<'ai' | 'tickets'>('tickets')
  const showingPanel =
    Boolean(props.selectedTicket) || props.panel !== 'overview'
  const goBack = () => {
    setSidebarView('tickets')
    props.onSelectTicket(null)
    props.onPanelChange('overview')
  }
  const startNewAIConversation = () => {
    assistantState.createConversation()
    assistantState.updateConfig('model', 'deepseek-flash')
    setAssistantFiles([])
    setSidebarView('ai')
    props.onPanelChange('ai')
  }

  return (
    <div className='grid h-full min-h-0 grid-cols-[minmax(0,1fr)] overflow-hidden md:grid-cols-[minmax(16rem,20rem)_minmax(0,1fr)]'>
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
            onClick={startNewAIConversation}
          >
            <HugeiconsIcon icon={AiChat02Icon} data-icon='inline-start' />
            {t('Ask AI')}
          </Button>
          <Button
            disabled={assistantBusy}
            onClick={() => {
              setSidebarView('tickets')
              props.onPanelChange('create')
            }}
          >
            <HugeiconsIcon icon={Add01Icon} data-icon='inline-start' />
            {t('Create ticket')}
          </Button>
        </div>
        <div className='border-b p-3'>
          <Tabs
            value={sidebarView}
            onValueChange={(value) => setSidebarView(value as 'ai' | 'tickets')}
          >
            <TabsList className='w-full'>
              <TabsTrigger value='ai' className='flex-1'>
                {t('AI conversations')}
              </TabsTrigger>
              <TabsTrigger value='tickets' className='flex-1 gap-1.5'>
                {t('My tickets')}
                <Badge variant='secondary' className='h-5 min-w-5 px-1.5'>
                  {props.tickets.length}
                </Badge>
              </TabsTrigger>
            </TabsList>
          </Tabs>
        </div>
        <ScrollArea className='min-h-0 flex-1'>
          {sidebarView === 'ai' && (
            <div className='py-2'>
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
                      setSidebarView('ai')
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
              {assistantState.sessions.every(
                (session) => session.messages.length === 0
              ) && (
                <div className='text-muted-foreground px-4 py-8 text-center text-sm'>
                  {t('No AI conversations yet')}
                </div>
              )}
            </div>
          )}
          {sidebarView === 'tickets' && props.loading && <LoadingState />}
          {sidebarView === 'tickets' &&
            !props.loading &&
            props.tickets.length === 0 && (
              <div className='text-muted-foreground px-4 py-8 text-center text-sm'>
                {t('No tickets yet')}
              </div>
            )}
          {sidebarView === 'tickets' &&
            !props.loading &&
            props.tickets.length > 0 && (
              <div>
                <TicketList
                  tickets={props.tickets}
                  selectedTicket={props.selectedTicket}
                  onSelect={props.onSelectTicket}
                  disabled={assistantBusy}
                />
              </div>
            )}
        </ScrollArea>
      </aside>

      <section
        className={cn(
          'min-h-0 min-w-0 flex-col',
          showingPanel ? 'flex' : 'hidden md:flex'
        )}
      >
        {!showingPanel && (
          <SupportOverview
            onAskAI={() => {
              startNewAIConversation()
            }}
            onCreateTicket={() => {
              setSidebarView('tickets')
              props.onPanelChange('create')
            }}
          />
        )}
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
            onCreateBlankTicket={() => {
              setSidebarView('tickets')
              props.onPanelChange('create')
            }}
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
        {props.selectedTicket && props.detailError && (
          <ErrorState onRetry={props.onRetry} />
        )}
        {props.selectedTicket && props.detailLoading && <LoadingState />}
        {props.selectedTicket && !props.detailError && props.detail && (
          <TicketDetail
            key={props.detail.ticket.id}
            data={props.detail}
            reply={props.reply}
            onReplyChange={props.onReplyChange}
            files={props.files ?? []}
            onFilesChange={props.onFilesChange ?? (() => undefined)}
            onBack={goBack}
            onSend={props.onSend}
            sending={props.sending}
          />
        )}
      </section>
    </div>
  )
}

function SupportOverview(props: {
  onAskAI: () => void
  onCreateTicket: () => void
}) {
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
        <div className='mt-6 flex flex-wrap gap-2'>
          <Button onClick={props.onAskAI}>
            <HugeiconsIcon icon={AiChat02Icon} data-icon='inline-start' />
            {t('Ask AI')}
          </Button>
          <Button variant='outline' onClick={props.onCreateTicket}>
            <HugeiconsIcon icon={Add01Icon} data-icon='inline-start' />
            {t('Create ticket')}
          </Button>
        </div>
      </div>
    </div>
  )
}

export function TicketBrowser(props: {
  loading: boolean
  tickets: SupportTicket[]
  selectedTicket: string | null
  onSelect: (id: string | null) => void
  detail: Awaited<ReturnType<typeof getSupportTicket>> | undefined
  detailLoading: boolean
  detailError: boolean
  onRetry: () => void
  reply: string
  onReplyChange: (value: string) => void
  files?: File[]
  onFilesChange?: (files: File[]) => void
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
    <div className='grid h-full min-h-0 grid-cols-[minmax(0,1fr)] overflow-hidden md:grid-cols-[minmax(16rem,22rem)_minmax(0,1fr)]'>
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
          <TicketList
            tickets={props.tickets}
            selectedTicket={props.selectedTicket}
            onSelect={props.onSelect}
            admin
          />
        </ScrollArea>
      </aside>

      <section
        className={cn(
          'min-h-0 min-w-0 flex-col',
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
        {props.selectedTicket && props.detailError && (
          <ErrorState onRetry={props.onRetry} />
        )}
        {props.selectedTicket && props.detailLoading && <LoadingState />}
        {props.selectedTicket && !props.detailError && props.detail && (
          <TicketDetail
            key={props.detail.ticket.id}
            data={props.detail}
            reply={props.reply}
            onReplyChange={props.onReplyChange}
            files={props.files ?? []}
            onFilesChange={props.onFilesChange ?? (() => undefined)}
            onBack={() => props.onSelect(null)}
            onSend={props.onSend}
            sending={props.sending}
          />
        )}
      </section>
    </div>
  )
}

function isTicketClosed(ticket: SupportTicket) {
  return (ticket.statusType || ticket.status) === 'Closed'
}

function ticketStatusLabel(ticket: SupportTicket, t: (key: string) => string) {
  if (ticket.status === 'On Hold') return t('Waiting')
  if (ticket.status === 'Open') {
    return ticket.activity === 'agent' ? t('In progress') : t('Unprocessed')
  }
  return t(ticket.status)
}

function ticketActivityLabel(
  ticket: SupportTicket,
  t: (key: string) => string
) {
  if (ticket.activity === 'customer') return t('Customer replied')
  if (ticket.activity === 'agent') return t('Support replied')
  if (ticket.activity === 'new') return t('New ticket')
  return null
}

function formatSupportDate(value: string | undefined, language: string) {
  if (!value) return ''
  return new Date(value).toLocaleString(toIntlLocale(language), {
    year: 'numeric',
    month: 'numeric',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
  })
}

function TicketList(props: {
  tickets: SupportTicket[]
  selectedTicket: string | null
  onSelect: (id: string) => void
  admin?: boolean
  disabled?: boolean
}) {
  const { t, i18n } = useTranslation()
  const [filter, setFilter] = useState('active')
  const [search, setSearch] = useState('')
  const needsReply = (ticket: SupportTicket) =>
    !isTicketClosed(ticket) &&
    (ticket.activity === 'customer' || ticket.activity === 'new')
  const visible = props.tickets
    .filter((ticket) => {
      if (filter === 'active' && isTicketClosed(ticket)) return false
      if (filter === 'attention' && !needsReply(ticket)) return false
      if (filter === 'closed' && !isTicketClosed(ticket)) return false
      return [
        ticket.subject,
        ticket.ticketNumber,
        ticket.email,
        ticket.user?.username,
        ticket.user?.id,
      ]
        .join(' ')
        .toLocaleLowerCase()
        .includes(search.trim().toLocaleLowerCase())
    })
    .sort((a, b) => {
      if (props.admin && needsReply(a) !== needsReply(b)) {
        return Number(needsReply(b)) - Number(needsReply(a))
      }
      return (
        Date.parse(b.modifiedTime || b.createdTime) -
        Date.parse(a.modifiedTime || a.createdTime)
      )
    })
  return (
    <>
      <div className='space-y-2 border-b p-3'>
        <Input
          value={search}
          onChange={(event) => setSearch(event.target.value)}
          placeholder={t('Search tickets')}
          aria-label={t('Search tickets')}
        />
        <Tabs value={filter} onValueChange={setFilter}>
          <TabsList className='w-full flex-wrap group-data-horizontal/tabs:h-auto'>
            <TabsTrigger value='active' className='h-7 flex-1'>
              {t('Unresolved')}
            </TabsTrigger>
            {props.admin && (
              <TabsTrigger value='attention' className='h-7 flex-1'>
                {t('Needs reply')}
              </TabsTrigger>
            )}
            <TabsTrigger value='closed' className='h-7 flex-1'>
              {t('Closed')}
            </TabsTrigger>
            <TabsTrigger value='all' className='h-7 flex-1'>
              {t('All')}
            </TabsTrigger>
          </TabsList>
        </Tabs>
      </div>
      <div className='divide-y'>
        {visible.map((ticket) => (
          <button
            key={ticket.id}
            type='button'
            disabled={props.disabled}
            aria-current={
              props.selectedTicket === ticket.id ? 'page' : undefined
            }
            onClick={() => props.onSelect(ticket.id)}
            className={cn(
              'hover:bg-muted/60 grid w-full grid-cols-[minmax(0,1fr)_auto] gap-x-3 gap-y-2 px-4 py-3.5 text-left transition-colors',
              props.selectedTicket === ticket.id && 'bg-muted',
              props.admin &&
                needsReply(ticket) &&
                'border-l-2 border-amber-500 bg-amber-500/5'
            )}
          >
            <span className='col-span-2 flex w-full min-w-0 items-start gap-2'>
              {props.admin && needsReply(ticket) && (
                <span
                  className='size-2 shrink-0 rounded-full bg-amber-500'
                  aria-label={t('Needs reply')}
                />
              )}
              <span className='min-w-0 flex-1 truncate text-sm font-medium'>
                {ticket.subject}
              </span>
              <span className='flex shrink-0 flex-col items-end gap-1'>
                <Badge variant='outline'>{ticketStatusLabel(ticket, t)}</Badge>
                {ticketActivityLabel(ticket, t) && (
                  <Badge
                    variant='secondary'
                    className={cn(
                      'h-5 px-1.5 text-[10px] font-normal',
                      needsReply(ticket) &&
                        'bg-amber-500/15 text-amber-700 dark:text-amber-400'
                    )}
                  >
                    {ticketActivityLabel(ticket, t)}
                  </Badge>
                )}
              </span>
            </span>
            <span className='text-muted-foreground col-span-2 flex w-full min-w-0 flex-wrap gap-x-2 text-xs'>
              <span className='min-w-0 truncate'>
                #{ticket.ticketNumber} · {t(ticket.category)}
              </span>
              {props.admin && (
                <span className='min-w-0 truncate'>
                  {ticket.user
                    ? `${ticket.user.username} · ID ${ticket.user.id}`
                    : null}
                </span>
              )}
              <time
                className='ml-auto text-right'
                dateTime={ticket.modifiedTime || ticket.createdTime}
              >
                {formatSupportDate(
                  ticket.modifiedTime || ticket.createdTime,
                  i18n.language
                )}
              </time>
            </span>
          </button>
        ))}
        {visible.length === 0 && (
          <p className='text-muted-foreground px-4 py-8 text-center text-sm'>
            {t('No matching tickets')}
          </p>
        )}
      </div>
    </>
  )
}

export function TicketDetail(props: {
  data: Awaited<ReturnType<typeof getSupportTicket>>
  reply: string
  onReplyChange: (value: string) => void
  files?: File[]
  onFilesChange?: (files: File[]) => void
  onBack: () => void
  onSend: () => void
  sending: boolean
}) {
  const { t, i18n } = useTranslation()
  const isAdmin = useIsAdmin()
  const queryClient = useQueryClient()
  const [closing, setClosing] = useState(false)
  const [preview, setPreview] = useState<{
    filename: string
    url: string
  } | null>(null)
  const ticket = props.data.ticket
  const closed = isTicketClosed(ticket)
  const status = useMutation({
    mutationFn: (value: string) => updateSupportTicketStatus(ticket.id, value),
    onSuccess: async () => {
      setClosing(false)
      await Promise.all([
        queryClient.invalidateQueries({
          queryKey: ['support', 'ticket', ticket.id],
        }),
        queryClient.invalidateQueries({ queryKey: ['support', 'tickets'] }),
      ])
      toast.success(t('Ticket status updated'))
    },
    onError: handleServerError,
  })
  const statusOptions = [
    { value: 'Open', label: ticketStatusLabel(ticket, t) },
    { value: 'On Hold', label: t('Waiting') },
    { value: 'Closed', label: t('Closed') },
  ]
  if (!statusOptions.some((option) => option.value === ticket.status)) {
    statusOptions.push({ value: ticket.status, label: ticket.status })
  }
  const conversations = [...props.data.conversations].sort(
    (a, b) =>
      Date.parse(a.createdTime || a.commentedTime || '') -
      Date.parse(b.createdTime || b.commentedTime || '')
  )
  const hasInitialConversation = conversations.some(
    (message) =>
      normalizeSupportContent(message.content || message.summary) ===
      normalizeSupportContent(ticket.description)
  )
  const downloadAttachment = async (
    attachment: (typeof props.data.attachments)[number]
  ) => {
    try {
      const file = await downloadSupportAttachment(
        props.data.ticket.id,
        attachment
      )
      if (/\.(gif|jpe?g|png|webp)$/i.test(attachment.name)) {
        setPreview((current) => {
          if (current) URL.revokeObjectURL(current.url)
          return file
        })
        return
      }
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
      <div className='flex shrink-0 flex-wrap items-center gap-x-3 gap-y-2 border-b px-3 py-2.5 sm:px-4'>
        <Button variant='ghost' size='icon-sm' onClick={props.onBack}>
          <HugeiconsIcon icon={ArrowLeft01Icon} />
          <span className='sr-only'>{t('Back to tickets')}</span>
        </Button>
        <div className='min-w-0 flex-1'>
          <h2 className='truncate font-semibold'>
            {props.data.ticket.subject}
          </h2>
          <p className='text-muted-foreground flex flex-wrap gap-x-2 text-xs'>
            #{props.data.ticket.ticketNumber} · {t(props.data.ticket.category)}{' '}
            · {formatSupportDate(ticket.createdTime, i18n.language)}
          </p>
        </div>
        <div className='flex min-w-0 flex-wrap items-center justify-end gap-2'>
          {isAdmin && ticket.user && (
            <span className='text-muted-foreground text-xs'>
              {ticket.user.username} · ID {ticket.user.id}
            </span>
          )}
          <span
            className='text-muted-foreground max-w-56 truncate text-xs'
            title={ticket.email}
          >
            {ticket.email}
          </span>
          <Badge variant='secondary' className='shrink-0'>
            {ticketStatusLabel(ticket, t)}
          </Badge>
          {isAdmin && ticket.user && (
            <Button
              variant='outline'
              size='sm'
              role='link'
              render={
                <a
                  href={`/usage-logs/common?username=${encodeURIComponent(ticket.user.username)}`}
                  target='_blank'
                  rel='noopener noreferrer'
                />
              }
              nativeButton={false}
            >
              <HugeiconsIcon icon={LinkSquare02Icon} data-icon='inline-start' />
              {t('User logs')}
            </Button>
          )}
          {isAdmin && (
            <Select
              items={statusOptions}
              value={ticket.status}
              disabled={status.isPending || props.sending}
              onValueChange={(value) => {
                if (!value || value === ticket.status) return
                if (value === 'Closed') setClosing(true)
                else status.mutate(value)
              }}
            >
              <SelectTrigger
                size='sm'
                aria-label={t('Ticket status')}
                className='w-32'
              >
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectGroup>
                  {statusOptions.map((option) => (
                    <SelectItem key={option.value} value={option.value}>
                      {option.label}
                    </SelectItem>
                  ))}
                </SelectGroup>
              </SelectContent>
            </Select>
          )}
          {!isAdmin && !closed && (
            <Button
              size='sm'
              className='bg-primary text-primary-foreground hover:bg-primary/90'
              disabled={status.isPending || props.sending}
              onClick={() => setClosing(true)}
            >
              <HugeiconsIcon icon={Tick02Icon} data-icon='inline-start' />
              {t('Close ticket')}
            </Button>
          )}
        </div>
      </div>

      <ScrollArea className='min-h-0 flex-1'>
        <div className='w-full px-4 sm:px-6 lg:px-8'>
          {!hasInitialConversation && (
            <Message
              sender={props.data.ticket.email}
              content={props.data.ticket.description}
              html
              time={ticket.createdTime}
              customer
            />
          )}
          {conversations.map((message) => (
              <Message
                key={message.id}
                sender={
                  message.fromEmailAddress ||
                  message.author?.name ||
                  message.commenter?.name ||
                  t('Support')
                }
                content={message.content || message.summary}
                html={
                  message.contentType === 'text/html' ||
                  message.type === 'comment'
                }
                time={message.createdTime || message.commentedTime}
                internal={
                  message.type === 'comment'
                    ? !message.isPublic
                    : message.visibility !== 'public'
                }
                customer={
                  message.direction === 'in' ||
                  message.author?.type === 'END_USER' ||
                  message.commenter?.type === 'END_USER'
                }
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
                  <span className='max-w-48 truncate'>{attachment.name}</span>
                </Button>
              ))}
            </div>
          )}
        </div>
      </ScrollArea>

      {closed ? (
        <div className='flex shrink-0 flex-wrap items-center justify-between gap-2 border-t p-4 text-sm'>
          <p className='text-muted-foreground'>{t('This ticket is closed.')}</p>
          <Button
            variant='outline'
            size='sm'
            disabled={status.isPending}
            onClick={() => status.mutate('Open')}
          >
            <HugeiconsIcon icon={RotateLeft01Icon} data-icon='inline-start' />
            {t('Reopen ticket')}
          </Button>
        </div>
      ) : (
        <div className='shrink-0 border-t p-3 sm:p-4'>
          <InputGroup className='bg-background overflow-hidden'>
            <InputGroupTextarea
              id='ticket-reply'
              aria-label={t('Reply')}
              rows={3}
              maxLength={10000}
              disabled={props.sending || status.isPending}
              value={props.reply}
              onChange={(event) => props.onReplyChange(event.target.value)}
            />
            <InputGroupAddon
              align='block-end'
              className='bg-muted/20 flex-wrap justify-between gap-2 border-t px-2 py-2'
            >
              <AttachmentPicker
                compact
                files={props.files ?? []}
                onFilesChange={props.onFilesChange ?? (() => undefined)}
                disabled={props.sending || status.isPending}
                onRejected={(fileName) =>
                  toast.error(
                    t('{{file}} exceeds the 5 MB attachment limit.', {
                      file: fileName,
                    })
                  )
                }
              />
              <Button
                onClick={props.onSend}
                disabled={
                  !props.reply.trim() || props.sending || status.isPending
                }
              >
                {props.sending && <Spinner data-icon='inline-start' />}
                {props.sending ? t('Sending...') : t('Send reply')}
              </Button>
            </InputGroupAddon>
          </InputGroup>
        </div>
      )}
      <ConfirmDialog
        open={closing}
        onOpenChange={(open) => {
          if (!status.isPending) setClosing(open)
        }}
        title={t('Close ticket')}
        desc={t('Close this ticket? You can reopen it if the issue returns.')}
        confirmText={t('Close ticket')}
        isLoading={status.isPending}
        handleConfirm={() => status.mutate('Closed')}
      />
      <Dialog
        open={preview != null}
        onOpenChange={(open) => {
          if (!open && preview) {
            URL.revokeObjectURL(preview.url)
            setPreview(null)
          }
        }}
      >
        <DialogContent className='max-w-5xl'>
          <DialogHeader>
            <DialogTitle>{preview?.filename}</DialogTitle>
          </DialogHeader>
          {preview && (
            <img
              src={preview.url}
              alt={preview.filename}
              className='max-h-[75vh] w-full object-contain'
            />
          )}
        </DialogContent>
      </Dialog>
    </div>
  )
}

function normalizeSupportContent(value: string) {
  return value
    .replace(/<[^>]+>/g, ' ')
    .replace(/\s+/g, ' ')
    .trim()
}

function Message(props: {
  sender: string
  content: string
  html?: boolean
  time?: string
  internal?: boolean
  customer?: boolean
}) {
  const { t, i18n } = useTranslation()
  const safeContent = useMemo(
    () =>
      props.html
        ? DOMPurify.sanitize(props.content, {
            ALLOWED_TAGS: [
              'p',
              'br',
              'div',
              'span',
              'strong',
              'b',
              'em',
              'i',
              'u',
              's',
              'blockquote',
              'pre',
              'code',
              'ul',
              'ol',
              'li',
              'a',
              'table',
              'thead',
              'tbody',
              'tr',
              'th',
              'td',
            ],
            ALLOWED_ATTR: ['href', 'title'],
            ALLOW_DATA_ATTR: false,
          })
        : props.content,
    [props.content, props.html]
  )
  return (
    <div
      className={cn(
        'flex border-b py-5 last:border-b-0',
        props.customer ? 'justify-end' : 'justify-start'
      )}
    >
      <article
        className={cn(
          'w-fit max-w-[88%] rounded-2xl px-4 py-3',
          props.customer ? 'bg-primary text-primary-foreground' : 'bg-muted'
        )}
      >
        <div
          className={cn(
            'mb-2 flex flex-wrap items-center gap-2 text-xs',
            props.customer
              ? 'text-primary-foreground/75 justify-end'
              : 'text-muted-foreground'
          )}
        >
          <span className='break-all'>{props.sender}</span>
          {props.internal && (
            <Badge variant='outline'>{t('Internal note')}</Badge>
          )}
          {props.time && (
            <time dateTime={props.time}>
              {formatSupportDate(props.time, i18n.language)}
            </time>
          )}
        </div>
        {props.html ? (
          <HtmlContent
            content={safeContent}
            className='overflow-x-auto text-sm leading-relaxed break-words whitespace-pre-wrap'
          />
        ) : (
          <p className='text-sm leading-relaxed break-words whitespace-pre-wrap'>
            {props.content}
          </p>
        )}
      </article>
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
