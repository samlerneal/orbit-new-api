/* Copyright (C) 2023-2026 QuantumNous */
import assert from 'node:assert/strict'
import { describe, test } from 'node:test'

import { createElement } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'

import i18n from '@/i18n/config'

import type { PublicComparisonResult } from '../../lib/public-comparison'
import { PUBLIC_COMPARISON_CATALOG } from '../../public-comparison-catalog'
import type { PricingModel } from '../../types'
import { ModelCard } from '../model-card'

await i18n.changeLanguage('en')

function comparisonResult(modelName: 'gpt-5.5' | 'gpt-5.4-mini') {
  const catalogModel = PUBLIC_COMPARISON_CATALOG[0].models.find(
    (model) => model.publicModelId === modelName
  )
  assert.ok(catalogModel)
  const model: PricingModel = {
    id: modelName === 'gpt-5.5' ? 55 : 54,
    model_name: modelName,
    quota_type: 0,
    model_ratio: modelName === 'gpt-5.5' ? 0.15 : 0.0225,
    completion_ratio: 6,
    cache_ratio: 0.1,
    create_cache_ratio: null,
    enable_groups: ['default'],
  }
  return {
    group: PUBLIC_COMPARISON_CATALOG[0],
    catalogModel,
    model,
    savingsPercent: 94,
    salePrices: {
      input: modelName === 'gpt-5.5' ? 0.3 : 0.045,
      output: modelName === 'gpt-5.5' ? 1.8 : 0.27,
      cacheCreate: null,
      cacheRead: modelName === 'gpt-5.5' ? 0.03 : 0.0045,
    },
  } satisfies PublicComparisonResult
}

describe('public comparison model card', () => {
  for (const modelName of ['gpt-5.5', 'gpt-5.4-mini'] as const) {
    test(`${modelName} renders not_applicable cache creation without NaN`, () => {
      const comparison = comparisonResult(modelName)
      const markup = renderToStaticMarkup(
        createElement(ModelCard, {
          model: comparison.model,
          comparison,
          onClick: () => {},
        })
      )

      assert.match(markup, /Cache create[^<]*:[\s\S]*Not applicable/)
      assert.doesNotMatch(markup, /NaN/)
      assert.match(markup, /title="Copy"/)
      assert.match(markup, />Details</)
    })
  }
})
