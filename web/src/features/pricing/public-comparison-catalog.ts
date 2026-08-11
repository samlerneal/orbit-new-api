/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/

export type OfficialPrice = {
  amount: string
  currency: 'USD'
  perTokens: 1000000
}

export type OfficialPriceDimension = OfficialPrice | 'not_applicable'

export type PublicComparisonModel = {
  publicModelId: string
  canonicalModelId: string
  mappingEvidenceType: 'OWNER_PROVIDED_PRODUCTION_CONFIGURATION_ATTESTATION'
  mappingObservedAt: string
  salesGroupId: string
  officialPriceSourceUrl: string
  sourceObservedAt: string
  officialEffectiveAt?: string
  serviceTier: 'standard'
  contextTier: 'short'
  comparisonRatio: string
  officialPrices: {
    input: OfficialPriceDimension
    output: OfficialPriceDimension
    cacheCreate: OfficialPriceDimension
    cacheRead: OfficialPriceDimension
  }
}

export type PublicComparisonGroup = {
  groupId: string
  displayName: string
  description: string
  models: PublicComparisonModel[]
}

const modelSourceUrl = (modelId: string) =>
  `https://developers.openai.com/api/docs/models/${modelId}`
const SOURCE_OBSERVED_AT = '2026-08-10'
const MAPPING_OBSERVED_AT = '2026-08-10'
const PRICE = (amount: string): OfficialPrice => ({
  amount,
  currency: 'USD',
  perTokens: 1000000,
})

export const PUBLIC_COMPARISON_CATALOG: PublicComparisonGroup[] = [
  {
    groupId: 'gpt-pro',
    displayName: 'GPT-Pro',
    description:
      'GPTPro 低倍率自营号池，适合日常 Codex / API 编程请求；缓存写入按模型价格表展示',
    models: [
      {
        publicModelId: 'gpt-5.6-sol',
        canonicalModelId: 'gpt-5.6-sol',
        mappingEvidenceType:
          'OWNER_PROVIDED_PRODUCTION_CONFIGURATION_ATTESTATION',
        mappingObservedAt: MAPPING_OBSERVED_AT,
        salesGroupId: 'default',
        officialPriceSourceUrl: modelSourceUrl('gpt-5.6-sol'),
        sourceObservedAt: SOURCE_OBSERVED_AT,
        serviceTier: 'standard',
        contextTier: 'short',
        comparisonRatio: '0.06',
        officialPrices: {
          input: PRICE('5.00'),
          output: PRICE('30.00'),
          cacheCreate: PRICE('6.25'),
          cacheRead: PRICE('0.50'),
        },
      },
      {
        publicModelId: 'gpt-5.5',
        canonicalModelId: 'gpt-5.5',
        mappingEvidenceType:
          'OWNER_PROVIDED_PRODUCTION_CONFIGURATION_ATTESTATION',
        mappingObservedAt: MAPPING_OBSERVED_AT,
        salesGroupId: 'default',
        officialPriceSourceUrl: modelSourceUrl('gpt-5.5'),
        sourceObservedAt: SOURCE_OBSERVED_AT,
        serviceTier: 'standard',
        contextTier: 'short',
        comparisonRatio: '0.06',
        officialPrices: {
          input: PRICE('5.00'),
          output: PRICE('30.00'),
          cacheCreate: 'not_applicable',
          cacheRead: PRICE('0.50'),
        },
      },
      {
        publicModelId: 'gpt-5.6-terra',
        canonicalModelId: 'gpt-5.6-terra',
        mappingEvidenceType:
          'OWNER_PROVIDED_PRODUCTION_CONFIGURATION_ATTESTATION',
        mappingObservedAt: MAPPING_OBSERVED_AT,
        salesGroupId: 'default',
        officialPriceSourceUrl: modelSourceUrl('gpt-5.6-terra'),
        sourceObservedAt: SOURCE_OBSERVED_AT,
        serviceTier: 'standard',
        contextTier: 'short',
        comparisonRatio: '0.06',
        officialPrices: {
          input: PRICE('2.00'),
          output: PRICE('12.00'),
          cacheCreate: PRICE('2.50'),
          cacheRead: PRICE('0.20'),
        },
      },
      {
        publicModelId: 'gpt-5.6-luna',
        canonicalModelId: 'gpt-5.6-luna',
        mappingEvidenceType:
          'OWNER_PROVIDED_PRODUCTION_CONFIGURATION_ATTESTATION',
        mappingObservedAt: MAPPING_OBSERVED_AT,
        salesGroupId: 'default',
        officialPriceSourceUrl: modelSourceUrl('gpt-5.6-luna'),
        sourceObservedAt: SOURCE_OBSERVED_AT,
        serviceTier: 'standard',
        contextTier: 'short',
        comparisonRatio: '0.06',
        officialPrices: {
          input: PRICE('0.20'),
          output: PRICE('1.20'),
          cacheCreate: PRICE('0.25'),
          cacheRead: PRICE('0.02'),
        },
      },
      {
        publicModelId: 'gpt-5.4-mini',
        canonicalModelId: 'gpt-5.4-mini',
        mappingEvidenceType:
          'OWNER_PROVIDED_PRODUCTION_CONFIGURATION_ATTESTATION',
        mappingObservedAt: MAPPING_OBSERVED_AT,
        salesGroupId: 'default',
        officialPriceSourceUrl: modelSourceUrl('gpt-5.4-mini'),
        sourceObservedAt: SOURCE_OBSERVED_AT,
        serviceTier: 'standard',
        contextTier: 'short',
        comparisonRatio: '0.06',
        officialPrices: {
          input: PRICE('0.75'),
          output: PRICE('4.50'),
          cacheCreate: 'not_applicable',
          cacheRead: PRICE('0.075'),
        },
      },
    ],
  },
]
