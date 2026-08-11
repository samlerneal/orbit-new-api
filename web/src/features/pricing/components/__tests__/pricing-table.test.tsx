/* Copyright (C) 2023-2026 QuantumNous */
import assert from 'node:assert/strict'
import { after, afterEach, before, beforeEach, describe, test } from 'node:test'

import { GlobalRegistrator } from '@happy-dom/global-registrator'
import { act, createElement } from 'react'
import { createRoot, type Root } from 'react-dom/client'

import i18n from '@/i18n/config'

import {
  canShowGroupComparison,
  getNormalModelsForVisibleComparisons,
  type PublicComparisonResult,
} from '../../lib/public-comparison'
import { PUBLIC_COMPARISON_CATALOG } from '../../public-comparison-catalog'
import type { PricingModel } from '../../types'
import {
  ComparisonSavingsBadge,
  PricingTable,
  PublicComparisonModelCell,
} from '../pricing-table'

let container: HTMLDivElement
let root: Root
let copiedValues: string[]
let parentClicks: number

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
    officialPrices: {
      input: 5,
      output: 30,
      cacheCreate: 6.25,
      cacheRead: 0.5,
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
    vendor_id: 4,
    vendor_name: 'OpenAI',
    quota_type: 0,
    model_ratio: ratios[index][0],
    completion_ratio: ratios[index][1],
    cache_ratio: ratios[index][2],
    create_cache_ratio: ratios[index][3],
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

async function renderTable(
  models: PricingModel[],
  onModelClick?: (modelName: string) => void
) {
  await act(async () => {
    root.render(
      createElement(PricingTable, {
        models,
        groups: PUBLIC_COMPARISON_CATALOG,
        usdExchangeRate: 7.2,
        onModelClick,
      })
    )
  })
}

before(async () => {
  GlobalRegistrator.register({ url: 'http://localhost/pricing-table-test' })
  ;(
    globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }
  ).IS_REACT_ACT_ENVIRONMENT = true
  await i18n.changeLanguage('en')
})

beforeEach(async () => {
  copiedValues = []
  parentClicks = 0
  Object.defineProperty(navigator, 'clipboard', {
    configurable: true,
    value: {
      writeText: async (value: string) => {
        copiedValues.push(value)
      },
    },
  })
  container = document.createElement('div')
  document.body.replaceChildren(container)
  root = createRoot(container)
  await act(async () => {
    root.render(
      createElement(
        'div',
        { onClick: () => (parentClicks += 1) },
        PUBLIC_COMPARISON_CATALOG[0].models.map((_model, index) =>
          createElement(PublicComparisonModelCell, {
            key: index,
            model: comparisonResult(index).model,
          })
        )
      )
    )
  })
})

afterEach(async () => {
  await act(async () => root?.unmount())
  container?.remove()
})

after(() => {
  GlobalRegistrator.unregister()
})

