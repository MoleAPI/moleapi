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
import { beforeEach, describe, expect, test, vi } from 'vitest'

import { UserAuthForm } from '../../sign-in/components/user-auth-form'
import { SignUpForm } from '../../sign-up/components/sign-up-form'

const mocks = vi.hoisted(() => ({
  githubLogin: vi.fn(),
}))

vi.mock('@tanstack/react-router', () => ({
  Link: (props: React.AnchorHTMLAttributes<HTMLAnchorElement>) => (
    <a {...props} />
  ),
}))

vi.mock('@/hooks/use-status', () => ({
  useStatus: () => ({
    status: {
      password_login_enabled: true,
      github_oauth: true,
      oauth_register_enabled: true,
      user_agreement_enabled: true,
      privacy_policy_enabled: true,
    },
  }),
}))

vi.mock('@/features/auth/hooks/use-oauth-login', () => ({
  useOAuthLogin: () => ({
    isLoading: false,
    githubButtonText: 'Continue with GitHub',
    githubButtonDisabled: false,
    handleGitHubLogin: mocks.githubLogin,
    handleDiscordLogin: vi.fn(),
    handleOIDCLogin: vi.fn(),
    handleLinuxDOLogin: vi.fn(),
    handleTelegramLogin: vi.fn(),
    handleCustomOAuthLogin: vi.fn(),
  }),
}))

vi.mock('@/features/auth/hooks/use-turnstile', () => ({
  useTurnstile: () => ({
    isTurnstileEnabled: false,
    turnstileSiteKey: '',
    turnstileToken: '',
    setTurnstileToken: vi.fn(),
    validateTurnstile: () => true,
  }),
}))

vi.mock('@/features/auth/hooks/use-auth-redirect', () => ({
  useAuthRedirect: () => ({
    handleLoginResult: vi.fn(async () => true),
    redirectToLogin: vi.fn(),
  }),
}))

vi.mock('@/features/auth/hooks/use-email-verification', () => ({
  useEmailVerification: () => ({
    isSending: false,
    secondsLeft: 0,
    isActive: false,
    sendCode: vi.fn(async () => false),
  }),
}))

vi.mock('@/features/auth/api', () => ({
  login: vi.fn(),
  register: vi.fn(),
  wechatLoginByCode: vi.fn(),
}))

vi.mock('@/features/auth/passkey', () => ({
  beginPasskeyLogin: vi.fn(),
  finishPasskeyLogin: vi.fn(),
}))

vi.mock('@/lib/passkey', () => ({
  isPasskeySupported: vi.fn(async () => false),
}))

beforeEach(() => {
  mocks.githubLogin.mockClear()
})

async function expectLegalConsentGate(): Promise<void> {
  const githubButton = screen.getByRole('button', {
    name: /Continue with GitHub$/,
  })
  expect(githubButton).toBeEnabled()

  fireEvent.click(githubButton)

  const checkbox = screen.getByRole('checkbox')
  expect(mocks.githubLogin).not.toHaveBeenCalled()
  expect(screen.getByRole('alert')).toHaveTextContent(
    'Please agree to the legal terms first'
  )
  await waitFor(() => expect(checkbox).toHaveFocus())

  fireEvent.click(checkbox)
  expect(screen.queryByRole('alert')).not.toBeInTheDocument()
  fireEvent.click(githubButton)
  expect(mocks.githubLogin).toHaveBeenCalledTimes(1)
}

describe('legal consent gate', () => {
  test('keeps alternative sign-in enabled and gates the action on click', async () => {
    render(<UserAuthForm />)
    expect(screen.getByRole('button', { name: 'Sign in' })).toBeEnabled()
    await expectLegalConsentGate()
  })

  test('keeps alternative registration enabled and gates the action on click', async () => {
    render(<SignUpForm />)
    expect(screen.getByRole('button', { name: 'Create account' })).toBeEnabled()
    await expectLegalConsentGate()
  })
})
