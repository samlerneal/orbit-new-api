/* Copyright (C) 2023-2026 QuantumNous */
import assert from 'node:assert/strict'
import { describe, test } from 'node:test'

import type { ModelOption } from '../../types'
import {
  filterPublicChatModels,
  resolveModelOptions,
} from './playground-option-utils'

const option = (value: string): ModelOption => ({ label: value, value })

describe('public chat model option projection', () => {
  test('hides only the exact case-sensitive gpt-image-2 value', () => {
    const models = [
      option('gpt-5.6-sol'),
      option('gpt-image-2'),
      option('GPT-IMAGE-2'),
      option('gpt-image-2-preview'),
    ]

    assert.deepEqual(
      filterPublicChatModels(models).map((model) => model.value),
      ['gpt-5.6-sol', 'GPT-IMAGE-2', 'gpt-image-2-preview']
    )
    assert.equal(models.length, 4)
  })

  test('falls back from a persisted hidden selection to the first text model', () => {
    const resolved = resolveModelOptions(
      [option('gpt-image-2'), option('gpt-5.6-sol'), option('gpt-5.5')],
      'gpt-image-2',
      filterPublicChatModels
    )

    assert.deepEqual(
      resolved.models.map((model) => model.value),
      ['gpt-5.6-sol', 'gpt-5.5']
    )
    assert.equal(resolved.nextModel, 'gpt-5.6-sol')
  })

  test('clears a hidden selection when no text model remains', () => {
    const resolved = resolveModelOptions(
      [option('gpt-image-2')],
      'gpt-image-2',
      filterPublicChatModels
    )

    assert.deepEqual(resolved.models, [])
    assert.equal(resolved.nextModel, '')
  })

  test('does not alter the complete playground without a projection', () => {
    const models = [option('gpt-image-2'), option('gpt-5.6-sol')]
    const resolved = resolveModelOptions(models, 'gpt-image-2')

    assert.equal(resolved.models, models)
    assert.equal(resolved.nextModel, null)
  })
})
