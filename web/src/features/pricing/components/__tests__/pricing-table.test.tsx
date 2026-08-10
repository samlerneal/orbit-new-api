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
import { PublicComparisonModelCell } from '../pricing-table'

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
  }
}

async function click(element: Element) {
  await act(async () => {
    element.dispatchEvent(
      new MouseEvent('click', { bubbles: true, cancelable: true })
    )
    await Promise.resolve()
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
            result: comparisonResult(index),
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

  test('exposes one safe official source link for each public model', async () => {
    const links = [...container.querySelectorAll('a')]
    assert.equal(links.length, 5)
    assert.deepEqual(
      links.map((link) => link.getAttribute('href')),
      PUBLIC_COMPARISON_CATALOG[0].models.map(
        (model) => model.officialPriceSourceUrl
      )
    )
    for (const link of links) {
      assert.equal(link.target, '_blank')
      assert.equal(link.rel, 'noopener noreferrer')
    }

    links[0].addEventListener('click', (event) => event.preventDefault(), {
      once: true,
    })
    await click(links[0])
    assert.equal(parentClicks, 0)
  })
})
