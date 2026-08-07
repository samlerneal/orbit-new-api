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
})
