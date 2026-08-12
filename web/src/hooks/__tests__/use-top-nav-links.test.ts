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
*/
import assert from 'node:assert/strict'
import { describe, test } from 'node:test'

import { createElement } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'

// @ts-expect-error Bun provides mock.module at runtime.
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
    rankings: { enabled: true, requireAuth: false },
    docs: true,
    about: true,
  }),
}))

const { useTopNavLinks } = await import('../use-top-nav-links')

function HookProbe(props: { scope?: 'default' | 'public' }) {
  return createElement('output', {
    'data-links': JSON.stringify(useTopNavLinks({ scope: props.scope })),
  })
}

describe('top navigation scope contract', () => {
  test('keeps existing logged-in navigation semantics by default', () => {
    currentStatus = { docs_link: 'https://example.test/guide' }
    const markup = renderToStaticMarkup(createElement(HookProbe, {}))
    const links = JSON.parse(
      markup.match(/data-links="([^"]+)"/)?.[1].replaceAll('&quot;', '"') ||
        '[]'
    ) as Array<{ title: string }>

    assert.deepEqual(
      links.map((link) => link.title),
      ['Home', 'Console', 'Rankings', 'Usage guide', 'About']
    )
  })

  test('keeps public destinations independent from dashboard modules', () => {
    currentStatus = { docs_link: 'https://example.test/guide' }
    const markup = renderToStaticMarkup(
      createElement(HookProbe, { scope: 'public' })
    )
    const links = JSON.parse(
      markup.match(/data-links="([^"]+)"/)?.[1].replaceAll('&quot;', '"') ||
        '[]'
    ) as Array<{ title: string; href: string; requiresAuth?: boolean }>

    assert.deepEqual(
      links.map((link) => link.title),
      ['Console', 'Models', 'Public tutorial', 'Chat', 'Image studio']
    )
    assert.equal(links.find((link) => link.title === 'Chat')?.href, '/chat')
    assert.equal(
      links.find((link) => link.title === 'Chat')?.requiresAuth,
      true
    )
    assert.equal(
      links.find((link) => link.title === 'Image studio')?.href,
      '/image-studio'
    )
  })

  test('falls back to internal docs when the configured URL is unsafe', () => {
    currentStatus = { docs_link: 'javascript:alert(1)' }
    const markup = renderToStaticMarkup(
      createElement(HookProbe, { scope: 'public' })
    )
    const links = JSON.parse(
      markup.match(/data-links="([^"]+)"/)?.[1].replaceAll('&quot;', '"') ||
        '[]'
    ) as Array<{ title: string; href: string }>

    assert.equal(
      links.find((link) => link.title === 'Public tutorial')?.href,
      '/docs'
    )
  })
})
