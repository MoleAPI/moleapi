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
import { useReducedMotion } from 'motion/react'
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'

const TYPEWRITER_SLOGAN_KEYS = [
  'Simple setup, ready in moments',
  'One-click access, exceptional experience',
  'One API, seamless switching',
  'Better value, broader compatibility',
  'Many models, one unified service',
] as const

interface TypewriterState {
  phraseIndex: number
  characterCount: number
  deleting: boolean
}

export function HeroTypewriter() {
  const { i18n, t } = useTranslation()
  const shouldReduceMotion = useReducedMotion()
  const firstPhrase = t(TYPEWRITER_SLOGAN_KEYS[0])
  const [state, setState] = useState<TypewriterState>(() => ({
    phraseIndex: 0,
    characterCount: firstPhrase.length,
    deleting: false,
  }))
  const phrase = t(TYPEWRITER_SLOGAN_KEYS[state.phraseIndex])

  useEffect(() => {
    setState({
      phraseIndex: 0,
      characterCount: firstPhrase.length,
      deleting: false,
    })
  }, [firstPhrase, i18n.resolvedLanguage])

  useEffect(() => {
    if (shouldReduceMotion) return

    let delay = state.deleting ? 45 : 85
    if (!state.deleting && state.characterCount === phrase.length) delay = 1800
    if (state.deleting && state.characterCount === 0) delay = 240

    const timer = window.setTimeout(() => {
      setState((current) => {
        if (!current.deleting && current.characterCount < phrase.length) {
          return { ...current, characterCount: current.characterCount + 1 }
        }
        if (!current.deleting) return { ...current, deleting: true }
        if (current.characterCount > 0) {
          return { ...current, characterCount: current.characterCount - 1 }
        }
        return {
          phraseIndex:
            (current.phraseIndex + 1) % TYPEWRITER_SLOGAN_KEYS.length,
          characterCount: 0,
          deleting: false,
        }
      })
    }, delay)

    return () => window.clearTimeout(timer)
  }, [phrase.length, shouldReduceMotion, state])

  const visibleText = shouldReduceMotion
    ? firstPhrase
    : phrase.slice(0, state.characterCount)

  return (
    <p className='text-muted-foreground/80 mt-5 flex min-h-7 max-w-xl items-center text-base leading-relaxed md:text-[15px]'>
      <span className='sr-only'>{firstPhrase}</span>
      <span aria-hidden='true' className='font-medium'>
        {visibleText}
        <span className='hero-typewriter-cursor ml-0.5 text-blue-500'>|</span>
      </span>
    </p>
  )
}
