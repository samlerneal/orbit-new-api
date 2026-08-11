/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import { Copy } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { useCopyToClipboard } from '@/hooks/use-copy-to-clipboard'

import {
  canShowGroupComparison,
  convertUsdToCny,
  getPublicComparisonResults,
  type PublicPriceDimension,
  type PublicComparisonResult,
} from '../lib/public-comparison'
import {
  PUBLIC_COMPARISON_CATALOG,
  type PublicComparisonGroup,
} from '../public-comparison-catalog'
import type { PricingModel } from '../types'
import { PUBLIC_COMPARISON_COLUMN_KEYS } from './pricing-columns'

export interface PricingTableProps {
  models: PricingModel[]
  groups?: PublicComparisonGroup[]
  usdExchangeRate: number
  onModelClick?: (modelName: string) => void
}

export function PublicComparisonModelCell({ model }: { model: PricingModel }) {
  const { t } = useTranslation()
  const { copyToClipboard } = useCopyToClipboard()
  const modelName = model.model_name

  const handleCopy = (event: React.MouseEvent<HTMLButtonElement>) => {
    event.stopPropagation()
    void copyToClipboard(modelName)
  }

  return (
    <div className='min-w-48'>
      <div className='flex items-center gap-1'>
        <span className='truncate font-mono font-medium'>{modelName}</span>
        <button
          type='button'
          onClick={handleCopy}
          className='text-muted-foreground hover:text-foreground hover:bg-muted shrink-0 rounded p-1 transition-colors'
          aria-label={`${t('Copy')} ${modelName}`}
          title={t('Copy')}
        >
          <Copy className='size-3.5' />
        </button>
      </div>
    </div>
  )
}

function GroupDescription({ group }: { group: PublicComparisonGroup }) {
  return (
    <p
      className='text-muted-foreground text-xs sm:text-sm'
      data-pricing-group-description
    >
      {group.description}
    </p>
  )
}

function formatCnyPerMillion(
  valueInUsd: number | null,
  usdExchangeRate: number
): string | null {
  const valueInCny = convertUsdToCny(valueInUsd, usdExchangeRate)
  if (valueInCny === null) return null
  return valueInCny.toLocaleString('zh-CN', {
    style: 'currency',
    currency: 'CNY',
    minimumFractionDigits: 2,
    maximumFractionDigits: 6,
  })
}

function ComparisonPriceCell(props: {
  result?: PublicComparisonResult
  dimension: PublicPriceDimension
  usdExchangeRate: number
}) {
  const { t } = useTranslation()
  if (!props.result) {
    return <span aria-label={t('Not applicable')}>—</span>
  }

  const sitePrice = formatCnyPerMillion(
    props.result.salePrices[props.dimension],
    props.usdExchangeRate
  )
  const officialPrice = formatCnyPerMillion(
    props.result.officialPrices[props.dimension],
    props.usdExchangeRate
  )
  return sitePrice === null || officialPrice === null ? (
    <span>{t('Not applicable')}</span>
  ) : (
    <div className='space-y-1 leading-tight'>
      <div data-site-price>
        <span className='text-muted-foreground mr-1 font-sans text-[11px]'>
          {t('Site price')}
        </span>
        <span>{sitePrice}</span>
      </div>
      <div className='text-muted-foreground' data-official-price>
        <span className='mr-1 font-sans text-[11px]'>
          {t('Official price')}
        </span>
        <span>{officialPrice}</span>
      </div>
    </div>
  )
}

function ComparisonTableHeader() {
  const { t } = useTranslation()
  return (
    <thead className='text-muted-foreground bg-muted/40 border-b text-left'>
      <tr>
        {PUBLIC_COMPARISON_COLUMN_KEYS.map((key, index) => (
          <th
            key={key}
            scope='col'
            className={
              index === 0
                ? 'bg-muted/95 sticky left-0 z-20 min-w-52 p-3'
                : 'p-3 whitespace-nowrap'
            }
          >
            {t(key)}
          </th>
        ))}
      </tr>
    </thead>
  )
}

