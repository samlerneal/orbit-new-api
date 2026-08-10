/* Copyright (C) 2023-2026 QuantumNous */
import assert from 'node:assert/strict'
import { describe, test } from 'node:test'

import { PUBLIC_COMPARISON_CATALOG } from '../public-comparison-catalog'
import type { PricingModel } from '../types'
import {
  canShowGroupComparison,
  getPublicComparisonResults,
} from './public-comparison'

function model(
  model_name: string,
  model_ratio: number,
  completion_ratio: number,
  cache_ratio: number | null,
  create_cache_ratio: number | null
): PricingModel {
  return {
    id: 1,
    model_name,
    quota_type: 0,
    model_ratio,
    completion_ratio,
    cache_ratio,
    create_cache_ratio,
    enable_groups: ['default'],
  }
}

const validModels = [
  model('gpt-5.6-sol', 0.15, 6, 0.1, 1.25),
  model('gpt-5.5', 0.15, 6, 0.1, null),
  model('gpt-5.6-terra', 0.06, 6, 0.1, 1.25),
  model('gpt-5.6-luna', 0.006, 6, 0.1, 1.25),
  model('gpt-5.4-mini', 0.0225, 6, 0.1, null),
]

describe('public comparison', () => {
  test('shows all five verified models at 6% and 94% savings', () => {
    const results = getPublicComparisonResults(
      validModels,
      PUBLIC_COMPARISON_CATALOG
    )
    assert.equal(results.length, 5)
    assert.equal(
      canShowGroupComparison(results, PUBLIC_COMPARISON_CATALOG[0]),
      true
    )
    assert.ok(results.every((result) => result.savingsPercent === 94))
  })

  test('suppresses a group when a required cache ratio is missing or invalid', () => {
    const results = getPublicComparisonResults(
      [
        ...validModels.slice(0, 2),
        model('gpt-5.6-terra', 0.06, 6, null, 1.25),
        ...validModels.slice(3),
      ],
      PUBLIC_COMPARISON_CATALOG
    )
    assert.equal(
      canShowGroupComparison(results, PUBLIC_COMPARISON_CATALOG[0]),
      false
    )
  })

  test('rejects non-token, dynamic, and wrong-group models', () => {
    for (const invalid of [
      { ...validModels[0], quota_type: 1 },
      { ...validModels[0], billing_expr: 'input*2' },
      { ...validModels[0], enable_groups: ['other'] },
    ]) {
      assert.equal(
        getPublicComparisonResults([invalid], PUBLIC_COMPARISON_CATALOG).length,
        0
      )
    }
  })

  test('does not calculate catalog dimensions marked not_applicable', () => {
    const guardedGpt55 = model('gpt-5.5', 0.15, 6, 0.1, null)
    const guardedGpt54Mini = model('gpt-5.4-mini', 0.0225, 6, 0.1, null)
    for (const guardedModel of [guardedGpt55, guardedGpt54Mini]) {
      Object.defineProperty(guardedModel, 'create_cache_ratio', {
        get: () => {
          throw new Error('not_applicable cache creation was calculated')
        },
      })
    }

    const results = getPublicComparisonResults(
      [
        validModels[0],
        guardedGpt55,
        validModels[2],
        validModels[3],
        guardedGpt54Mini,
      ],
      PUBLIC_COMPARISON_CATALOG
    )
    const gpt55 = results.find(
      (result) => result.model.model_name === 'gpt-5.5'
    )
    const gpt54Mini = results.find(
      (result) => result.model.model_name === 'gpt-5.4-mini'
    )

    assert.equal(gpt55?.salePrices.cacheCreate, null)
    assert.equal(gpt54Mini?.salePrices.cacheCreate, null)
  })
})
