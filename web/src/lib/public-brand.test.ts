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
import test from 'node:test'

import {
  CANONICAL_PUBLIC_BRAND_NAME,
  CC_SWITCH_DEFAULT_PROVIDER_BY_APP,
  getPublicBrandName,
} from './public-brand.ts'

const defaultSystemName = 'New API'

test('only zhCN uses the configured Chinese public brand', () => {
  for (const locale of ['zhCN', 'zhTW', 'en', 'unknown']) {
    assert.equal(
      getPublicBrandName({
        locale,
        publicBrandZhCN: ' 日课 API ',
        systemName: 'Mydaily API',
        defaultSystemName,
      }),
      locale === 'zhCN' ? '日课 API' : 'Mydaily API'
    )
  }
})

test('Chinese locale falls back when the public brand is unavailable', () => {
  for (const publicBrandZhCN of [undefined, null, '', '   ']) {
    assert.equal(
      getPublicBrandName({
        locale: 'zhCN',
        publicBrandZhCN,
        systemName: 'Mydaily API',
        defaultSystemName,
      }),
      'Mydaily API'
    )
  }
})

test('uses the existing default when the canonical name is unavailable', () => {
  assert.equal(
    getPublicBrandName({
      locale: 'zhCN',
      publicBrandZhCN: null,
      systemName: '   ',
      defaultSystemName,
    }),
    defaultSystemName
  )
})

test('keeps the canonical input available while deriving a Chinese display name', () => {
  const canonicalSystemName = 'Mydaily API'
  const displaySystemName = getPublicBrandName({
    locale: 'zhCN',
    publicBrandZhCN: '日课 API',
    systemName: canonicalSystemName,
    defaultSystemName,
  })

  assert.equal(displaySystemName, '日课 API')
  assert.equal(canonicalSystemName, 'Mydaily API')
})

test('all CC Switch applications use the canonical provider name', () => {
  for (const app of ['claude', 'codex', 'gemini'] as const) {
    assert.equal(
      CC_SWITCH_DEFAULT_PROVIDER_BY_APP[app],
      CANONICAL_PUBLIC_BRAND_NAME
    )
  }
})
