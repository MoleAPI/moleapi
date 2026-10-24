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
import createDOMPurify from 'dompurify'
import { expect, test } from 'vitest'

test.each(['parent', 'sibling'])(
  'removes event handlers when a sanitizing hook detaches the %s',
  (target) => {
    const purifier = createDOMPurify(window)
    const root = document.createElement('div')
    root.innerHTML =
      '<section><img id="remove"><img id="unsafe" onerror="alert(1)"></section><p>Safe text</p>'
    const unsafe = root.querySelector('#unsafe')
    purifier.addHook('afterSanitizeAttributes', (node) => {
      if (node.id === 'remove') {
        if (target === 'parent') node.parentElement?.remove()
        else node.nextElementSibling?.remove()
      }
    })

    purifier.sanitize(root, { IN_PLACE: true })

    expect(unsafe?.hasAttribute('onerror')).toBe(false)
    expect(root.textContent).toContain('Safe text')
  }
)

test('removes template expressions split across a shadow tree', () => {
  const purifier = createDOMPurify(window)
  const host = document.createElement('div')
  const shadow = host.attachShadow({ mode: 'open' })
  shadow.append('$', document.createElement('object'), '{alert(1)}')

  purifier.sanitize(host, { IN_PLACE: true, SAFE_FOR_TEMPLATES: true })

  expect(shadow.textContent).not.toContain('${')
})

test('prevents XHTML SVG animation from changing a link to a script URL', () => {
  const purifier = createDOMPurify(window)
  const clean = purifier.sanitize(
    '<svg xmlns="http://www.w3.org/2000/svg"><a><animate attributeName="href" values="safe;javascript:alert(1)"/><text>Safe text</text></a></svg>',
    {
      PARSER_MEDIA_TYPE: 'application/xhtml+xml',
      ALLOWED_TAGS: ['svg', 'a', 'animate', 'text'],
      ALLOWED_ATTR: ['attributeName', 'values'],
    }
  )

  expect(clean).not.toContain('attributeName="href"')
  expect(clean).toContain('Safe text')
})
