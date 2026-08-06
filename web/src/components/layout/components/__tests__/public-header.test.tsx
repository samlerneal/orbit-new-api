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

const source = readFileSync(resolvePath('../public-header.tsx'), 'utf8')

function resolvePath(relativePath: string): string {
  return new URL(relativePath, import.meta.url).pathname
}

describe('public header contract', () => {
  test('keeps the five public destinations on a directly visible mobile row', () => {
    assert.match(source, /useTopNavLinks\(\{ scope: 'public' \}\)/)
    assert.match(source, /overflow-x-auto border-t/)
    assert.doesNotMatch(source, /Toggle navigation menu/)
  })

  test('preserves Playground authentication redirect and exposes support only in a copyable popover', () => {
    assert.match(source, /redirect: authPromptTarget\.href/)
    assert.match(source, /<PublicSupportPopover \/>/)
    assert.match(source, /<PublicSupportPopover mobile \/>/)
    assert.match(source, /copyToClipboard\('3184917639'\)/)
    assert.match(source, /PUBLIC_CONTACT_FALLBACK_LABEL/)
    assert.doesNotMatch(source, /PUBLIC_CONTACT_TARGET/)
  })

  test('shows only the locked brand without a hostname', () => {
    assert.match(source, /'日课 API'/)
    assert.doesNotMatch(source, /getPublicHostname/)
  })
})
