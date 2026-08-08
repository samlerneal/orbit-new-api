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

import { DEFAULT_CONFIG } from '../../constants'
import { buildChatCompletionPayload } from '../../lib/streaming/payload-builder'
import { canSubmitPublicChatInput } from './public-chat-input'

describe('public chat input contract', () => {
  test('blocks empty input, empty model lists, and disabled requests', () => {
    assert.equal(
      canSubmitPublicChatInput({
        disabled: false,
        hasModels: true,
        text: '   ',
      }),
      false
    )
    assert.equal(
      canSubmitPublicChatInput({
        disabled: false,
        hasModels: false,
        text: 'hello',
      }),
      false
    )
    assert.equal(
      canSubmitPublicChatInput({
        disabled: true,
        hasModels: true,
        text: 'hello',
      }),
      false
    )
  })

  test('allows a non-empty message only after a user model is available', () => {
    assert.equal(
      canSubmitPublicChatInput({
        disabled: false,
        hasModels: true,
        text: 'hello',
      }),
      true
    )
  })

  test('passes only enabled parameters through the real chat payload builder', () => {
    const config = {
      ...DEFAULT_CONFIG,
      model: 'allowed-model',
      group: 'allowed-group',
      temperature: 0.4,
      top_p: 0.7,
      max_tokens: 123,
      frequency_penalty: 0.2,
      presence_penalty: 0.3,
      seed: 99,
    }
    const payload = buildChatCompletionPayload(
      [
        {
          key: 'user',
          from: 'user',
          versions: [{ id: 'v', content: 'hello' }],
        },
      ],
      config,
      {
        temperature: true,
        top_p: false,
        max_tokens: true,
        frequency_penalty: false,
        presence_penalty: false,
        seed: true,
      }
    )
    assert.equal(payload.temperature, 0.4)
    assert.equal(payload.max_tokens, 123)
    assert.equal(payload.seed, 99)
    assert.equal('top_p' in payload, false)
    assert.equal('frequency_penalty' in payload, false)
    assert.equal('presence_penalty' in payload, false)
  })
})
