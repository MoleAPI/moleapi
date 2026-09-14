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
import '@testing-library/jest-dom/vitest'
import { cleanup, configure } from '@testing-library/react'
import i18next from 'i18next'
import { initReactI18next } from 'react-i18next'
import { afterEach, beforeAll } from 'vitest'

// The testing-library default of 1000ms for findBy*/waitFor is too tight for
// this suite on contended CI runners, where a first-in-file test also pays the
// full cold-render cost. Keep it below vitest's testTimeout so async lookup
// failures still report the missing element instead of a generic test timeout.
configure({ asyncUtilTimeout: 5000 })

beforeAll(async () => {
  await i18next.use(initReactI18next).init({
    lng: 'en',
    fallbackLng: 'en',
    resources: {
      en: {
        translation: {},
      },
    },
  })
})

afterEach(() => {
  cleanup()
})

// Prefer reduced motion in tests: entrance animations write inline
// `opacity: 0` on their first frame, and jsdom advances frames through a
// setTimeout-based rAF shim, so jest-dom visibility assertions would race the
// animation. The reduced-motion code paths render the same DOM without
// transient hidden states. Both `(prefers-reduced-motion: reduce)` and the
// boolean `(prefers-reduced-motion)` form match; `no-preference` does not.
Object.defineProperty(window, 'matchMedia', {
  configurable: true,
  value: (query: string): MediaQueryList => ({
    matches:
      query.includes('prefers-reduced-motion') &&
      !query.includes('no-preference'),
    media: query,
    onchange: null,
    addListener: () => undefined,
    removeListener: () => undefined,
    addEventListener: () => undefined,
    removeEventListener: () => undefined,
    dispatchEvent: () => false,
  }),
})

window.requestAnimationFrame = (callback: FrameRequestCallback) =>
  window.setTimeout(() => callback(performance.now()), 0)
window.cancelAnimationFrame = (handle: number) => window.clearTimeout(handle)

class ResizeObserverMock {
  observe(): void {}
  unobserve(): void {}
  disconnect(): void {}
}

Object.defineProperty(globalThis, 'ResizeObserver', {
  configurable: true,
  value: ResizeObserverMock,
})

Object.defineProperty(HTMLElement.prototype, 'scrollIntoView', {
  configurable: true,
  value: () => undefined,
})
