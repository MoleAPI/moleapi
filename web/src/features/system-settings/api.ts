import { api } from '@/lib/api'

import type {
  ApplyDefaultInviteRebateRatioResponse,
  ConfirmPaymentComplianceResponse,
  FetchUpstreamRatiosRequest,
  InviteRebateBatchUpdateRequest,
  InviteRebateBatchUpdateResponse,
  LogCleanupTask,
  ModelPricingExport,
  ModelPricingImportResponse,
  SystemOptionsResponse,
  SystemTaskListResponse,
  SystemTaskResponse,
  UpdateOptionRequest,
  UpdateOptionResponse,
  UpstreamChannelsResponse,
  UpstreamRatiosResponse,
} from './types'

export async function getSystemOptions() {
  const res = await api.get<SystemOptionsResponse>('/api/option/')
  return res.data
}

export async function updateSystemOption(request: UpdateOptionRequest) {
  const res = await api.put<UpdateOptionResponse>('/api/option/', request)
  return res.data
}

export async function applyDefaultInviteRebateRatio() {
  const res = await api.post<ApplyDefaultInviteRebateRatioResponse>(
    '/api/user/invite_rebate/apply_default'
  )
  return res.data
}

export async function batchUpdateInviteRebateRatio(
  request: InviteRebateBatchUpdateRequest
) {
  const res = await api.post<InviteRebateBatchUpdateResponse>(
    '/api/user/invite_rebate/batch_update',
    request
  )
  return res.data
}

export async function confirmPaymentCompliance() {
  const res = await api.post<ConfirmPaymentComplianceResponse>(
    '/api/option/payment_compliance',
    { confirmed: true }
  )
  return res.data
}

export async function startLogCleanupTask(targetTimestamp: number) {
  const res = await api.post<SystemTaskResponse<LogCleanupTask>>(
    '/api/system-task/log-cleanup',
    null,
    {
      params: { target_timestamp: targetTimestamp },
    }
  )
  return res.data
}

export async function getCurrentLogCleanupTask() {
  const res = await api.get<SystemTaskResponse<LogCleanupTask | null>>(
    '/api/system-task/current',
    {
      params: { type: 'log_cleanup' },
    }
  )
  return res.data
}

export async function getSystemTask(taskId: string) {
  const res = await api.get<SystemTaskResponse<LogCleanupTask>>(
    `/api/system-task/${taskId}`
  )
  return res.data
}

export async function listSystemTasks(limit = 20) {
  const res = await api.get<SystemTaskListResponse>('/api/system-task/list', {
    params: { limit },
  })
  return res.data
}

export async function resetModelRatios() {
  const res = await api.post<UpdateOptionResponse>(
    '/api/option/rest_model_ratio'
  )
  return res.data
}

export async function exportModelPricing(): Promise<ModelPricingExport> {
  const res = await api.get('/api/option/model_pricing/export')
  return res.data
}

export async function importModelPricing(
  data: ModelPricingExport
): Promise<ModelPricingImportResponse> {
  const res = await api.post('/api/option/model_pricing/import', data, {
    skipBusinessError: true,
  })
  return res.data
}

export async function getUpstreamChannels() {
  const res = await api.get<UpstreamChannelsResponse>(
    '/api/ratio_sync/channels'
  )
  return res.data
}

export async function fetchUpstreamRatios(request: FetchUpstreamRatiosRequest) {
  const res = await api.post<UpstreamRatiosResponse>(
    '/api/ratio_sync/fetch',
    request
  )
  return res.data
}
