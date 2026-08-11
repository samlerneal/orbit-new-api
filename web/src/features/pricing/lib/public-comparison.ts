/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import type {
  OfficialPriceDimension,
  PublicComparisonGroup,
  PublicComparisonModel,
} from '../public-comparison-catalog'
import type { PricingModel } from '../types'
import { calculateTokenPriceInUsd } from './price'

const RATIO_TOLERANCE = 0.000001

export type PublicPriceDimension =
  | 'input'
  | 'output'
  | 'cacheCreate'
  | 'cacheRead'

type PublicPrices = Record<PublicPriceDimension, number | null>

export type PublicComparisonResult = {
  group: PublicComparisonGroup
  catalogModel: PublicComparisonModel
  model: PricingModel
  savingsPercent: number
  salePrices: PublicPrices
  officialPrices: PublicPrices
}

export type PublicComparisonDisplayPlan = {
  totalPages: number
  currentPage: number
  pageModels: PricingModel[]
  comparisonResults: PublicComparisonResult[]
  visibleGroups: PublicComparisonGroup[]
  normalModels: PricingModel[]
}

function isOfficialPrice(
  value: OfficialPriceDimension
): value is Exclude<OfficialPriceDimension, 'not_applicable'> {
  return value !== 'not_applicable'
}

function getOfficialPriceInUsd(value: OfficialPriceDimension): number | null {
  if (!isOfficialPrice(value)) return null
  const amount = Number(value.amount)
  return Number.isFinite(amount) && amount > 0 ? amount : null
}

export function convertUsdToCny(
  value: number | null,
  usdExchangeRate: number
): number | null {
  if (
    value === null ||
    !Number.isFinite(value) ||
    value < 0 ||
    !Number.isFinite(usdExchangeRate) ||
    usdExchangeRate <= 0
  ) {
    return null
  }
  const valueInCny = value * usdExchangeRate
  return Number.isFinite(valueInCny) && valueInCny > 0 ? valueInCny : null
}

function calculateApplicableSalePrice(
  catalogPrice: OfficialPriceDimension,
  model: PricingModel,
  priceType: 'input' | 'output' | 'create_cache' | 'cache'
): number | null {
  if (!isOfficialPrice(catalogPrice)) {
    return null
  }
  return calculateTokenPriceInUsd(model, priceType, 1)
}

function isValidCatalogModel(
  catalogModel: PublicComparisonModel,
  model: PricingModel
): boolean {
  if (
    model.quota_type !== 0 ||
    model.billing_expr ||
    !model.enable_groups.includes(catalogModel.salesGroupId)
  ) {
    return false
  }
  const expectedRatio = Number(catalogModel.comparisonRatio)
  if (
    !Number.isFinite(expectedRatio) ||
    expectedRatio <= 0 ||
    expectedRatio > 1
  ) {
    return false
  }
  const dimensions = [
    ['input', 'input'],
    ['output', 'output'],
    ['cacheCreate', 'create_cache'],
    ['cacheRead', 'cache'],
  ] as const
  return dimensions.every(([catalogKey, priceType]) => {
    const official = catalogModel.officialPrices[catalogKey]
    if (!isOfficialPrice(official)) {
      return true
    }
    const actual = calculateTokenPriceInUsd(model, priceType, 1)
    const officialAmount = Number(official.amount)
    if (
      !Number.isFinite(actual) ||
      actual <= 0 ||
      !Number.isFinite(officialAmount) ||
      officialAmount <= 0
    ) {
      return false
    }
    return (
      Math.abs(actual / officialAmount - expectedRatio) <=
      Math.max(RATIO_TOLERANCE, expectedRatio * 0.001)
    )
  })
}

export function getPublicComparisonResults(
  models: PricingModel[],
  groups: PublicComparisonGroup[]
): PublicComparisonResult[] {
  const modelById = new Map(models.map((model) => [model.model_name, model]))
  return groups.flatMap((group) =>
    group.models.flatMap((catalogModel) => {
      const model = modelById.get(catalogModel.publicModelId)
      if (!model || !isValidCatalogModel(catalogModel, model)) return []
      return [
        {
          group,
          catalogModel,
          model,
          savingsPercent: Math.round(
            (1 - Number(catalogModel.comparisonRatio)) * 100
          ),
          salePrices: {
            input: calculateApplicableSalePrice(
              catalogModel.officialPrices.input,
              model,
              'input'
            ),
            output: calculateApplicableSalePrice(
              catalogModel.officialPrices.output,
              model,
              'output'
            ),
            cacheCreate: calculateApplicableSalePrice(
              catalogModel.officialPrices.cacheCreate,
              model,
              'create_cache'
            ),
            cacheRead: calculateApplicableSalePrice(
              catalogModel.officialPrices.cacheRead,
              model,
              'cache'
            ),
          },
          officialPrices: {
            input: getOfficialPriceInUsd(catalogModel.officialPrices.input),
            output: getOfficialPriceInUsd(catalogModel.officialPrices.output),
            cacheCreate: getOfficialPriceInUsd(
              catalogModel.officialPrices.cacheCreate
            ),
            cacheRead: getOfficialPriceInUsd(
              catalogModel.officialPrices.cacheRead
            ),
          },
        },
      ]
    })
  )
}

export function canShowGroupComparison(
  results: PublicComparisonResult[],
  group: PublicComparisonGroup
): boolean {
  return (
    results.filter((result) => result.group.groupId === group.groupId)
      .length === group.models.length &&
    group.models.every((model) => model.comparisonRatio === '0.06')
  )
}

export function getNormalModelsForVisibleComparisons(
  models: PricingModel[],
  results: PublicComparisonResult[],
  visibleGroups: PublicComparisonGroup[]
): PricingModel[] {
  const visibleGroupIds = new Set(visibleGroups.map((group) => group.groupId))
  const visibleComparisonModelNames = new Set(
    results
      .filter((result) => visibleGroupIds.has(result.group.groupId))
      .map((result) => result.model.model_name)
  )
  return models.filter(
    (model) => !visibleComparisonModelNames.has(model.model_name)
  )
}

export function getPublicComparisonDisplayPlan(
  models: PricingModel[],
  groups: PublicComparisonGroup[],
  requestedPage: number,
  pageSize: number
): PublicComparisonDisplayPlan {
  const totalPages = Math.max(1, Math.ceil(models.length / pageSize))
  const currentPage = Math.min(Math.max(1, requestedPage), totalPages)
  const start = (currentPage - 1) * pageSize
  const pageModels = models.slice(start, start + pageSize)
  const comparisonResults = getPublicComparisonResults(pageModels, groups)
  const visibleGroups = groups.filter((group) =>
    canShowGroupComparison(comparisonResults, group)
  )
  const normalModels = getNormalModelsForVisibleComparisons(
    pageModels,
    comparisonResults,
    visibleGroups
  )

  return {
    totalPages,
    currentPage,
    pageModels,
    comparisonResults,
    visibleGroups,
    normalModels,
  }
}

export function formatUsdPerMillion(value: number | null): string {
  if (value === null || !Number.isFinite(value)) return '—'
  return `$${value.toFixed(8).replace(/\.?0+$/, '')}`
}
