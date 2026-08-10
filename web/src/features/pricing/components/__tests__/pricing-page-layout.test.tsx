/* Copyright (C) 2023-2026 QuantumNous */
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { after, afterEach, before, beforeEach, describe, test } from 'node:test'

import { GlobalRegistrator } from '@happy-dom/global-registrator'
import { act, createElement } from 'react'
import { createRoot, type Root } from 'react-dom/client'

import i18n from '@/i18n/config'

import {
  ENDPOINT_TYPES,
  FILTER_ALL,
  QUOTA_TYPES,
  SORT_OPTIONS,
} from '../../constants'
import { normalizeViewMode } from '../../hooks/use-filters'
import { PUBLIC_COMPARISON_CATALOG } from '../../public-comparison-catalog'
import type { PricingModel, PricingVendor } from '../../types'
import type { PricingToolbarProps } from '../pricing-toolbar'

const VIEWPORT_WIDTHS = [1440, 1024, 390]

let container: HTMLDivElement
let root: Root
let PricingCatalogLayout: typeof import('../../index').PricingCatalogLayout

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

function toolbarProps(
  models: PricingModel[],
  overrides: Partial<PricingToolbarProps> = {}
): PricingToolbarProps {
  const ignoreString = (_value: string) => undefined
  const ignoreBoolean = (_value: boolean) => undefined
  return {
    filteredCount: models.length,
    totalCount: models.length,
    sortBy: SORT_OPTIONS.NAME,
    onSortChange: ignoreString,
    tokenUnit: 'M',
    onTokenUnitChange: () => undefined,
    showRechargePrice: false,
    onRechargePriceChange: ignoreBoolean,
    quotaTypeFilter: QUOTA_TYPES.ALL,
    endpointTypeFilter: ENDPOINT_TYPES.ALL,
    vendorFilter: FILTER_ALL,
    groupFilter: FILTER_ALL,
    tagFilter: FILTER_ALL,
    onQuotaTypeChange: ignoreString,
    onEndpointTypeChange: ignoreString,
    onVendorChange: ignoreString,
    onGroupChange: ignoreString,
    onTagChange: ignoreString,
    vendors: [{ id: 4, name: 'OpenAI' }],
    groups: ['default'],
    tags: [],
    models,
    hasActiveFilters: false,
    activeFilterCount: 0,
    onClearFilters: () => undefined,
    ...overrides,
  }
}

async function renderLayout(options?: {
  filteredModels?: PricingModel[]
  vendors?: PricingVendor[]
  toolbarOverrides?: Partial<PricingToolbarProps>
  onModelClick?: (modelName: string) => void
}) {
  const models = validComparisonModels()
  const vendors = options?.vendors ?? [
    { id: 4, name: 'OpenAI' },
    { id: 5, name: 'Anthropic' },
  ]
  await act(async () => {
    root.render(
      createElement(PricingCatalogLayout, {
        models,
        filteredModels: options?.filteredModels ?? models,
        vendors,
        searchInput: '',
        onSearchChange: () => undefined,
        onClearSearch: () => undefined,
        hasActiveFilters: options?.toolbarOverrides?.hasActiveFilters ?? false,
        onClearAll: () => undefined,
        toolbarProps: toolbarProps(models, options?.toolbarOverrides),
        onModelClick: options?.onModelClick ?? (() => undefined),
      })
    )
  })
}

async function click(element: Element) {
  await act(async () => {
    ;(element as HTMLElement).click()
    await new Promise((resolve) => setTimeout(resolve, 20))
  })
}

function findButton(label: string): HTMLButtonElement {
  const button = [...document.querySelectorAll('button')].find(
    (candidate) => candidate.textContent?.trim() === label
  )
  assert.ok(button, `button not found: ${label}`)
  return button
}

before(async () => {
  GlobalRegistrator.register({ url: 'http://localhost/pricing' })
  ;(
    globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }
  ).IS_REACT_ACT_ENVIRONMENT = true
  await i18n.changeLanguage('en')
  PricingCatalogLayout = (await import('../../index')).PricingCatalogLayout
})

beforeEach(() => {
  container = document.createElement('div')
  document.body.replaceChildren(container)
  root = createRoot(container)
})

afterEach(async () => {
  await act(async () => root?.unmount())
  container?.remove()
})

after(() => {
  GlobalRegistrator.unregister()
})

