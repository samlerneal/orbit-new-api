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

import { hasCompletePublicHomeCatalog } from '../constants'

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
})
