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
import {
  createMemoryHistory,
  createRootRoute,
  createRoute,
  createRouter,
  RouterProvider,
} from '@tanstack/react-router'
import {
  cleanup,
  render,
  renderHook,
  screen,
  waitFor,
  within,
} from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, assert, beforeEach, describe, expect, it, vi } from 'vitest'

import { Profile } from '@/features/profile'
import type { UserProfile } from '@/features/profile/types'
import { useSidebarData } from '@/hooks/use-sidebar-data'
import { api } from '@/lib/api'
import { useAuthStore } from '@/stores/auth-store'

import { Security } from '../index'

const profile: UserProfile = {
  id: 1,
  username: 'alice',
  display_name: 'Alice',
  role: 1,
  group: 'default',
  quota: 1000000,
  used_quota: 0,
  request_count: 0,
  status: 1,
  aff_count: 0,
  aff_quota: 0,
  aff_history_quota: 0,
  created_time: 0,
  setting: JSON.stringify({
    notify_type: 'email',
    quota_warning_threshold: 500000,
  }),
}

let currentProfile: UserProfile

beforeEach(() => {
  currentProfile = { ...profile }
  vi.stubGlobal('localStorage', {
    getItem: () => null,
    setItem: () => undefined,
    removeItem: () => undefined,
  })
  vi.spyOn(window, 'scrollTo').mockImplementation(() => undefined)
  useAuthStore
    .getState()
    .auth.setUser({ ...profile, permissions: { sidebar_settings: false } })
  vi.spyOn(api, 'get').mockImplementation(async (url) => {
    if (url === '/api/user/token/status') {
      return {
        data: {
          success: true,
          data: {
            exists: false,
            token_ref: '',
            created_at: null,
            last_used_at: null,
            last_used_ip: '',
          },
        },
      }
    }
    if (url === '/api/user/self') {
      return { data: { success: true, data: currentProfile } }
    }
    if (url === '/api/user/passkey') {
      return { data: { success: true, data: { enabled: false } } }
    }
    if (url === '/api/user/2fa/status') {
      return {
        data: {
          success: true,
          data: { enabled: false, locked: false, backup_codes_remaining: 0 },
        },
      }
    }
    if (url === '/api/user/sessions') {
      return { data: { success: true, data: [] } }
    }
    throw new Error(`Unexpected GET ${url}`)
  })
})

afterEach(() => {
  cleanup()
  vi.unstubAllGlobals()
  useAuthStore.getState().auth.reset()
  vi.restoreAllMocks()
})

async function renderPage(path = '/profile') {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  client.setQueryData(['status'], {
    checkin_enabled: false,
    wechat_login: true,
    github_oauth: true,
    oidc_enabled: true,
    custom_oauth_providers: [
      {
        id: 1,
        name: 'Gitea',
        slug: 'gitea',
        client_id: 'test-client',
        authorization_endpoint: 'https://example.com/oauth/authorize',
        scopes: 'openid',
      },
    ],
  })
  const root = createRootRoute()
  const security = createRoute({
    getParentRoute: () => root,
    path: '/security',
    component: Security,
  })
  const personal = createRoute({
    getParentRoute: () => root,
    path: '/profile',
    component: Profile,
  })
  const router = createRouter({
    routeTree: root.addChildren([security, personal]),
    history: createMemoryHistory({ initialEntries: [path] }),
  })
  await router.load()
  const rendered = render(
    <QueryClientProvider client={client}>
      <RouterProvider router={router} />
    </QueryClientProvider>
  )
  return { ...rendered, router }
}

