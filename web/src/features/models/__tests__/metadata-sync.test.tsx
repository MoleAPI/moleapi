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
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'

import { SyncWizardDialog } from '../components/dialogs/sync-wizard-dialog'
import type { MetadataSyncCandidate, MetadataSyncPreview } from '../types'

const preview: MetadataSyncPreview = {
  source: {
    locale: 'en',
    models_url: 'https://example.test/models.json',
    vendors_url: 'https://example.test/vendors.json',
    version: 'source-v1',
  },
  candidates: [
    {
      model_name: 'new-model',
      scope: 'site',
      kind: 'create',
      record_version: 'new-v1',
      vendor_to_create: 'Example vendor',
      fields: [{ field: 'description', local: '', upstream: 'New metadata' }],
    },
    {
      model_name: 'existing-model',
      scope: 'site',
      kind: 'update',
      record_version: 'existing-v1',
      fields: [
        { field: 'description', local: 'Local', upstream: 'Updated' },
        { field: 'status', local: 1, upstream: 0 },
      ],
    },
    {
      model_name: 'protected-model',
      scope: 'site',
      kind: 'blocked',
      record_version: 'protected-v1',
      fields: [],
    },
  ],
}

describe('metadata sync preview', () => {
  it('selects and clears changed fields across selected models while keeping new-model imports selected', async () => {
    const catalog: MetadataSyncCandidate = {
      ...preview.candidates[1],
      model_name: 'catalog-model',
      scope: 'catalog',
      record_version: 'catalog-v1',
      fields: [{ field: 'icon', local: 'Old icon', upstream: 'New icon' }],
    }
    vi.spyOn(api, 'get').mockResolvedValue({
      data: {
        success: true,
        data: { ...preview, candidates: [...preview.candidates, catalog] },
      },
    })
    const post = vi.spyOn(api, 'post').mockResolvedValue({
      data: {
        success: true,
        data: {
          created_models: ['new-model'],
          updated_models: [
            { model_name: 'existing-model', fields: ['description'] },
            { model_name: 'catalog-model', fields: ['icon'] },
          ],
          created_vendors: ['Example vendor'],
        },
      },
    })
    const client = new QueryClient({
      defaultOptions: {
        queries: { retry: false },
        mutations: { retry: false },
      },
    })
    render(
      <QueryClientProvider client={client}>
        <SyncWizardDialog open onOpenChange={() => {}} />
      </QueryClientProvider>
    )
    const user = userEvent.setup()
    await user.click(
      screen.getByRole('button', { name: 'Load metadata preview' })
    )
    await screen.findByRole('checkbox', { name: 'Select existing-model' })
    await user.click(screen.getByRole('combobox', { name: 'Sync scope' }))
    await user.click(
      screen.getByRole('option', { name: 'Upstream model list' })
    )
    await user.click(
      screen.getByRole('button', {
        name: 'Select all syncable models (all pages, 3)',
      })
    )
    await user.type(
      screen.getByRole('textbox', { name: 'Search models' }),
      'existing'
    )
    await user.click(
      screen.getByRole('button', { name: 'Preview selected changes' })
    )

    const selectAll = screen.getByRole('button', {
      name: 'Select all changed fields',
    })
    const clear = screen.getByRole('button', { name: 'Clear field selection' })
    const description = screen.getByRole('checkbox', {
      name: 'Apply Description for existing-model',
    })
    const status = screen.getByRole('checkbox', {
      name: 'Apply Model square visibility for existing-model',
    })
    const icon = screen.getByRole('checkbox', {
      name: 'Apply Icon for catalog-model',
    })
    const newDescription = screen.getByRole('checkbox', {
      name: 'Apply Description for new-model',
    })
    expect(screen.getByText('0 of 3 changed fields selected')).toBeVisible()
    expect(clear).toBeDisabled()
    expect(newDescription).toBeChecked()
    expect(newDescription).toHaveAttribute('aria-disabled', 'true')

    await user.click(description)
    expect(screen.getByText('1 of 3 changed fields selected')).toBeVisible()
    await user.tab({ shift: true })
    expect(clear).toHaveFocus()
    await user.tab({ shift: true })
    expect(selectAll).toHaveFocus()
    await user.keyboard('{Enter}')
    expect(description).toBeChecked()
    expect(status).toBeChecked()
    expect(icon).toBeChecked()
    expect(selectAll).toBeDisabled()
    expect(screen.getByText('3 of 3 changed fields selected')).toBeVisible()

    await user.click(clear)
    expect(description).not.toBeChecked()
    expect(status).not.toBeChecked()
    expect(icon).not.toBeChecked()
    expect(newDescription).toBeChecked()
    expect(clear).toBeDisabled()
    expect(screen.getByText('3 selected models')).toBeVisible()
    expect(
      screen.getByRole('button', { name: 'Review confirmation' })
    ).toBeEnabled()

    await user.click(selectAll)
    await user.click(status)
    expect(selectAll).toBeEnabled()
    expect(screen.getByText('2 of 3 changed fields selected')).toBeVisible()
    await user.click(
      screen.getByRole('button', { name: 'Review confirmation' })
    )
    expect(post).not.toHaveBeenCalled()
    await user.click(
      screen.getByRole('button', { name: 'Apply 3 model changes' })
    )
    await screen.findByText('Metadata sync completed')
    expect(post).toHaveBeenCalledWith('/api/models/sync_upstream', {
      locale: 'en',
      source_version: 'source-v1',
      selections: [
        {
          model_name: 'new-model',
          record_version: 'new-v1',
          create: true,
          fields: ['description'],
        },
        {
          model_name: 'existing-model',
          record_version: 'existing-v1',
          create: false,
          fields: ['description'],
        },
        {
          model_name: 'catalog-model',
          record_version: 'catalog-v1',
          create: false,
          fields: ['icon'],
        },
      ],
    })
    client.clear()
  })

  it('disables confirmation after clearing all fields when only existing models are selected', async () => {
    vi.spyOn(api, 'get').mockResolvedValue({
      data: { success: true, data: preview },
    })
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    })
    render(
      <QueryClientProvider client={client}>
        <SyncWizardDialog open onOpenChange={() => {}} />
      </QueryClientProvider>
    )
    const user = userEvent.setup()
    await user.click(
      screen.getByRole('button', { name: 'Load metadata preview' })
    )
    await user.click(
      await screen.findByRole('checkbox', { name: 'Select existing-model' })
    )
    await user.click(
      screen.getByRole('button', { name: 'Preview selected changes' })
    )
    expect(
      screen.getByText(
        'Existing models: select fields to replace their current values with the values on the right. Unselected fields stay unchanged.'
      )
    ).toBeVisible()
    const review = screen.getByRole('button', { name: 'Review confirmation' })
    expect(review).toBeDisabled()
    await user.click(
      screen.getByRole('button', { name: 'Select all changed fields' })
    )
    expect(review).toBeEnabled()
    await user.click(
      screen.getByRole('button', { name: 'Clear field selection' })
    )
    expect(review).toBeDisabled()
    expect(screen.getByText('1 selected models')).toBeVisible()
    client.clear()
  })
})