describe('public pricing table model cells', () => {
  test('renders the fixed six-column group from catalog-derived prices', async () => {
    const detailClicks: string[] = []
    await renderTable(validComparisonModels(), (modelName) =>
      detailClicks.push(modelName)
    )

    const table = container.querySelector('table')
    assert.ok(table)
    assert.deepEqual(
      [...table.querySelectorAll('th')].map((header) =>
        header.textContent?.trim()
      ),
      [
        'Model',
        'Input price',
        'Output price',
        'Cache write',
        'Cache read',
        'Savings',
      ]
    )
    assert.equal(table.querySelectorAll('tbody tr').length, 5)
    assert.equal(
      new Set(
        [...table.querySelectorAll('tbody tr')].map((row) =>
          row.getAttribute('data-model-name')
        )
      ).size,
      5
    )

    const solRow = table.querySelector('[data-model-name="gpt-5.6-sol"]')
    assert.ok(solRow)
    const sitePrices = [...solRow.querySelectorAll('[data-site-price]')].map(
      (cell) => cell.textContent?.trim()
    )
    const officialPrices = [
      ...solRow.querySelectorAll('[data-official-price]'),
    ].map((cell) => cell.textContent?.trim())
    assert.deepEqual(sitePrices, [
      'Site price¥2.16',
      'Site price¥12.96',
      'Site price¥2.70',
      'Site price¥0.216',
    ])
    assert.deepEqual(officialPrices, [
      'Official price¥36.00',
      'Official price¥216.00',
      'Official price¥45.00',
      'Official price¥3.60',
    ])
    assert.match(solRow.textContent ?? '', /Save 94%/)
    const sitePriceValues = solRow.querySelectorAll('[data-site-price-value]')
    assert.equal(sitePriceValues.length, 4)
    for (const price of sitePriceValues) {
      assert.match(price.className, /text-amber-/)
      assert.match(price.className, /font-bold/)
    }
    const officialPriceValues = solRow.querySelectorAll(
      '[data-official-price-value]'
    )
    assert.equal(officialPriceValues.length, 4)
    for (const price of officialPriceValues) {
      assert.match(price.className, /line-through/)
    }
    assert.doesNotMatch(
      solRow.querySelector('[data-official-price]')?.firstElementChild
        ?.className ?? '',
      /line-through/
    )
    const savingsBadge = solRow.querySelector('[data-pricing-savings-badge]')
    assert.ok(savingsBadge)
    assert.match(savingsBadge.className, /bg-emerald-/)
    assert.match(savingsBadge.className, /border-emerald-/)
    assert.match(savingsBadge.textContent ?? '', /Save 94%/)
    assert.doesNotMatch(table.textContent ?? '', /NaN/)

    await click(solRow)
    assert.deepEqual(detailClicks, ['gpt-5.6-sol'])

    await act(async () => {
      solRow.dispatchEvent(
        new KeyboardEvent('keydown', { key: 'Enter', bubbles: true })
      )
    })
    assert.deepEqual(detailClicks, ['gpt-5.6-sol', 'gpt-5.6-sol'])
  })

  test('keeps an incomplete filtered group in the table and hides comparison promotion', async () => {
    await renderTable(validComparisonModels().slice(0, 1))

    const table = container.querySelector('table')
    assert.ok(table)
    assert.equal(table.querySelectorAll('th').length, 6)
    assert.equal(table.querySelectorAll('tbody tr').length, 1)
    assert.doesNotMatch(container.textContent ?? '', /Official price: 6%/)
    assert.doesNotMatch(container.textContent ?? '', /Save 94%/)
  })

  test('derives the savings badge from the comparison result instead of a fixed value', async () => {
    await act(async () => {
      root.render(createElement(ComparisonSavingsBadge, { savingsPercent: 92 }))
    })

    const badge = container.querySelector('[data-pricing-savings-badge]')
    assert.ok(badge)
    assert.match(badge.className, /border-emerald-/)
    assert.match(badge.textContent ?? '', /Save 92%/)
    assert.doesNotMatch(badge.textContent ?? '', /94%/)
  })

  test('fails a partially invalid five-model group closed without dropping rows', async () => {
    const models = validComparisonModels()
    models[2] = { ...models[2], cache_ratio: null }
    await renderTable(models)

    const table = container.querySelector('table')
    assert.ok(table)
    assert.equal(table.querySelectorAll('tbody tr').length, 5)
    assert.doesNotMatch(container.textContent ?? '', /6% of official price/)
    assert.doesNotMatch(container.textContent ?? '', /Save 94%/)
    assert.doesNotMatch(container.textContent ?? '', /NaN/)
    const gpt55 = table.querySelector('[data-model-name="gpt-5.5"]')
    assert.ok(gpt55)
    assert.match(
      gpt55.querySelectorAll('td')[3]?.textContent ?? '',
      /Not applicable/
    )
    assert.equal(
      gpt55.querySelectorAll('[data-official-price-value]').length,
      3
    )
  })

  test('makes the horizontally scrollable table keyboard accessible', async () => {
    await renderTable(validComparisonModels())

    const region = container.querySelector('[data-pricing-table-scroll]')
    assert.ok(region)
    assert.equal(region.getAttribute('role'), 'region')
    assert.equal(region.getAttribute('tabindex'), '0')
    assert.match(region.getAttribute('aria-label') ?? '', /GPT-Pro/)
    assert.equal(container.querySelector('[data-horizontal-scroll-hint]'), null)
  })

  test('falls back every model in an incomplete comparison group to the normal table', () => {
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

  test('copies the desktop model ID without opening model details', async () => {
    const button = container.querySelector('button')
    assert.ok(button)

    await click(button)

    assert.deepEqual(copiedValues, ['gpt-5.6-sol'])
    assert.equal(parentClicks, 0)
  })

  test('does not expose model source links or verification dates', () => {
    assert.equal(container.querySelectorAll('a').length, 0)
    assert.doesNotMatch(container.textContent ?? '', /Price source/)
    assert.doesNotMatch(container.textContent ?? '', /Verified on/)
  })
})
