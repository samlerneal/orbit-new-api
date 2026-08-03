/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.
*/
import assert from 'node:assert/strict'
import { describe, test } from 'node:test'

import { createElement } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'

// @ts-expect-error Bun 1.3.14 provides mock.module at runtime; the app tsconfig only declares Node types.
const { mock } = await import('bun:test')

let currentStatus: Record<string, unknown> | null = null

mock.module('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}))

mock.module('@/hooks/use-status', () => ({
  useStatus: () => ({ status: currentStatus }),
}))

mock.module('@/stores/auth-store', () => ({
  useAuthStore: () => ({ auth: { user: null } }),
}))

mock.module('@/lib/nav-modules', () => ({
  parseHeaderNavModulesFromStatus: () => ({
    home: true,
    console: true,
    docs: true,
    about: true,
  }),
}))

const { useTopNavLinks } = await import('../use-top-nav-links')

function HookProbe() {
  const links = useTopNavLinks()

  return createElement('output', { 'data-links': JSON.stringify(links) }, null)
}

function renderTopNavLinks(status: Record<string, unknown> | null) {
  currentStatus = status
  const markup = renderToStaticMarkup(createElement(HookProbe))
  const linksAttribute = markup.match(/data-links="([^"]+)"/)?.[1]

  assert.ok(linksAttribute)
  return JSON.parse(linksAttribute.replaceAll('&quot;', '"')) as Array<{
    title: string
    href: string
    external?: boolean
  }>
}

describe('top navigation usage guide contract', () => {
  test('returns the dynamic external Usage guide link when docs_link exists', () => {
    const links = renderTopNavLinks({ docs_link: 'https://example.test/guide' })
    const usageGuideLink = links.find((link) => link.title === 'Usage guide')

    assert.deepEqual(usageGuideLink, {
      title: 'Usage guide',
      href: 'https://example.test/guide',
      external: true,
    })
  })

  test('returns the internal docs fallback when docs_link is absent', () => {
    const links = renderTopNavLinks(null)
    const usageGuideLink = links.find((link) => link.title === 'Usage guide')

    assert.deepEqual(usageGuideLink, {
      title: 'Usage guide',
      href: '/docs',
    })
  })
})