describe('unified profile and security page', () => {
  it('keeps one copy of each security control alongside profile preferences', async () => {
    await renderPage()
    expect(
      await screen.findByRole('button', { name: 'Change Password' })
    ).toBeVisible()
    expect(
      screen.getAllByRole('button', { name: 'Change Password' })
    ).toHaveLength(1)
    expect(
      screen.getAllByRole('button', { name: 'Delete Account' })
    ).toHaveLength(1)
    expect(
      screen.getAllByRole('list', { name: 'Account Bindings' })
    ).toHaveLength(1)
    expect(
      screen.getAllByRole('heading', { name: 'Access Token' })
    ).toHaveLength(1)
    expect(screen.getAllByText('Passkey Login')).toHaveLength(1)
    expect(screen.getAllByText('Two-Factor Authentication')).toHaveLength(1)
    expect(await screen.findByText('No active login sessions')).toBeVisible()
    expect(
      screen.getAllByRole('switch', { name: 'Record IP Address' })
    ).toHaveLength(1)
    expect(screen.getByRole('button', { name: 'Save Settings' })).toBeVisible()
    const settings = screen.getByText('Settings & Preferences')
    const left = settings.closest('[data-profile-column]')
    expect(left).toHaveAttribute('data-profile-column', 'left')
    expect(left?.firstElementChild).toContainElement(settings)
    for (const title of [
      'Account Bindings',
      'Language Preferences',
      'Security',
    ]) {
      expect(
        screen.getByText(title).closest('[data-profile-column]')
      ).toHaveAttribute('data-profile-column', 'right')
    }
  })

  it('old security links redirect to Profile without a second settings page', async () => {
    const { router } = await renderPage('/security')
    await waitFor(() => expect(router.state.location.pathname).toBe('/profile'))
    expect(
      await screen.findByRole('button', { name: 'Change Password' })
    ).toBeVisible()
  })

  it('passwordless accounts refresh their profile after setting a verified password', async () => {
    currentProfile = { ...profile, has_password: false }
    const originalGet = vi.mocked(api.get).getMockImplementation()
    assert(originalGet)
    vi.mocked(api.get).mockImplementation(async (url, config) => {
      if (url === '/api/verify/methods') {
        return {
          data: {
            success: true,
            data: {
              scope: 'account.password.set',
              methods: [{ method: '2fa', available: true }],
              oauth_providers: [],
              password_encryption_enabled: false,
            },
          },
        }
      }
      return originalGet(url, config)
    })
    vi.spyOn(api, 'post').mockResolvedValue({
      data: {
        success: true,
        data: {
          scope: 'account.password.set',
          method: '2fa',
          proof_token: 'test-password-proof',
          expires_at: Math.floor(Date.now() / 1000) + 60,
        },
      },
    })
    const put = vi.spyOn(api, 'put').mockImplementation(async () => {
      currentProfile = { ...currentProfile, has_password: true }
      return { data: { success: true, data: { has_password: true } } }
    })
    const user = userEvent.setup()
    await renderPage()
    const setPassword = await screen.findByRole('button', {
      name: 'Set Password',
    })
    const notificationEmail = screen.getByRole('textbox', {
      name: 'Notification Email',
    })
    await user.clear(notificationEmail)
    await user.type(notificationEmail, 'draft@example.com')
    await user.click(setPassword)
    const dialog = await screen.findByRole('dialog', { name: 'Set Password' })
    expect(
      within(dialog).queryByLabelText('Current Password')
    ).not.toBeInTheDocument()
    await user.type(
      within(dialog).getByLabelText('New Password'),
      'test-account-password!42'
    )
    await user.type(
      within(dialog).getByLabelText('Confirm New Password'),
      'test-account-password!42'
    )
    await user.click(
      within(dialog).getByRole('button', { name: 'Set Password' })
    )
    expect(put).not.toHaveBeenCalled()
    await user.type(
      await screen.findByLabelText('Authenticator code or backup code', {
        selector: 'input',
      }),
      '123456'
    )
    await user.click(screen.getByRole('button', { name: 'Verify' }))
    await waitFor(() =>
      expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
    )
    expect(
      await screen.findByRole('button', { name: 'Change Password' })
    ).toBeVisible()
    expect(notificationEmail).toHaveValue('draft@example.com')
    expect(put).toHaveBeenCalledWith(
      '/api/user/self',
      { password: 'test-account-password!42' },
      expect.objectContaining({
        headers: { 'X-Security-Proof': 'test-password-proof' },
        singleUseAuthorization: true,
      })
    )
  })

  it('the single privacy control saves with notification preferences', async () => {
    currentProfile = {
      ...profile,
      setting: JSON.stringify({
        record_ip_log: true,
        notify_type: 'email',
        quota_warning_threshold: 500000,
        notification_email: 'alerts@example.com',
      }),
    }
    const put = vi
      .spyOn(api, 'put')
      .mockImplementation(async (_url, settings) => {
        currentProfile = {
          ...currentProfile,
          setting: JSON.stringify(settings),
        }
        return { data: { success: true } }
      })
    const user = userEvent.setup()
    await renderPage()
    const toggle = await screen.findByRole('switch', {
      name: 'Record IP Address',
    })
    await waitFor(() => expect(toggle).toBeChecked())
    await user.click(toggle)
    await user.click(screen.getByRole('button', { name: 'Save Settings' }))
    await waitFor(() =>
      expect(put).toHaveBeenCalledWith(
        '/api/user/setting',
        expect.objectContaining({
          record_ip_log: false,
          notify_type: 'email',
          notification_email: 'alerts@example.com',
        })
      )
    )
    expect(toggle).not.toBeChecked()
  })

  it('built-in and custom bindings share one compact responsive grid', async () => {
    await renderPage()
    const bindings = await screen.findByRole('list', {
      name: 'Account Bindings',
    })
    expect(within(bindings).getAllByRole('listitem')).toHaveLength(5)
    expect(within(bindings).getByText('Gitea')).toBeVisible()
    expect(bindings).toHaveClass(
      'grid-cols-1',
      'sm:grid-cols-2',
      'lg:grid-cols-3',
      'gap-2'
    )
    expect(screen.queryByText('Custom OAuth')).not.toBeInTheDocument()
  })

  it('the password action opens the existing dialog by keyboard and Escape closes it', async () => {
    const user = userEvent.setup()
    await renderPage()
    const action = await screen.findByRole('button', {
      name: 'Change Password',
    })
    action.focus()
    await user.keyboard('{Enter}')
    const dialog = await screen.findByRole('dialog', {
      name: 'Change Password',
    })
    expect(within(dialog).getByLabelText('Current Password')).toBeVisible()
    expect(
      within(dialog).getByLabelText('New Password', { exact: true })
    ).toBeVisible()
    await user.keyboard('{Escape}')
    await waitFor(() =>
      expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
    )
  })

  it('personal navigation has one Profile entry and no separate Security entry', () => {
    const { result } = renderHook(useSidebarData)
    const items = result.current.navGroups.find(
      (group) => group.id === 'personal'
    )?.items
    expect(
      items?.filter((item) => 'url' in item && item.url === '/profile')
    ).toHaveLength(1)
    expect(
      items?.some((item) => 'url' in item && item.url === '/security')
    ).toBe(false)
  })

  it('a failed profile load offers retry before exposing account actions', async () => {
    const originalGet = vi.mocked(api.get).getMockImplementation()
    assert(originalGet)
    let failed = false
    vi.mocked(api.get).mockImplementation(async (url, config) => {
      if (url === '/api/user/self' && !failed) {
        failed = true
        return { data: { success: false } }
      }
      return originalGet(url, config)
    })
    const user = userEvent.setup()
    await renderPage()
    expect(await screen.findByText('Failed to load profile')).toBeVisible()
    expect(
      screen.queryByRole('button', { name: 'Delete Account' })
    ).not.toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: 'Retry' }))
    expect(
      await screen.findByRole('button', { name: 'Delete Account' })
    ).toBeVisible()
  })
})
