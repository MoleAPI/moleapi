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
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { z } from 'zod'

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
import { Switch } from '@/components/ui/switch'
import { handleServerError } from '@/lib/handle-server-error'
import { requireServerSuccess } from '@/lib/server-error-message'

import { updateSystemOption } from '../api'
import { SettingsForm } from '../components/settings-form-layout'
import { SettingsPageFormActions } from '../components/settings-page-context'
import { SettingsSection } from '../components/settings-section'
import { useResetForm } from '../hooks/use-reset-form'

const urlOrEmpty = z.union([z.literal(''), z.string().url()])
const schema = z.object({
  ZohoDeskEnabled: z.boolean(),
  ZohoDeskClientId: z.string(),
  ZohoDeskClientSecret: z.string(),
  ZohoDeskRefreshToken: z.string(),
  ZohoDeskOrgId: z.string(),
  ZohoDeskDepartmentId: z.string(),
  ZohoDeskApiDomain: z.string().url(),
  ZohoDeskAccountsDomain: z.string().url(),
  ZohoDeskFromEmail: z.string().email(),
  SupportDiscordUrl: urlOrEmpty,
  SupportTelegramUrl: urlOrEmpty,
  SupportQQUrl: urlOrEmpty,
  SupportWeChatUrl: urlOrEmpty,
})

type Values = z.infer<typeof schema>

export function SupportSettingsSection(props: { defaultValues: Values }) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const form = useForm<Values>({
    resolver: zodResolver(schema),
    defaultValues: props.defaultValues,
  })
  useResetForm(form, props.defaultValues)
  const save = useMutation({
    mutationFn: async (values: Values) => {
      const entries = Object.entries(values).filter(
        ([key, value]) =>
          !['ZohoDeskClientSecret', 'ZohoDeskRefreshToken'].includes(key) ||
          value !== ''
      )
      for (const [key, value] of entries) {
        requireServerSuccess(await updateSystemOption({ key, value }))
      }
    },
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ['system-options'] })
      await queryClient.invalidateQueries({ queryKey: ['support'] })
      toast.success(t('Support settings saved'))
    },
    onError: (error) =>
      handleServerError(error, t('Failed to save support settings')),
  })

  const fields: Array<{
    name: Exclude<keyof Values, 'ZohoDeskEnabled'>
    label: string
    secret?: boolean
    description?: string
  }> = [
    { name: 'ZohoDeskClientId', label: 'Client ID' },
    { name: 'ZohoDeskClientSecret', label: 'Client secret', secret: true },
    { name: 'ZohoDeskRefreshToken', label: 'Refresh token', secret: true },
    { name: 'ZohoDeskOrgId', label: 'Organization ID' },
    { name: 'ZohoDeskDepartmentId', label: 'Department ID' },
    { name: 'ZohoDeskApiDomain', label: 'API domain' },
    { name: 'ZohoDeskAccountsDomain', label: 'Accounts domain' },
    { name: 'ZohoDeskFromEmail', label: 'Sender email' },
    { name: 'SupportDiscordUrl', label: 'Discord link' },
    { name: 'SupportTelegramUrl', label: 'Telegram link' },
    { name: 'SupportQQUrl', label: 'QQ group link' },
    { name: 'SupportWeChatUrl', label: 'WeChat group link' },
  ]

  return (
    <SettingsSection title={t('Support & Community')}>
      <Form {...form}>
        <SettingsForm
          onSubmit={form.handleSubmit((value) => save.mutate(value))}
        >
          <SettingsPageFormActions
            onSave={form.handleSubmit((value) => save.mutate(value))}
            isSaving={save.isPending}
            saveLabel='Save support settings'
          />
          <FormField
            control={form.control}
            name='ZohoDeskEnabled'
            render={({ field }) => (
              <FormItem className='flex items-center justify-between gap-4 rounded-lg border p-4'>
                <div>
                  <FormLabel>{t('Enable ticket service')}</FormLabel>
                  <FormDescription>
                    {t(
                      'Turn this on after the Zoho Desk connection is complete.'
                    )}
                  </FormDescription>
                </div>
                <FormControl>
                  <Switch
                    checked={field.value}
                    onCheckedChange={field.onChange}
                  />
                </FormControl>
              </FormItem>
            )}
          />
          <div className='grid gap-5 md:grid-cols-2'>
            {fields.map((item) => (
              <FormField
                key={item.name}
                control={form.control}
                name={item.name}
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t(item.label)}</FormLabel>
                    <FormControl>
                      <Input
                        type={item.secret ? 'password' : 'text'}
                        autoComplete='off'
                        placeholder={
                          item.secret
                            ? t('Leave blank to keep the saved value')
                            : undefined
                        }
                        {...field}
                      />
                    </FormControl>
                    {item.description && (
                      <FormDescription>{t(item.description)}</FormDescription>
                    )}
                    <FormMessage />
                  </FormItem>
                )}
              />
            ))}
          </div>
        </SettingsForm>
      </Form>
    </SettingsSection>
  )
}
