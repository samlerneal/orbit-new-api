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
let currentStatus: Record<string, unknown> | null = {
  docs_link: 'https://docs.mydaily.info/api/cc-switch',
}
mock.module('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}))
mock.module('@/hooks/use-status', () => ({
  useStatus: () => ({ status: currentStatus }),
}))
mock.module('@tanstack/react-router', () => ({
  Link: (props: { children?: React.ReactNode }) =>
    createElement('a', props, props.children),
}))
const { Hero } = await import('../hero')

describe('public hero content', () => {
  test('renders the Owner-approved copy and console action', () => {
    const markup = renderToStaticMarkup(createElement(Hero))
    assert.match(markup, /Public hero title/)
    assert.match(markup, /Public open console/)
  })

  test('uses only the safe configured tutorial target', () => {
    const markup = renderToStaticMarkup(createElement(Hero))
    assert.match(markup, /href="https:\/\/docs\.mydaily\.info\/api\/cc-switch"/)
    assert.doesNotMatch(markup, /href="\/docs"/)

    currentStatus = { docs_link: 'javascript:alert(1)' }
    const unsafeMarkup = renderToStaticMarkup(createElement(Hero))
    assert.doesNotMatch(unsafeMarkup, /href=/)
  })

  test('uses a responsive description measure instead of a forced line break', async () => {
    const source = await import('node:fs/promises').then(({ readFile }) =>
      readFile(new URL('../hero.tsx', import.meta.url), 'utf8')
    )
    assert.match(source, /max-w-\[52rem\]/)
    assert.doesNotMatch(source, /<br\s*\/?\s*>/)
  })
})
