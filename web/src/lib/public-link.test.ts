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

import {
  getPublicHostname,
  PUBLIC_CONTACT_FALLBACK_LABEL,
  PUBLIC_CONTACT_TARGET,
  resolveSafePublicLink,
} from './public-link'

describe('public link safety', () => {
  test('accepts only approved public contact protocols without secrets', () => {
    assert.equal(resolveSafePublicLink('/contact'), '/contact')
    assert.equal(resolveSafePublicLink('/contact/../support'), '/support')
    assert.equal(
      resolveSafePublicLink('https://example.test/contact'),
      'https://example.test/contact'
    )
    assert.equal(
      resolveSafePublicLink('mailto:help@example.test'),
      'mailto:help@example.test'
    )
    assert.equal(resolveSafePublicLink('javascript:alert(1)'), null)
    assert.equal(resolveSafePublicLink('https://user:pass@example.test'), null)
    assert.equal(
      resolveSafePublicLink('https://example.test/?token=value'),
      null
    )
    assert.equal(resolveSafePublicLink('/contact?token=value'), null)
    assert.equal(resolveSafePublicLink('\\\\example.test'), null)
    assert.equal(resolveSafePublicLink('/%5cexample.test'), null)
    assert.equal(resolveSafePublicLink('/%2e%2e/%2e%2e//evil.test'), null)
    assert.equal(
      resolveSafePublicLink('https://example.test/?%74%6f%6b%65%6e=secret'),
      null
    )
    assert.equal(
      resolveSafePublicLink('mailto:help@example.test?%61%75%74%68=secret'),
      null
    )
  })

  test('uses the canonical host when status is unsafe', () => {
    assert.equal(
      getPublicHostname('https://api.example.test'),
      'api.example.test'
    )
    assert.equal(getPublicHostname('javascript:alert(1)'), 'api.mydaily.info')
  })

  test('uses the approved non-interactive QQ fallback when no safe link exists', () => {
    assert.equal(PUBLIC_CONTACT_TARGET, null)
    assert.equal(PUBLIC_CONTACT_FALLBACK_LABEL, '客服 QQ：3184917639')
    assert.equal(resolveSafePublicLink(PUBLIC_CONTACT_TARGET), null)
  })
})
