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
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { expect, test, vi } from 'vitest'

import { UserAuthForm } from '../user-auth-form'

const mocks = vi.hoisted(() => ({
  login: vi.fn(async () => ({ success: false })),
  setTurnstileToken: vi.fn(),
}))

vi.mock('@tanstack/react-router', () => ({
  Link: (props: React.AnchorHTMLAttributes<HTMLAnchorElement>) => (
    <a {...props} />
  ),
}))

vi.mock('@/features/auth/api', () => ({
  login: mocks.login,
  wechatLoginByCode: vi.fn(),
}))

vi.mock('@/features/auth/hooks/use-turnstile', () => ({
  useTurnstile: () => ({
    isTurnstileEnabled: true,
    turnstileSiteKey: 'site-key',
    turnstileToken: 'verified-token',
    setTurnstileToken: mocks.setTurnstileToken,
    validateTurnstile: () => true,
  }),
}))

vi.mock('@/features/auth/hooks/use-auth-redirect', () => ({
  useAuthRedirect: () => ({
    handleLoginSuccess: vi.fn(),
    redirectTo2FA: vi.fn(),
  }),
}))

vi.mock('@/hooks/use-status', () => ({
  useStatus: () => ({ status: { password_login_enabled: true } }),
}))

vi.mock('@/features/auth/passkey', () => ({
  beginPasskeyLogin: vi.fn(),
  finishPasskeyLogin: vi.fn(),
}))

vi.mock('@/lib/passkey', () => ({
  buildAssertionResult: vi.fn(),
  prepareCredentialRequestOptions: vi.fn(),
  isPasskeySupported: vi.fn(async () => false),
}))

vi.mock('@/stores/auth-store', () => ({
  useAuthStore: (selector: (state: unknown) => unknown) =>
    selector({ auth: { setPending2FAFlowToken: vi.fn() } }),
}))

vi.mock('@/components/turnstile', () => ({
  Turnstile: () => <div data-testid='turnstile' />,
}))

test('consumes the Turnstile token after each password login attempt', async () => {
  render(<UserAuthForm />)

  fireEvent.change(screen.getByLabelText('Username or Email'), {
    target: { value: 'alice' },
  })
  fireEvent.change(screen.getByLabelText('Password'), {
    target: { value: 'password123' },
  })
  fireEvent.click(screen.getByRole('button', { name: 'Sign in' }))

  await waitFor(() => {
    expect(mocks.login).toHaveBeenCalledWith({
      username: 'alice',
      password: 'password123',
      turnstile: 'verified-token',
    })
  })
  expect(mocks.setTurnstileToken).toHaveBeenCalledWith('')
})
