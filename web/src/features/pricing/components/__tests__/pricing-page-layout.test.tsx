/* Copyright (C) 2023-2026 QuantumNous */
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { after, afterEach, before, beforeEach, describe, test } from 'node:test'

import { GlobalRegistrator } from '@happy-dom/global-registrator'
import { act, createElement } from 'react'
import { createRoot, type Root } from 'react-dom/client'

import i18n from '@/i18n/config'

import { PUBLIC_COMPARISON_CATALOG } from '../../public-comparison-catalog'
import type { PricingModel, PricingVendor } from '../../types'

const VIEWPORT_WIDTHS = [1440, 1024, 390]
const OWNER_GROUP_DESCRIPTION =
  'GPTPro 低倍率自营号池，适合日常 Codex / API 编程请求；缓存写入按模型价格表展示'

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

async function renderLayout(options?: {
  models?: PricingModel[]
  vendors?: PricingVendor[]
  usdExchangeRate?: number
  onModelClick?: (modelName: string) => void
}) {
  await act(async () => {
    root.render(
      createElement(PricingCatalogLayout, {
        models: options?.models ?? validComparisonModels(),
        vendors: options?.vendors ?? [
          { id: 4, name: 'OpenAI' },
          { id: 5, name: 'Anthropic' },
        ],
        usdExchangeRate: options?.usdExchangeRate ?? 7.2,
        onModelClick: options?.onModelClick ?? (() => undefined),
      })
    )
  })
}

function assertSixColumnTable(width: number) {
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
      'Cache write',
      'Cache read',
      'Savings',
    ]
  )
  const rows = tables[0].querySelectorAll('tbody tr')
  assert.equal(rows.length, 5)
  assert.equal(
    new Set([...rows].map((row) => row.getAttribute('data-model-name'))).size,
    5
  )
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
  test('uses one accessible six-column table at every locked viewport', async () => {
    for (const width of VIEWPORT_WIDTHS) {
      Object.defineProperty(window, 'innerWidth', {
        configurable: true,
        value: width,
      })
      await renderLayout()
      assertSixColumnTable(width)

      const page = container.querySelector('[data-pricing-page]')
      const scrollRegion = container.querySelector(
        '[data-pricing-table-scroll]'
      )
      assert.ok(page)
      assert.match(page.className, /min-w-0/)
      assert.match(page.className, /max-w-full/)
      assert.ok(scrollRegion)
      assert.equal(scrollRegion.getAttribute('role'), 'region')
      assert.equal(scrollRegion.getAttribute('tabindex'), '0')
      assert.match(scrollRegion.className, /overflow-x-auto/)
      assert.equal(
        container.querySelector('[data-horizontal-scroll-hint]'),
        null
      )
    }
  })

  test('renders the locked text hierarchy and only non-empty taxonomy', async () => {
    await renderLayout()

    assert.deepEqual(
      [...container.querySelectorAll('[data-pricing-block]')].map((block) =>
        block.getAttribute('data-pricing-block')
      ),
      [
        'title',
        'supplier-navigation',
        'product-group-navigation',
        'catalog-table',
      ]
    )
    assert.match(container.textContent ?? '', /Model pricing/)
    assert.match(
      container.textContent ?? '',
      /Text models billed by multiplier · Image models billed per image/
    )
    assert.match(container.textContent ?? '', /OpenAI/)
    assert.doesNotMatch(container.textContent ?? '', /Anthropic/)
    assert.equal(
      container.querySelector('[data-pricing-group-heading]')?.textContent,
      'GPT-Pro'
    )
    assert.equal(
      container.querySelector('[data-pricing-group-description]')?.textContent,
      OWNER_GROUP_DESCRIPTION
    )
    assert.match(container.textContent ?? '', /6% of official price/)
  })

  test('uses one exchange rate for every site and official CNY price', async () => {
    await renderLayout({ usdExchangeRate: 7.2 })

    const solRow = container.querySelector('[data-model-name="gpt-5.6-sol"]')
    assert.ok(solRow)
    assert.deepEqual(
      [...solRow.querySelectorAll('[data-site-price]')].map((price) =>
        price.textContent?.trim()
      ),
      [
        'Site price¥2.16',
        'Site price¥12.96',
        'Site price¥2.70',
        'Site price¥0.216',
      ]
    )
    assert.deepEqual(
      [...solRow.querySelectorAll('[data-official-price]')].map((price) =>
        price.textContent?.trim()
      ),
      [
        'Official price¥36.00',
        'Official price¥216.00',
        'Official price¥45.00',
        'Official price¥3.60',
      ]
    )
    assert.doesNotMatch(container.textContent ?? '', /NaN|\$|free/i)
  })

  test('removes search, controls, counts, sources, dates, hints, and cards', async () => {
    await renderLayout()

    assert.equal(container.querySelectorAll('input').length, 0)
    assert.equal(
      container.querySelector('[data-pricing-secondary-controls]'),
      null
    )
    assert.equal(container.querySelectorAll('aside').length, 0)
    assert.doesNotMatch(
      container.textContent ?? '',
      /models enabled|Price source|Verified on|More|Card view|Table view/
    )

    const source = readFileSync(
      new URL('../../index.tsx', import.meta.url),
      'utf8'
    )
    assert.doesNotMatch(source, /SearchBar|PricingToolbar|useFilters/)
    assert.doesNotMatch(source, /ModelCardGrid|viewMode|filteredModels/)
  })

  test('keeps all new page labels translated in four locales', () => {
    const localeDirectory = new URL(
      '../../../../i18n/locales/',
      import.meta.url
    )
    for (const filename of ['zh.json', 'zh-TW.json', 'en.json', 'ru.json']) {
      const locale = JSON.parse(
        readFileSync(new URL(filename, localeDirectory), 'utf8')
      ) as { translation: Record<string, string> }
      for (const key of [
        'Model pricing',
        'Text models billed by multiplier · Image models billed per image',
        'Site price',
        'Official price',
      ]) {
        assert.ok(locale.translation[key], `${filename}: missing ${key}`)
      }
    }
  })
})
