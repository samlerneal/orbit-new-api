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

const source = readFileSync(new URL('../footer.tsx', import.meta.url), 'utf8')

describe('public footer contract', () => {
  test('uses native anchors for https and mailto targets', () => {
    assert.match(
      source,
      /startsWith\('https:'\) \|\| props\.href\.startsWith\('mailto:'\)/
    )
    assert.match(source, /target='_blank'/)
    assert.match(source, /rel='noopener noreferrer'/)
  })

  test('keeps legal links visibly unavailable without preparation copy and preserves AGPL attribution', () => {
    assert.match(source, /aria-disabled='true'/)
    assert.doesNotMatch(source, /Under preparation/)
    assert.match(source, /QuantumNous\/new-api/)
    assert.match(source, /Open source license/)
    assert.match(source, /Modified source/)
    assert.doesNotMatch(source, /PUBLIC_FILING_TEXT/)
  })

  test('uses public-only labels for every locked footer destination', () => {
    for (const key of [
      'Public privacy policy',
      'Public terms of service',
      'Public model pricing',
      'Public usage tutorial',
      'Public contact',
    ]) {
      assert.match(source, new RegExp(key))
    }
    assert.doesNotMatch(source, /t\('Privacy Policy'\)/)
    assert.doesNotMatch(source, /t\('User Agreement'\)/)
  })

  test('renders the QQ fallback through the non-interactive branch', () => {
    assert.match(source, /PUBLIC_CONTACT_FALLBACK_LABEL/)
    assert.match(source, /if \(!props\.href\)/)
    assert.match(source, /aria-disabled='true'/)
    assert.match(
      source,
      /t\('Public contact'\).*PUBLIC_CONTACT_FALLBACK_LABEL/s
    )
  })
})
