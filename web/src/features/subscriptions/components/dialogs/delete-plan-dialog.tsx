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
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { ConfirmDialog } from '@/components/confirm-dialog'

import { deletePlan } from '../../api'
import { useSubscriptions } from '../subscriptions-provider'

export function DeletePlanDialog() {
  const { t } = useTranslation()
  const { open, setOpen, currentRow, triggerRefresh } = useSubscriptions()
  const [deleting, setDeleting] = useState(false)
  const isOpen = open === 'delete'
  const plan = currentRow?.plan
  const planLabel = plan?.title || (plan?.id ? `#${plan.id}` : '-')

  const handleConfirm = async () => {
    if (!plan?.id) return
    setDeleting(true)
    try {
      const res = await deletePlan(plan.id)
      if (res.success) {
        toast.success(t('Plan deleted'))
        triggerRefresh()
        setOpen(null)
      } else {
        toast.error(res.message || t('Operation failed'))
      }
    } catch (error) {
      toast.error(error instanceof Error ? error.message : t('Operation failed'))
    } finally {
      setDeleting(false)
    }
  }

  return (
    <ConfirmDialog
      open={isOpen}
      onOpenChange={(nextOpen) => !nextOpen && setOpen(null)}
      title={t('Confirm delete')}
      desc={t(
        'Delete {{plan}}? This cannot be undone. Plans with existing subscriptions or payment orders cannot be deleted; disable them instead.',
        { plan: planLabel }
      )}
      confirmText={t('Delete')}
      handleConfirm={handleConfirm}
      disabled={!plan?.id}
      isLoading={deleting}
      destructive
    />
  )
}
