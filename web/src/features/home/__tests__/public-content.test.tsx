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
import { readFileSync } from 'node:fs'
import { describe, test } from 'node:test'

import { createElement } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'

import { hasCompletePublicHomeCatalog } from '../constants'

// @ts-expect-error Bun provides mock.module at runtime.
const { mock } = await import('bun:test')
mock.module('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}))
mock.module('@/lib/lobe-icon', () => ({
  getLobeIcon: (name: string) => createElement('svg', { 'data-logo': name }),
}))
const { Features } = await import('../components/sections/features')

describe('public Home catalog contract', () => {
  test('accepts the Owner-confirmed catalog sources', () => {
    assert.equal(
      hasCompletePublicHomeCatalog({
        availableModels: ['GPT'],
        plannedModels: ['Claude', 'Gemini', 'xAI'],
        availableSource: 'Owner production confirmation',
        plannedSource: 'Owner roadmap',
      }),
      true
    )
  })

  test('rejects overlapping catalog entries even when both sources exist', () => {
    assert.equal(
      hasCompletePublicHomeCatalog({
        availableModels: ['model-a'],
        plannedModels: ['model-a'],
        availableSource: 'production data',
        plannedSource: 'Owner roadmap',
      }),
      false
    )
  })

  test('keeps catalog logos centered, descriptive, and non-interactive', () => {
    const source = readFileSync(
      new URL('../components/sections/features.tsx', import.meta.url),
      'utf8'
    )
    assert.match(source, /items-center/)
    assert.match(source, /justify-center/)
    assert.match(source, /cursor-default/)
    assert.match(source, /aria-label=\{model\}/)
    assert.match(source, /Public catalog planned/)
    assert.doesNotMatch(source, /<button/)
    assert.doesNotMatch(source, /<a /)
    assert.doesNotMatch(source, /Coming soon/)
    assert.doesNotMatch(source, /暂未开放/)
  })

  test('does not expose provider names as visible logo-chip text', () => {
    const markup = renderToStaticMarkup(createElement(Features))
    for (const model of ['GPT', 'Claude', 'Gemini', 'xAI']) {
      assert.match(markup, new RegExp(`aria-label="${model}"`))
      assert.match(markup, new RegExp(`title="${model}"`))
      assert.doesNotMatch(markup, new RegExp(`>${model}<`))
    }
  })

  test('keeps the three actions and safe tutorial entry in the public flow', () => {
    const source = readFileSync(
      new URL('../components/sections/how-it-works.tsx', import.meta.url),
      'utf8'
    )
    for (const key of [
      'Public step one description',
      'Public step two description',
      'Public step three description',
    ]) {
      assert.match(source, new RegExp(key))
    }
    assert.match(source, /status\?\.docs_link/)
    assert.match(source, /resolveSafePublicLink/)
  })
})
