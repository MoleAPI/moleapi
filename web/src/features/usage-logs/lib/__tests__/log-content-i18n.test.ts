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
import assert from 'node:assert/strict'
import { test } from 'vitest'

import { usageLogSchema } from '../../data/schema'
import type { LogOtherData } from '../../types'
import { parseLogOther, renderAuditContent, renderLogContent } from '../format'

const t = (key: string, opts?: Record<string, unknown>) =>
  key.replaceAll(/\{\{(\w+)\}\}/g, (placeholder, name: string) =>
    opts?.[name] == null ? placeholder : String(opts[name])
  )

function makeLog(type: number, content: string, other = '') {
  return usageLogSchema.parse({
    id: 1,
    user_id: 1,
    created_at: 1_700_000_000,
    type,
    content,
    token_name: '',
    model_name: '',
    quota: 0,
    prompt_tokens: 0,
    completion_tokens: 0,
    use_time: 0,
    is_stream: false,
    channel: 0,
    group: '',
    other,
  })
}

test('management and business log operations have localized renderers', () => {
  const actions = [
    'user.invite_rebate_apply_default',
    'user.invite_rebate_batch_update',
    'user.reward_transfer',
    'user.registration_reward',
    'user.invitee_reward',
    'user.inviter_reward',
    'user.topup_rebate',
    'user.checkin_reward',
    'user.security_verification',
    'user.2fa_setup_started',
    'user.2fa_enabled',
    'user.2fa_disabled',
    'user.2fa_backup_codes_regenerated',
    'option.model_pricing.import',
    'channel.status_update',
    'channel.status_update_batch',
    'channel.upstream_detect',
    'channel.upstream_detect_all',
    'topup.redemption',
    'topup.completed',
    'topup.admin_complete',
    'topup.invoice_view',
    'subscription.plan_reset',
    'subscription.user_plan_reset',
    'subscription.purchase',
    'subscription.balance_purchase',
    'performance.reset_stats',
    'system_info.delete_instance',
    'model.descriptions_import',
    'deployment.extend',
  ]

  for (const action of actions) {
    const other: LogOtherData = { op: { action } }
    assert.notEqual(renderAuditContent(other, t), null, action)
  }

  assert.equal(
    renderAuditContent(
      { op: { action: 'channel.status_update', params: { id: 7, status: 1 } } },
      t
    ),
    'Updated channel 7 status to Enabled'
  )
  assert.equal(
    renderAuditContent(
      {
        op: {
          action: 'user.manage',
          params: { action: 'disable', username: 'alice', id: 9 },
        },
      },
      t
    ),
    'Performed Disable on user alice (ID: 9)'
  )
})

test('historical management, system, and top-up sentences are localized', () => {
  const manage = makeLog(3, '管理员重置订阅套餐 Pro（ID: 8）额度')
  assert.equal(
    renderLogContent(manage, null, t),
    'Reset active subscriptions for plan Pro (ID: 8)'
  )

  const system = makeLog(4, '成功启用两步验证')
  assert.equal(
    renderLogContent(system, null, t),
    'Enabled two-factor authentication'
  )

  const topup = makeLog(
    1,
    'Waffo充值成功，充值额度: ＄10.000000，支付金额: 10.00'
  )
  assert.equal(
    renderLogContent(topup, parseLogOther(topup.other), t),
    'Top-up completed via Waffo: credited ＄10.000000, paid 10.00'
  )
})
