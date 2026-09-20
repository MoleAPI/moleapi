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
import { render, screen } from '@testing-library/react'
import { expect, test, vi } from 'vitest'

import { SignIn } from '../sign-in'
import { SignUp } from '../sign-up'

vi.mock('@tanstack/react-router', () => ({
  Link: ({
    to,
    children,
    ...props
  }: {
    to: string
    children: React.ReactNode
  }) => (
    <a href={to} {...props}>
      {children}
    </a>
  ),
  useSearch: () => ({ redirect: undefined }),
}))

vi.mock('@/hooks/use-status', () => ({
  useStatus: () => ({
    status: {
      register_enabled: true,
      self_use_mode_enabled: true,
    },
  }),
}))

vi.mock('@/hooks/use-system-config', () => ({
  useSystemConfig: () => ({
    systemName: 'TEST',
    logo: '/logo.png',
    loading: false,
  }),
}))

vi.mock('../sign-in/components/user-auth-form', () => ({
  UserAuthForm: () => <div>Sign-in form</div>,
}))

vi.mock('../sign-up/components/sign-up-form', () => ({
  SignUpForm: () => <div>Sign-up form</div>,
}))

test('keeps the registration link visible when registration is enabled', () => {
  render(<SignIn />)

  expect(screen.getByRole('link', { name: 'Sign up' })).toHaveAttribute(
    'href',
    '/sign-up'
  )
})

test('keeps the sign-in link visible on the registration page', () => {
  render(<SignUp />)

  expect(screen.getByRole('link', { name: 'Sign in' })).toHaveAttribute(
    'href',
    '/sign-in'
  )
})
