/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import { useQuery } from '@tanstack/react-query'
import { ChevronDown, ChevronRight } from 'lucide-react'
import { Fragment, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { ErrorState } from '@/components/error-state'
import { LoadingState } from '@/components/loading-state'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { toIntlLocale } from '@/i18n/languages'
import { formatNumber, formatTimestampToDate } from '@/lib/format'

import { getChannelModelHealth } from '../api'
import type { ChannelModelHealth } from '../types'

type ChannelModelHealthTableProps = {
  channelId: number
  models: string[]
  onDelete: (model: string) => void
}

export function ChannelModelHealthTable(props: ChannelModelHealthTableProps) {
  const { t, i18n } = useTranslation()
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  const [expandedModel, setExpandedModel] = useState<string | null>(null)
  const query = useQuery({
    queryKey: ['channels', 'model-health', props.channelId],
    queryFn: () => getChannelModelHealth(props.channelId),
    enabled: props.channelId > 0,
    refetchInterval: 60_000,
  })

  if (query.isLoading) {
    return <LoadingState className='min-h-28' size='sm' />
  }
  if (query.isError) {
    return (
      <ErrorState
        className='min-h-28'
        description={t('Failed to load model health')}
      />
    )
  }

  const saved = new Map(
    (query.data?.data?.models ?? []).map((item) => [item.model, item])
  )
  const rows = props.models.map(
    (model): ChannelModelHealth =>
      saved.get(model) ?? {
        model,
        status: 'available',
        today: { date: '', requests: 0, failures: 0, failure_rate: 0 },
        days: [],
      }
  )

  return (
    <div className='border-border/60 space-y-3 rounded-lg border p-4'>
      <div>
        <h3 className='text-sm font-medium'>{t('Channel model health')}</h3>
        <p className='text-muted-foreground text-xs'>
          {t('Current status and request failures for each model.')}
        </p>
      </div>
      {!query.data?.data?.history_available && (
        <p className='text-muted-foreground text-xs'>
          {t('Request history is unavailable because error logs are disabled.')}
        </p>
      )}
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead>{t('Model')}</TableHead>
            <TableHead>{t('Status')}</TableHead>
            <TableHead>{t('Today failures')}</TableHead>
            <TableHead>{t('Failure rate')}</TableHead>
            <TableHead>{t('Last failure')}</TableHead>
            <TableHead className='text-right'>{t('Actions')}</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {rows.map((item) => {
            const expanded = expandedModel === item.model
            return (
              <Fragment key={item.model}>
                <TableRow>
                  <TableCell className='max-w-64 font-mono wrap-anywhere whitespace-normal'>
                    <Button
                      type='button'
                      variant='ghost'
                      size='sm'
                      className='h-auto max-w-full justify-start px-0 font-mono'
                      aria-expanded={expanded}
                      onClick={() =>
                        setExpandedModel(expanded ? null : item.model)
                      }
                    >
                      {expanded ? <ChevronDown /> : <ChevronRight />}
                      <span className='truncate'>{item.model}</span>
                    </Button>
                  </TableCell>
                  <TableCell>
                    <Badge
                      variant={
                        item.status === 'disabled' ? 'destructive' : 'secondary'
                      }
                    >
                      {item.status === 'disabled'
                        ? t('Disabled')
                        : t('Available')}
                    </Badge>
                  </TableCell>
                  <TableCell>
                    {formatNumber(item.today.failures, locale)} /{' '}
                    {formatNumber(item.today.requests, locale)}
                  </TableCell>
                  <TableCell>
                    {formatNumber(item.today.failure_rate, locale)}%
                  </TableCell>
                  <TableCell>
                    {item.today.last_failure_at
                      ? formatTimestampToDate(item.today.last_failure_at)
                      : '-'}
                  </TableCell>
                  <TableCell className='text-right'>
                    <Button
                      type='button'
                      variant='ghost'
                      size='sm'
                      className='text-destructive hover:text-destructive'
                      onClick={() => props.onDelete(item.model)}
                    >
                      {t('Delete')}
                    </Button>
                  </TableCell>
                </TableRow>
                {expanded && (
                  <TableRow>
                    <TableCell colSpan={6} className='bg-muted/20 p-3'>
                      <div className='grid gap-2 sm:grid-cols-2 lg:grid-cols-4'>
                        {item.days.map((day) => (
                          <div
                            key={day.date}
                            className='bg-background rounded-md border p-2 text-xs'
                          >
                            <div className='font-medium'>{day.date}</div>
                            <div className='text-muted-foreground mt-1'>
                              {t(
                                '{{failures}} failures / {{requests}} requests',
                                {
                                  failures: formatNumber(day.failures, locale),
                                  requests: formatNumber(day.requests, locale),
                                }
                              )}
                            </div>
                            <div className='text-muted-foreground'>
                              {t('Failure rate')}:{' '}
                              {formatNumber(day.failure_rate, locale)}%
                            </div>
                          </div>
                        ))}
                      </div>
                    </TableCell>
                  </TableRow>
                )}
              </Fragment>
            )
          })}
        </TableBody>
      </Table>
    </div>
  )
}
