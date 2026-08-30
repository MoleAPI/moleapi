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
type ApiEndpointRouteInput = {
  url?: unknown
  route?: unknown
  description?: unknown
}

export type ApiEndpointOption = {
  value: string
  label: string
  description: string
}

function normalizeApiEndpointUrl(rawUrl: string): string {
  const baseUrl = rawUrl.trim().replace(/\/+$/, '')
  const endpointBaseUrl = /^https?:\/\/home\.moleapi\.com(?:\/v1)?$/i.test(
    baseUrl
  )
    ? 'https://api.moleapi.com'
    : baseUrl
  if (!endpointBaseUrl || /\/v1$/i.test(endpointBaseUrl)) {
    return endpointBaseUrl || '/v1'
  }
  return `${endpointBaseUrl}/v1`
}

export function buildApiBaseUrl(
  configuredAddress: string,
  fallbackOrigin: string
): string {
  return normalizeApiEndpointUrl(
    configuredAddress.trim() || fallbackOrigin.trim()
  )
}

export function buildApiEndpointOptions(
  configuredAddress: string,
  fallbackOrigin: string,
  routes: ApiEndpointRouteInput[] = []
): ApiEndpointOption[] {
  const seen = new Set<string>()
  const options: ApiEndpointOption[] = []

  for (const route of routes) {
    if (typeof route.url !== 'string') continue
    const value = normalizeApiEndpointUrl(route.url)
    if (!value || value === '/v1' || seen.has(value)) continue
    seen.add(value)
    options.push({
      value,
      label:
        typeof route.route === 'string' && route.route.trim()
          ? route.route.trim()
          : value,
      description:
        typeof route.description === 'string' ? route.description.trim() : '',
    })
  }

  if (options.length === 0) {
    const value = buildApiBaseUrl(configuredAddress, fallbackOrigin)
    return [{ value, label: value, description: '' }]
  }

  const defaultIndex = options.findIndex(
    (option) => option.value === 'https://api.moleapi.com/v1'
  )
  if (defaultIndex <= 0) return options

  const defaultOption = options[defaultIndex]
  return [
    defaultOption,
    ...options.slice(0, defaultIndex),
    ...options.slice(defaultIndex + 1),
  ]
}
