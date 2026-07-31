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
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { Search, UsersRound } from 'lucide-react'
import { type ChangeEvent, useState } from 'react'
import type { Resolver } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import * as z from 'zod'

import { ConfirmDialog } from '@/components/confirm-dialog'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import {
  Form,
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'
import { formatQuota } from '@/lib/format'

import { batchUpdateInviteRebateRatio } from '../api'
import { FormDirtyIndicator } from '../components/form-dirty-indicator'
import { FormNavigationGuard } from '../components/form-navigation-guard'
import {
  SettingsForm,
  SettingsSwitchContent,
  SettingsSwitchItem,
  SettingsFormGrid,
  SettingsFormGridItem,
} from '../components/settings-form-layout'
import { SettingsPageFormActions } from '../components/settings-page-context'
import { SettingsSection } from '../components/settings-section'
import { useSettingsForm } from '../hooks/use-settings-form'
import { useUpdateOption } from '../hooks/use-update-option'
import type {
  InviteRebateBatchScope,
  InviteRebateBatchUpdateRequest,
  InviteRebateBatchUpdateResult,
} from '../types'

const quotaSchema = z.object({
  QuotaForNewUser: z.coerce.number().min(0),
  PreConsumedQuota: z.coerce.number().min(0),
  QuotaForInviter: z.coerce.number().min(0),
  QuotaForInvitee: z.coerce.number().min(0),
  TopUpLink: z.string(),
  general_setting: z.object({
    docs_link: z.string(),
  }),
  quota_setting: z.object({
    enable_free_model_pre_consume: z.boolean(),
    default_invite_rebate_ratio: z.coerce.number().min(0).max(10000),
  }),
})

type QuotaFormValues = z.infer<typeof quotaSchema>
type QuotaInputValue = number | ''

function formatQuotaInputValue(value: QuotaInputValue): string {
  return formatQuota(value === '' ? 0 : value)
}

function percentInputToRatio(value: QuotaInputValue): number {
  return value === '' ? 0 : Math.round(value * 100)
}

type QuotaSettingsSectionProps = {
  defaultValues: QuotaFormValues
  complianceConfirmed?: boolean
}

export function QuotaSettingsSection({
  defaultValues,
  complianceConfirmed = true,
}: QuotaSettingsSectionProps) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const updateOption = useUpdateOption()
  const [batchDialogOpen, setBatchDialogOpen] = useState(false)
  const [batchScope, setBatchScope] =
    useState<InviteRebateBatchScope>('non_standard')
  const [batchCurrentPercent, setBatchCurrentPercent] =
    useState<QuotaInputValue>('')
  const [batchTargetPercent, setBatchTargetPercent] =
    useState<QuotaInputValue>(1)
  const [batchPreview, setBatchPreview] =
    useState<InviteRebateBatchUpdateResult | null>(null)
  const handleNumberChange =
    (onChange: (value: QuotaInputValue) => void) =>
    (event: ChangeEvent<HTMLInputElement>) => {
      const value = event.currentTarget.valueAsNumber
      onChange(Number.isNaN(value) ? '' : value)
    }
  const handleBatchPercentChange =
    (onChange: (value: QuotaInputValue) => void) =>
    (event: ChangeEvent<HTMLInputElement>) => {
      const value = event.currentTarget.valueAsNumber
      onChange(Number.isNaN(value) ? '' : value)
      setBatchPreview(null)
    }

  const { form, handleSubmit, isDirty, isSubmitting } =
    useSettingsForm<QuotaFormValues>({
      resolver: zodResolver(quotaSchema) as Resolver<
        QuotaFormValues,
        unknown,
        QuotaFormValues
      >,
      defaultValues,
      onSubmit: async (_data, changedFields) => {
        for (const [key, value] of Object.entries(changedFields)) {
          await updateOption.mutateAsync({
            key,
            value: value as string | number | boolean,
          })
        }
      },
    })
  const defaultInviteRebateRatio =
    form.watch('quota_setting.default_invite_rebate_ratio') ?? 0
  const canPreviewBatch =
    batchTargetPercent !== '' &&
    (batchScope !== 'current_ratio' || batchCurrentPercent !== '')
  const buildBatchRebateRequest = (
    dryRun: boolean
  ): InviteRebateBatchUpdateRequest | null => {
    if (!canPreviewBatch) {
      return null
    }
    const request: InviteRebateBatchUpdateRequest = {
      scope: batchScope,
      target_ratio: percentInputToRatio(batchTargetPercent),
      dry_run: dryRun,
    }
    if (batchScope === 'current_ratio') {
      request.current_ratio = percentInputToRatio(batchCurrentPercent)
    }
    return request
  }
  const previewBatchRebate = useMutation({
    mutationFn: batchUpdateInviteRebateRatio,
    onSuccess: (data) => {
      if (!data.success || !data.data) {
        setBatchPreview(null)
        toast.error(data.message || t('Failed to preview users'))
        return
      }
      setBatchPreview(data.data)
      if (data.data.matched === 0) {
        toast.info(t('No users match this rule'))
      }
    },
    onError: (error: Error) => {
      setBatchPreview(null)
      toast.error(error.message || t('Failed to preview users'))
    },
  })
  const applyBatchRebate = useMutation({
    mutationFn: batchUpdateInviteRebateRatio,
    onSuccess: (data) => {
      if (!data.success || !data.data) {
        toast.error(data.message || t('Failed to update users'))
        return
      }
      queryClient.invalidateQueries({ queryKey: ['users'] })
      setBatchDialogOpen(false)
      setBatchPreview(null)
      toast.success(t('Updated {{count}} users', { count: data.data.updated }))
    },
    onError: (error: Error) => {
      toast.error(error.message || t('Failed to update users'))
    },
  })

  const handleOpenBatchDialog = () => {
    setBatchTargetPercent(
      defaultInviteRebateRatio > 0 ? defaultInviteRebateRatio / 100 : 1
    )
    setBatchPreview(null)
    setBatchDialogOpen(true)
  }

  const handlePreviewBatchRebate = () => {
    const request = buildBatchRebateRequest(true)
    if (!request) {
      toast.error(t('Please enter valid rebate ratios'))
      return
    }
    previewBatchRebate.mutate(request)
  }

  const handleApplyBatchRebate = () => {
    const request = buildBatchRebateRequest(false)
    if (!request) {
      toast.error(t('Please enter valid rebate ratios'))
      return
    }
    applyBatchRebate.mutate(request)
  }
  const batchScopeItems = [
    { value: 'non_standard', label: t('Non-standard rebate users') },
    { value: 'standard', label: t('Standard rebate users') },
    { value: 'zero', label: t('Zero rebate users') },
    {
      value: 'current_ratio',
      label: t('Users with a specific current rebate'),
    },
  ] satisfies { value: InviteRebateBatchScope; label: string }[]

  return (
    <SettingsSection title={t('Quota Settings')}>
      <FormNavigationGuard when={isDirty} />

      {!complianceConfirmed ? (
        <Alert variant='destructive'>
          <AlertDescription>
            {t(
              'Non-zero invitation rewards require compliance confirmation in Payment Gateway settings.'
            )}
          </AlertDescription>
        </Alert>
      ) : null}

      <Form {...form}>
        <SettingsForm onSubmit={handleSubmit}>
          <SettingsPageFormActions
            onSave={handleSubmit}
            isSaving={updateOption.isPending || isSubmitting}
          />
          <FormDirtyIndicator isDirty={isDirty} />
          <SettingsFormGrid>
            <FormField
              control={form.control}
              name='QuotaForNewUser'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('New User Quota')}</FormLabel>
                  <FormControl>
                    <Input
                      type='number'
                      value={field.value ?? ''}
                      onChange={handleNumberChange(field.onChange)}
                      name={field.name}
                      onBlur={field.onBlur}
                      ref={field.ref}
                    />
                  </FormControl>
                  <FormDescription>
                    {t(
                      'Initial quota given to new users ({{formattedQuota}})',
                      {
                        formattedQuota: formatQuotaInputValue(field.value),
                      }
                    )}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />

            <FormField
              control={form.control}
              name='PreConsumedQuota'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Pre-Consumed Quota')}</FormLabel>
                  <FormControl>
                    <Input
                      type='number'
                      value={field.value ?? ''}
                      onChange={handleNumberChange(field.onChange)}
                      name={field.name}
                      onBlur={field.onBlur}
                      ref={field.ref}
                    />
                  </FormControl>
                  <FormDescription>
                    {t('Quota consumed before charging users')}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />

            <FormField
              control={form.control}
              name='QuotaForInviter'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Inviter Reward')}</FormLabel>
                  <FormControl>
                    <Input
                      type='number'
                      value={field.value ?? ''}
                      onChange={handleNumberChange(field.onChange)}
                      name={field.name}
                      onBlur={field.onBlur}
                      ref={field.ref}
                    />
                  </FormControl>
                  <FormDescription>
                    {t(
                      'Quota given to users who invite others ({{formattedQuota}})',
                      {
                        formattedQuota: formatQuotaInputValue(field.value),
                      }
                    )}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />

            <FormField
              control={form.control}
              name='QuotaForInvitee'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Invitee Reward')}</FormLabel>
                  <FormControl>
                    <Input
                      type='number'
                      value={field.value ?? ''}
                      onChange={handleNumberChange(field.onChange)}
                      name={field.name}
                      onBlur={field.onBlur}
                      ref={field.ref}
                    />
                  </FormControl>
                  <FormDescription>
                    {t('Quota given to invited users ({{formattedQuota}})', {
                      formattedQuota: formatQuotaInputValue(field.value),
                    })}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />

            <SettingsFormGridItem span='full'>
              <FormField
                control={form.control}
                name='quota_setting.enable_free_model_pre_consume'
                render={({ field }) => (
                  <SettingsSwitchItem>
                    <SettingsSwitchContent>
                      <FormLabel>{t('Pre-Consume for Free Models')}</FormLabel>
                      <FormDescription>
                        {t(
                          'When enabled, zero-cost models also pre-consume quota before final settlement.'
                        )}
                      </FormDescription>
                    </SettingsSwitchContent>
                    <FormControl>
                      <Switch
                        checked={field.value}
                        onCheckedChange={field.onChange}
                        disabled={updateOption.isPending}
                      />
                    </FormControl>
                  </SettingsSwitchItem>
                )}
              />
            </SettingsFormGridItem>

            <FormField
              control={form.control}
              name='quota_setting.default_invite_rebate_ratio'
              render={({ field }) => {
                const ratioValue = field.value as number | ''
                return (
                  <FormItem>
                    <FormLabel>{t('Default top-up rebate (%)')}</FormLabel>
                    <FormControl>
                      <Input
                        type='number'
                        min={0}
                        max={100}
                        step={0.01}
                        value={
                          ratioValue === '' ? '' : Number(ratioValue) / 100
                        }
                        onChange={(event) => {
                          const value = event.currentTarget.valueAsNumber
                          field.onChange(
                            Number.isNaN(value) ? '' : Math.round(value * 100)
                          )
                        }}
                        name={field.name}
                        onBlur={field.onBlur}
                        ref={field.ref}
                      />
                    </FormControl>
                    <FormDescription>
                      {t(
                        'Default rebate ratio assigned to newly registered users. 1 means 1%.'
                      )}
                    </FormDescription>
                    <FormMessage />
                  </FormItem>
                )
              }}
            />

            <SettingsFormGridItem span='full'>
              <div className='border-border flex min-w-0 flex-col gap-3 rounded-lg border p-3 sm:flex-row sm:items-center sm:justify-between'>
                <div className='min-w-0 space-y-1'>
                  <FormLabel>{t('Batch adjust top-up rebate')}</FormLabel>
                  <FormDescription>
                    {isDirty
                      ? t(
                          'Save the default value before running this batch update.'
                        )
                      : t(
                          'Preview and update existing users by current rebate ratio. Default settings for new users are unchanged.'
                        )}
                  </FormDescription>
                </div>
                <Button
                  type='button'
                  variant='outline'
                  onClick={handleOpenBatchDialog}
                  disabled={isDirty || applyBatchRebate.isPending}
                >
                  <UsersRound data-icon='inline-start' />
                  <span>{t('Adjust rebate ratios')}</span>
                </Button>
              </div>
            </SettingsFormGridItem>

            <FormField
              control={form.control}
              name='TopUpLink'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Top-Up Link')}</FormLabel>
                  <FormControl>
                    <Input
                      placeholder={t('https://example.com/topup')}
                      {...field}
                    />
                  </FormControl>
                  <FormDescription>
                    {t('External link for users to purchase quota')}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />

            <FormField
              control={form.control}
              name='general_setting.docs_link'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Documentation Link')}</FormLabel>
                  <FormControl>
                    <Input
                      placeholder={t('https://docs.example.com')}
                      {...field}
                    />
                  </FormControl>
                  <FormDescription>
                    {t('Link to your documentation site')}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />
          </SettingsFormGrid>
        </SettingsForm>
      </Form>
      <ConfirmDialog
        open={batchDialogOpen}
        onOpenChange={(open) => {
          setBatchDialogOpen(open)
          if (!open) {
            setBatchPreview(null)
          }
        }}
        title={t('Batch adjust top-up rebate')}
        desc={t(
          'Preview and update existing users by current rebate ratio. Default settings for new users are unchanged.'
        )}
        confirmText={
          applyBatchRebate.isPending
            ? t('Updating...')
            : t('Apply to {{count}} users', {
                count: batchPreview?.matched ?? 0,
              })
        }
        disabled={!batchPreview || batchPreview.matched === 0}
        isLoading={applyBatchRebate.isPending}
        handleConfirm={handleApplyBatchRebate}
        className='sm:max-w-lg'
      >
        <div className='grid gap-4'>
          <div className='space-y-2'>
            <FormLabel>{t('User range')}</FormLabel>
            <Select
              items={batchScopeItems}
              value={batchScope}
              onValueChange={(value) => {
                setBatchScope(value as InviteRebateBatchScope)
                setBatchPreview(null)
              }}
            >
              <SelectTrigger className='w-full'>
                <SelectValue />
              </SelectTrigger>
              <SelectContent alignItemWithTrigger={false}>
                <SelectGroup>
                  {batchScopeItems.map((item) => (
                    <SelectItem key={item.value} value={item.value}>
                      {item.label}
                    </SelectItem>
                  ))}
                </SelectGroup>
              </SelectContent>
            </Select>
            {batchScope === 'non_standard' ? (
              <p className='text-muted-foreground text-sm'>
                {t(
                  'Non-standard means users whose current rebate is greater than 0% and different from the default.'
                )}
              </p>
            ) : null}
          </div>

          {batchScope === 'current_ratio' ? (
            <div className='space-y-2'>
              <FormLabel>{t('Current rebate (%)')}</FormLabel>
              <Input
                type='number'
                min={0}
                max={100}
                step={0.01}
                value={batchCurrentPercent}
                onChange={handleBatchPercentChange(setBatchCurrentPercent)}
              />
            </div>
          ) : null}

          <div className='space-y-2'>
            <FormLabel>{t('Target rebate (%)')}</FormLabel>
            <Input
              type='number'
              min={0}
              max={100}
              step={0.01}
              value={batchTargetPercent}
              onChange={handleBatchPercentChange(setBatchTargetPercent)}
            />
          </div>

          <div className='flex items-center justify-between gap-3'>
            <Button
              type='button'
              variant='outline'
              onClick={handlePreviewBatchRebate}
              disabled={!canPreviewBatch || previewBatchRebate.isPending}
            >
              <Search data-icon='inline-start' />
              <span>
                {previewBatchRebate.isPending
                  ? t('Previewing...')
                  : t('Preview changes')}
              </span>
            </Button>
            {batchPreview ? (
              <span className='text-muted-foreground text-sm'>
                {t('{{count}} users match this rule.', {
                  count: batchPreview.matched,
                })}
              </span>
            ) : null}
          </div>
        </div>
      </ConfirmDialog>
    </SettingsSection>
  )
}
