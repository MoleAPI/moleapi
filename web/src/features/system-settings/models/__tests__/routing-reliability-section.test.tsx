import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen } from '@testing-library/react'
import type { ComponentProps } from 'react'
import { describe, expect, test, vi } from 'vitest'

import { RoutingReliabilitySection } from '../routing-reliability-section'

vi.mock('@/features/dashboard/api', () => ({
  getChannelSuccessMetrics: () =>
    Promise.resolve({
      success: true,
      data: {
        channels: [],
        probe_overview: {
          enabled: true,
          mode: 'intelligence',
          enabled_channels: 1,
          total_models: 1,
          healthy: 1,
          degraded: 0,
          pending: 0,
          items: [
            {
              channel_id: 7,
              channel_name: 'Primary route',
              model: 'model-a',
              status: 'healthy',
              recent_pass: 1,
              recent_total: 1,
            },
          ],
        },
      },
    }),
}))

vi.mock('@tanstack/react-router', () => ({
  Link: ({
    to,
    search: _search,
    ...props
  }: ComponentProps<'a'> & { to: string; search?: unknown }) => (
    <a {...props} href={to} />
  ),
}))

describe('routing reliability section', () => {
  test('shows the scheduled target as a manageable table row', async () => {
    const queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    })

    render(
      <QueryClientProvider client={queryClient}>
        <RoutingReliabilitySection
          defaultValues={{
            RetryTimes: 2,
            ChannelDisableThreshold: '0',
            AutomaticDisableChannelEnabled: true,
            AutomaticEnableChannelEnabled: true,
            AutomaticDisableKeywords: 'quota',
            AutomaticDisableStatusCodes: '401',
            AutomaticRetryStatusCodes: '500-599',
            'monitor_setting.auto_test_channel_enabled': true,
            'monitor_setting.auto_test_channel_minutes': 10,
            'monitor_setting.channel_test_concurrency': 1,
            'monitor_setting.channel_test_type': 'intelligence',
            'monitor_setting.channel_test_custom_prompt': '',
            'monitor_setting.channel_test_custom_answer': '',
            'monitor_setting.channel_test_mode': 'scheduled_all',
          }}
        />
      </QueryClientProvider>
    )

    expect(
      await screen.findByRole('columnheader', { name: 'Channel ID' })
    ).toBeInTheDocument()
    expect(screen.getByText('#7')).toBeInTheDocument()
    expect(screen.getByText('Primary route')).toBeInTheDocument()
    expect(screen.getByText('model-a')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Manage' })).toHaveAttribute(
      'href',
      '/channels'
    )
    expect(
      screen.getByText(
        'Status codes and failure keywords use OR logic: matching either one can trigger auto-disable.'
      )
    ).toBeInTheDocument()

    queryClient.clear()
  })
})
