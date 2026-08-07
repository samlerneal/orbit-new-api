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

For commercial licensing, please contact support@quantumnous.com
*/
import assert from 'node:assert/strict'
import { describe, test } from 'node:test'

import { getPublicChatSignInRedirect } from './public-chat-route'

// @ts-expect-error Bun provides mock.module at runtime.
const { mock } = await import('bun:test')

let routeConfig: {
  beforeLoad: (options: { location: { href: string } }) => void
  component: () => { props: { children: { type: unknown } }; type: unknown }
} | null = null
let auth: { user: unknown; accessToken: string | null } = {
  user: null,
  accessToken: null,
}
let publicChatMounts = 0
const modelRequests = 0
const groupRequests = 0
const completionRequests = 0

const PublicLayout = () => null
const PublicChat = () => {
  publicChatMounts += 1
  return null
}

mock.module('@tanstack/react-router', () => ({
  createFileRoute: () => (config: NonNullable<typeof routeConfig>) => {
    routeConfig = config
    return config
  },
  redirect: (options: unknown) => options,
}))
mock.module('react/jsx-dev-runtime', () => ({
  Fragment: Symbol.for('react.fragment'),
  jsxDEV: (type: unknown, props: Record<string, unknown>) => ({ type, props }),
}))
mock.module('react/jsx-runtime', () => ({
  Fragment: Symbol.for('react.fragment'),
  jsx: (type: unknown, props: Record<string, unknown>) => ({ type, props }),
  jsxs: (type: unknown, props: Record<string, unknown>) => ({ type, props }),
}))
mock.module('@/components/layout', () => ({ PublicLayout }))
mock.module('@/features/playground', () => ({ PublicChat }))
mock.module('@/stores/auth-store', () => ({
  useAuthStore: { getState: () => ({ auth }) },
}))

await import('../../../routes/chat/index')
;(globalThis as { window: { location: { origin: string } } }).window = {
  location: { origin: 'https://console.example.test' },
}

function expectSignInRedirect() {
  assert.ok(routeConfig)
  try {
    routeConfig.beforeLoad({
      location: { href: 'https://console.example.test/chat' },
    })
    assert.fail('expected an authentication redirect')
  } catch (result) {
    assert.deepEqual(result, {
      to: '/sign-in',
      search: { redirect: '/chat' },
    })
  }
}

describe('public chat sign-in redirect', () => {
  test('keeps a same-origin chat target after sanitizer validation', () => {
    assert.equal(
      getPublicChatSignInRedirect(
        'https://console.example.test/chat?source=public#message',
        'https://console.example.test'
      ),
      '/chat?source=public#message'
    )
  })

  test('falls back to the public chat path for an unsafe target', () => {
    assert.equal(
      getPublicChatSignInRedirect(
        'https://attacker.example/chat',
        'https://console.example.test'
      ),
      '/chat'
    )
  })

  test('rejects protocol-relative redirect targets', () => {
    assert.equal(
      getPublicChatSignInRedirect(
        '//attacker.example/chat',
        'https://console.example.test'
      ),
      '/chat'
    )
  })

  for (const missingField of ['user', 'accessToken'] as const) {
    test(`redirects before mounting or requesting when auth.${missingField} is missing`, () => {
      auth = {
        user: missingField === 'user' ? null : { id: 1 },
        accessToken: missingField === 'accessToken' ? null : 'token',
      }
      publicChatMounts = 0

      expectSignInRedirect()

      assert.equal(publicChatMounts, 0)
      assert.equal(modelRequests, 0)
      assert.equal(groupRequests, 0)
      assert.equal(completionRequests, 0)
    })
  }

  test('uses PublicLayout rather than the authenticated surface after auth succeeds', () => {
    auth = { user: { id: 1 }, accessToken: 'token' }
    const config = routeConfig
    assert.ok(config)
    assert.doesNotThrow(() =>
      config.beforeLoad({
        location: { href: 'https://console.example.test/chat' },
      })
    )

    const page = config.component()
    assert.equal(page.type, PublicLayout)
    assert.equal(page.props.children.type, PublicChat)
  })
})