describe('public pricing page layout', () => {
  test('uses the same six-column table at 1440, 1024, and 390 pixels', async () => {
    for (const width of VIEWPORT_WIDTHS) {
      Object.defineProperty(window, 'innerWidth', {
        configurable: true,
        value: width,
      })
      await renderLayout()

      const tables = container.querySelectorAll('table')
      assert.equal(tables.length, 1, `table count at ${width}px`)
      assert.deepEqual(
        [...tables[0].querySelectorAll('th')].map((header) =>
          header.textContent?.trim()
        ),
        [
          'Model',
          'Input price',
          'Output price',
          'Cache create',
          'Cache read',
          'Savings',
        ]
      )
      const rows = tables[0].querySelectorAll('tbody tr')
      assert.equal(rows.length, 5)
      assert.equal(
        new Set([...rows].map((row) => row.getAttribute('data-model-name')))
          .size,
        5
      )
      assert.ok(container.querySelector('[data-pricing-six-column-table]'))
      assert.equal(container.querySelectorAll('aside').length, 0)
    }
  })

  test('projects only real categories and preserves the locked block order', async () => {
    await renderLayout()

    assert.deepEqual(
      [...container.querySelectorAll('[data-pricing-block]')].map((block) =>
        block.getAttribute('data-pricing-block')
      ),
      [
        'title',
        'supplier-navigation',
        'product-group-navigation',
        'catalog-controls',
        'catalog-table',
      ]
    )
    const orderedSelectors = [
      '[data-pricing-block="title"]',
      '[data-pricing-block="supplier-navigation"]',
      '[data-pricing-block="product-group-navigation"]',
      '[data-pricing-block="catalog-controls"]',
      '[data-pricing-group-heading]',
      '[data-pricing-group-badge]',
      '[data-pricing-group-description]',
      'table',
    ]
    const orderedElements = orderedSelectors.map((selector) => {
      const element = container.querySelector(selector)
      assert.ok(element, `missing ordered element: ${selector}`)
      return element
    })
    for (let index = 1; index < orderedElements.length; index += 1) {
      assert.ok(
        orderedElements[index - 1].compareDocumentPosition(
          orderedElements[index]
        ) & Node.DOCUMENT_POSITION_FOLLOWING
      )
    }
    assert.match(container.textContent ?? '', /OpenAI/)
    assert.doesNotMatch(container.textContent ?? '', /Anthropic/)
    assert.match(container.textContent ?? '', /GPT-Pro/)
    assert.match(container.textContent ?? '', /6% of official price/)
    assert.match(
      container.textContent ?? '',
      /OpenAI · Standard · Short Context · USD\/1M tokens · Verified on 2026-08-10/
    )

    const solRow = container.querySelector('[data-model-name="gpt-5.6-sol"]')
    assert.ok(solRow)
    assert.match(solRow.textContent ?? '', /\$0\.30/)
    assert.match(solRow.textContent ?? '', /\$1\.80/)
    assert.match(solRow.textContent ?? '', /\$0\.375/)
    assert.match(solRow.textContent ?? '', /\$0\.03/)
    assert.match(solRow.textContent ?? '', /Save about 94%/)
  })

  test('normalizes fresh and legacy view queries to table mode', () => {
    assert.equal(normalizeViewMode(undefined), 'table')
    assert.equal(normalizeViewMode('card'), 'table')
    assert.equal(normalizeViewMode('unknown'), 'table')
    assert.equal(normalizeViewMode('table'), 'table')
  })

  test('keeps the table while filters open, reset, and close', async () => {
    let clearCount = 0
    const clearFilters = () => {
      clearCount += 1
    }
    await renderLayout({
      toolbarOverrides: {
        hasActiveFilters: true,
        activeFilterCount: 1,
        onClearFilters: clearFilters,
      },
    })

    const filterButton = findButton('Filter1')
    assert.equal(filterButton.getAttribute('aria-expanded'), 'false')
    await click(filterButton)
    assert.equal(filterButton.getAttribute('aria-expanded'), 'true')
    assert.equal(container.querySelectorAll('table').length, 1)

    const toolbarSource = readFileSync(
      new URL('../pricing-toolbar.tsx', import.meta.url),
      'utf8'
    )
    assert.match(toolbarSource, /onClearFilters=\{props\.onClearFilters\}/)
    clearFilters()
    assert.equal(clearCount, 1)
    assert.equal(container.querySelectorAll('table').length, 1)

    await click(filterButton)
    assert.equal(filterButton.getAttribute('aria-expanded'), 'false')
    assert.equal(container.querySelectorAll('table').length, 1)
  })

  test('keeps an active-filter partial group in the same table without promotion', async () => {
    await renderLayout({
      filteredModels: validComparisonModels().slice(0, 1),
      toolbarOverrides: { hasActiveFilters: true, activeFilterCount: 1 },
    })

    const table = container.querySelector('table')
    assert.ok(table)
    assert.equal(table.querySelectorAll('th').length, 6)
    assert.equal(table.querySelectorAll('tbody tr').length, 1)
    assert.doesNotMatch(container.textContent ?? '', /Official price: 6%/)
    assert.doesNotMatch(container.textContent ?? '', /Save about 94%/)
  })

  test('keeps mobile scrolling accessible and removes legacy page composition', async () => {
    Object.defineProperty(window, 'innerWidth', {
      configurable: true,
      value: 390,
    })
    await renderLayout()

    const scrollRegion = container.querySelector('[data-pricing-table-scroll]')
    assert.ok(scrollRegion)
    assert.equal(scrollRegion.getAttribute('role'), 'region')
    assert.equal(scrollRegion.getAttribute('tabindex'), '0')
    assert.match(scrollRegion.getAttribute('aria-label') ?? '', /GPT-Pro/)
    assert.ok(container.querySelector('[data-horizontal-scroll-hint]'))
    assert.equal(
      container.querySelectorAll('input[aria-label="Search models"]').length,
      1
    )
    assert.ok(container.querySelector('[data-pricing-secondary-controls]'))
    assert.doesNotMatch(container.textContent ?? '', /Card view|Table view/)
    assert.doesNotMatch(container.textContent ?? '', /NaN/)

    const source = readFileSync(
      new URL('../../index.tsx', import.meta.url),
      'utf8'
    )
    assert.doesNotMatch(source, /ModelCardGrid/)
    assert.doesNotMatch(source, /<PricingSidebar/)
    assert.doesNotMatch(source, /viewMode ===/)
    assert.doesNotMatch(source, /radial-gradient/)
    assert.doesNotMatch(source, /Discover curated AI models/)
  })
})
