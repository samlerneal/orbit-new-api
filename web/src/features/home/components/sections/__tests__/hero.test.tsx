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

import {
  cloneElement,
  createElement,
  type ReactElement,
  type ReactNode,
} from 'react'
import { renderToStaticMarkup } from 'react-dom/server'

// @ts-expect-error Bun 1.3.14 provides mock.module at runtime; the app tsconfig only declares Node types.
const { mock } = await import('bun:test')

let currentStatus: Record<string, unknown> | null = null

mock.module('@lobehub/icons', () => ({
  CherryStudio: { Color: () => null },
}))

mock.module('@tanstack/react-router', () => ({
  Link: ({ children, to }: { children?: ReactNode; to: string }) =>
    createElement('a', { href: to }, children),
}))

mock.module('lucide-react', () => ({
  ArrowRight: () => null,
  BookOpen: () => null,
}))

mock.module('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}))

mock.module('@/components/ui/button', () => ({
  Button: ({
    children,
    render,
  }: {
    children?: ReactNode
    render?: ReactElement
  }) => (render ? cloneElement(render, {}, children) : null),
}))

mock.module('@/hooks/use-status', () => ({
  useStatus: () => ({ status: currentStatus }),
}))

mock.module('../hero-terminal-demo', () => ({
  HeroTerminalDemo: () => null,
}))

const { Hero } = await import('../hero')

function renderHero(status: Record<string, unknown> | null) {
  currentStatus = status
  return renderToStaticMarkup(createElement(Hero, { isAuthenticated: true }))
}

describe('hero usage guide contract', () => {
  test('renders an external docs_link with a new tab and safe rel', () => {
    const markup = renderHero({ docs_link: 'https://example.test/guide' })

    assert.match(
      markup,
      /<a href="https:\/\/example\.test\/guide" target="_blank" rel="noopener noreferrer"><span>Usage guide<\/span><\/a>/
    )
  })

  test('renders a relative docs_link as the internal docs fallback', () => {
    const markup = renderHero({ docs_link: '/docs' })

    assert.match(markup, /<a href="\/docs"><span>Usage guide<\/span><\/a>/)
    assert.doesNotMatch(markup, /href="\/docs" target="_blank"/)
  })
})
