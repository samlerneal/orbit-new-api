/* Copyright (C) 2023-2026 QuantumNous */
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { after, before, describe, test } from 'node:test'

import { GlobalRegistrator } from '@happy-dom/global-registrator'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { act, createElement } from 'react'
import { createRoot } from 'react-dom/client'

import i18n from '@/i18n/config'

import {
  canShowGroupComparison,
  getNormalModelsForVisibleComparisons,
  type PublicComparisonResult,
} from '../../lib/public-comparison'
import { PUBLIC_COMPARISON_CATALOG } from '../../public-comparison-catalog'
import type { PricingModel } from '../../types'
import { ModelCardGrid } from '../model-card-grid'

function comparisonResult(index: number): PublicComparisonResult {
  const catalogModel = PUBLIC_COMPARISON_CATALOG[0].models[index]
  const model: PricingModel = {
    id: index + 1,
    model_name: catalogModel.publicModelId,
    quota_type: 0,
    model_ratio: 1,
    completion_ratio: 1,
    enable_groups: ['default'],
  }
  return {
    group: PUBLIC_COMPARISON_CATALOG[0],
    catalogModel,
    model,
    savingsPercent: 94,
    salePrices: {
      input: 0.3,
      output: 1.8,
      cacheCreate: 0.375,
      cacheRead: 0.03,
    },
  }
}

function validComparisonModels(): PricingModel[] {
  const ratios = [
    [0.15, 6, 0.1, 1.25],
    [0.15, 6, 0.1, null],
    [0.06, 6, 0.1, 1.25],
    [0.006, 6, 0.1, 1.25],
    [0.0225, 6, 0.1, null],
  ] as const
  return PUBLIC_COMPARISON_CATALOG[0].models.map((catalogModel, index) => ({
    id: index + 1,
    model_name: catalogModel.publicModelId,
    quota_type: 0,
    model_ratio: ratios[index][0],
    completion_ratio: ratios[index][1],
    cache_ratio: ratios[index][2],
    create_cache_ratio: ratios[index][3],
    enable_groups: ['default'],
  }))
}

function normalModels(count: number): PricingModel[] {
  return Array.from({ length: count }, (_value, index) => ({
    id: index + 6,
    model_name: `normal-${String(index + 1).padStart(2, '0')}`,
    quota_type: 0,
    model_ratio: 1,
    completion_ratio: 1,
    enable_groups: ['default'],
  }))
}

async function click(element: Element) {
  await act(async () => {
    element.dispatchEvent(
      new MouseEvent('click', { bubbles: true, cancelable: true })
    )
    await Promise.resolve()
  })
}

function findButton(container: Element, label: string): HTMLButtonElement {
  const button = [...container.querySelectorAll('button')].find((candidate) =>
    candidate.textContent?.includes(label)
  )
  assert.ok(button, `button not found: ${label}`)
  return button
}

function renderedModelNames(container: Element): string[] {
  return [...container.querySelectorAll('h3')].map(
    (heading) => heading.textContent ?? ''
  )
}

before(async () => {
  GlobalRegistrator.register({ url: 'http://localhost/model-card-grid-test' })
  ;(
    globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }
  ).IS_REACT_ACT_ENVIRONMENT = true
  await i18n.changeLanguage('en')
})

after(() => {
  GlobalRegistrator.unregister()
})

describe('public comparison card grid', () => {
  test('falls back every model in an incomplete comparison group to normal cards', () => {
    const allResults = PUBLIC_COMPARISON_CATALOG[0].models.map(
      (_model, index) => comparisonResult(index)
    )
    const partialResults = allResults.slice(0, -1)
    const visibleGroups = PUBLIC_COMPARISON_CATALOG.filter((group) =>
      canShowGroupComparison(partialResults, group)
    )

    assert.equal(visibleGroups.length, 0)
    assert.deepEqual(
      getNormalModelsForVisibleComparisons(
        allResults.map((result) => result.model),
        partialResults,
        visibleGroups
      ).map((model) => model.model_name),
      PUBLIC_COMPARISON_CATALOG[0].models.map((model) => model.publicModelId)
    )
  })

  test('paginates 21 models as one non-repeating display plan with actions intact', async () => {
    const models = [...validComparisonModels(), ...normalModels(16)]
    const expectedNames = models.map((model) => model.model_name)
    const detailClicks: string[] = []
    const copiedValues: string[] = []
    Object.defineProperty(navigator, 'clipboard', {
      configurable: true,
      value: {
        writeText: async (value: string) => {
          copiedValues.push(value)
        },
      },
    })
    const queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    })
    queryClient.setQueryData(['perf-metrics-summary', 24], {
      data: { models: [] },
    })
    const container = document.createElement('div')
    document.body.replaceChildren(container)
    const root = createRoot(container)

    try {
      await act(async () => {
        root.render(
          createElement(
            QueryClientProvider,
            { client: queryClient },
            createElement(ModelCardGrid, {
              models,
              onModelClick: (modelName) => detailClicks.push(modelName),
            })
          )
        )
      })

      const firstPageNames = renderedModelNames(container)
      assert.equal(firstPageNames.length, 20)
      assert.equal(new Set(firstPageNames).size, 20)
      assert.match(container.textContent ?? '', /Page 1 of 2/)
      assert.match(
        container.textContent ?? '',
        new RegExp(PUBLIC_COMPARISON_CATALOG[0].displayName)
      )

      await click(findButton(container, 'Details'))
      const firstCopy = container.querySelector('button[title="Copy"]')
      assert.ok(firstCopy)
      await click(firstCopy)
      assert.deepEqual(detailClicks, ['gpt-5.6-sol'])
      assert.deepEqual(copiedValues, ['gpt-5.6-sol'])

      await click(findButton(container, 'Next page'))

      const secondPageNames = renderedModelNames(container)
      assert.deepEqual(secondPageNames, ['normal-16'])
      assert.match(container.textContent ?? '', /Page 2 of 2/)
      assert.doesNotMatch(
        container.textContent ?? '',
        new RegExp(PUBLIC_COMPARISON_CATALOG[0].displayName)
      )
      assert.doesNotMatch(container.textContent ?? '', /Official price: 6%/)
      assert.deepEqual(
        firstPageNames.filter((name) => secondPageNames.includes(name)),
        []
      )
      assert.deepEqual(
        [...firstPageNames, ...secondPageNames].sort(),
        [...expectedNames].sort()
      )

      await click(findButton(container, 'Details'))
      const secondCopy = container.querySelector('button[title="Copy"]')
      assert.ok(secondCopy)
      await click(secondCopy)
      assert.deepEqual(detailClicks, ['gpt-5.6-sol', 'normal-16'])
      assert.deepEqual(copiedValues, ['gpt-5.6-sol', 'normal-16'])
    } finally {
      await act(async () => root.unmount())
      queryClient.clear()
      container.remove()
    }
  })

  test('renders verified groups before normal paged cards without empty groups', () => {
    const source = readFileSync(
      new URL('../model-card-grid.tsx', import.meta.url),
      'utf8'
    )
    assert.match(source, /getPublicComparisonDisplayPlan/)
    assert.match(source, /displayPlan/)
    assert.match(source, /Official price: 6%/)
  })
})