function ComparisonTableRow(props: {
  model: PricingModel
  result?: PublicComparisonResult
  comparisonVisible: boolean
  usdExchangeRate: number
  onModelClick?: (modelName: string) => void
}) {
  const { t } = useTranslation()
  const openDetails = () => props.onModelClick?.(props.model.model_name)
  const handleKeyDown = (event: React.KeyboardEvent<HTMLTableRowElement>) => {
    if (event.target !== event.currentTarget) return
    if (event.key !== 'Enter' && event.key !== ' ') return
    event.preventDefault()
    openDetails()
  }

  return (
    <tr
      tabIndex={0}
      aria-label={`${props.model.model_name} ${t('Details')}`}
      data-model-name={props.model.model_name}
      className='hover:bg-muted/30 cursor-pointer border-b last:border-b-0'
      onClick={openDetails}
      onKeyDown={handleKeyDown}
    >
      <td className='bg-background sticky left-0 z-10 p-3'>
        <PublicComparisonModelCell model={props.model} />
      </td>
      {(['input', 'output', 'cacheCreate', 'cacheRead'] as const).map(
        (dimension) => (
          <td key={dimension} className='p-3 font-mono whitespace-nowrap'>
            <ComparisonPriceCell
              result={props.result}
              dimension={dimension}
              usdExchangeRate={props.usdExchangeRate}
            />
          </td>
        )
      )}
      <td className='p-3 whitespace-nowrap'>
        {props.comparisonVisible && props.result
          ? t('Save about {{percent}}%', {
              percent: props.result.savingsPercent,
            })
          : '—'}
      </td>
    </tr>
  )
}

function PricingGroupTable(props: {
  group: PublicComparisonGroup
  models: PricingModel[]
  results: PublicComparisonResult[]
  comparisonVisible: boolean
  usdExchangeRate: number
  onModelClick?: (modelName: string) => void
}) {
  const { t } = useTranslation()
  const resultByModel = new Map(
    props.results.map((result) => [result.model.model_name, result])
  )

  return (
    <section
      className='space-y-3'
      data-pricing-group={props.group.groupId}
      data-comparison-ready={props.comparisonVisible ? 'true' : 'false'}
    >
      <div className='space-y-1'>
        <div className='flex flex-wrap items-start justify-between gap-3'>
          <h2 className='text-lg font-semibold' data-pricing-group-heading>
            {props.group.displayName}
          </h2>
          {props.comparisonVisible && (
            <span
              className='bg-primary/10 text-primary rounded-full px-3 py-1 text-sm font-semibold'
              data-pricing-group-badge
            >
              {t('Official price: 6%')}
            </span>
          )}
        </div>
        <GroupDescription group={props.group} />
      </div>

      <div
        role='region'
        tabIndex={0}
        aria-label={`${props.group.displayName} ${t('Model')}`}
        className='focus-visible:ring-primary/40 overflow-x-auto rounded-xl border focus-visible:ring-2 focus-visible:outline-none'
        data-pricing-table-scroll
      >
        <table className='w-full min-w-[900px] text-sm'>
          <ComparisonTableHeader />
          <tbody>
            {props.models.map((model) => {
              const result = resultByModel.get(model.model_name)
              return (
                <ComparisonTableRow
                  key={model.model_name}
                  model={model}
                  result={result}
                  comparisonVisible={props.comparisonVisible}
                  usdExchangeRate={props.usdExchangeRate}
                  onModelClick={props.onModelClick}
                />
              )
            })}
          </tbody>
        </table>
      </div>
    </section>
  )
}

export function PricingTable({
  models,
  groups = PUBLIC_COMPARISON_CATALOG,
  usdExchangeRate,
  onModelClick,
}: PricingTableProps) {
  const comparisonResults = getPublicComparisonResults(models, groups)
  const visibleGroupIds = new Set(
    groups
      .filter((group) => canShowGroupComparison(comparisonResults, group))
      .map((group) => group.groupId)
  )
  const modelByName = new Map(models.map((model) => [model.model_name, model]))

  return (
    <div className='space-y-6' data-pricing-six-column-table>
      {groups.map((group) => {
        const groupModels = group.models.flatMap((catalogModel) => {
          const model = modelByName.get(catalogModel.publicModelId)
          return model ? [model] : []
        })
        if (groupModels.length === 0) return null

        return (
          <PricingGroupTable
            key={group.groupId}
            group={group}
            models={groupModels}
            results={comparisonResults.filter(
              (result) => result.group.groupId === group.groupId
            )}
            comparisonVisible={visibleGroupIds.has(group.groupId)}
            usdExchangeRate={usdExchangeRate}
            onModelClick={onModelClick}
          />
        )
      })}
    </div>
  )
}
