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

import { resolvePublicServerAddress } from './public-server-address.ts'

const origin = 'https://current.example.com'

test('resolves direct and nested server addresses without trailing slashes', () => {
  assert.equal(
    resolvePublicServerAddress(
      { server_address: 'https://api.mydaily.info/' },
      origin
    ),
    'https://api.mydaily.info'
  )
  assert.equal(
    resolvePublicServerAddress(
      { data: { server_address: ' https://api.mydaily.info/ ' } },
      origin
    ),
    'https://api.mydaily.info'
  )
  assert.equal(
    resolvePublicServerAddress(
      { server_address: 'https://api.mydaily.info:8443/' },
      origin
    ),
    'https://api.mydaily.info:8443'
  )
})

test('falls back to the current origin without a valid configured address', () => {
  assert.equal(resolvePublicServerAddress(undefined, origin), origin)
  assert.equal(
    resolvePublicServerAddress({ server_address: '   ' }, origin),
    origin
  )
})

test('rejects credentials and non-origin server address configurations', () => {
  for (const serverAddress of [
    'ftp://api.mydaily.info',
    'https://user:password@api.mydaily.info',
    'https://api.mydaily.info/v1',
    'https://api.mydaily.info?preview=true',
    'https://api.mydaily.info#fragment',
    'not a URL',
  ]) {
    assert.equal(
      resolvePublicServerAddress({ server_address: serverAddress }, origin),
      origin
    )
  }
})
