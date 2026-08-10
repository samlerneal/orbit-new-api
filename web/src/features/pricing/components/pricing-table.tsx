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
import type { Row, PaginationState } from '@tanstack/react-table'
import { Copy } from 'lucide-react'
import { useState, useCallback } from 'react'
import { useTranslation } from 'react-i18next'

import {
  DataTablePagination,
  DataTableRow,
  DataTableView,
  useDataTable,
} from '@/components/data-table'
import { useCopyToClipboard } from '@/hooks/use-copy-to-clipboard'

import { DEFAULT_PRICING_PAGE_SIZE, DEFAULT_TOKEN_UNIT } from '../constants'
import {
  canShowGroupComparison,
  formatUsdPerMillion,
  getNormalModelsForVisibleComparisons,
  getPublicComparisonResults,
  type PublicComparisonResult,
} from '../lib/public-comparison'
import { PUBLIC_COMPARISON_CATALOG } from '../public-comparison-catalog'
import type { PricingModel, TokenUnit } from '../types'
import {
  PUBLIC_COMPARISON_COLUMN_KEYS,
  usePricingColumns,
} from './pricing-columns'

export interface PricingTableProps {
  models: PricingModel[]
  isLoading?: boolean
  priceRate?: number
  usdExchangeRate?: number
  tokenUnit?: TokenUnit
  showRechargePrice?: boolean
  selectedGroup?: string
  onModelClick?: (modelName: string) => void
}

export function PublicComparisonModelCell({
  result,
}: {
  result: PublicComparisonResult
}) {
  const { t } = useTranslation()
  const { copyToClipboard } = useCopyToClipboard()
  const modelName = result.model.model_name

  const handleCopy = (event: React.MouseEvent<HTMLButtonElement>) => {
    event.stopPropagation()
    void copyToClipboard(modelName)
  }

  return (
    <div className='min-w-0'>
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
      <a
        href={result.catalogModel.officialPriceSourceUrl}
        target='_blank'
        rel='noopener noreferrer'
        onClick={(event) => event.stopPropagation()}
        className='text-muted-foreground mt-1 block text-[11px] underline'
      >
        {t('Price source')} ·{' '}
        {t('Verified on {{date}}', {
          date: result.catalogModel.sourceObservedAt,
        })}
      </a>
    </div>
  )
}

export function PricingTable(props: PricingTableProps) {
  const { t } = useTranslation()
  const {
    models,
    isLoading = false,
    priceRate = 1,
    usdExchangeRate = 1,
    tokenUnit = DEFAULT_TOKEN_UNIT,
    showRechargePrice = false,
    selectedGroup,
    onModelClick,
  } = props

  const [pagination, setPagination] = useState<PaginationState>({
    pageIndex: 0,
    pageSize: DEFAULT_PRICING_PAGE_SIZE,
  })

  const columns = usePricingColumns({
    tokenUnit,
    priceRate,
    usdExchangeRate,
    showRechargePrice,
    selectedGroup,
  })
  const comparisonResults = getPublicComparisonResults(
    models,
    PUBLIC_COMPARISON_CATALOG
  )
  const visibleGroups = PUBLIC_COMPARISON_CATALOG.filter((group) =>
    canShowGroupComparison(comparisonResults, group)
  )
  const normalModels = getNormalModelsForVisibleComparisons(
    models,
    comparisonResults,
    visibleGroups
  )

  const { table } = useDataTable({
    data: normalModels,
    columns,
    pageCount: Math.ceil(normalModels.length / pagination.pageSize),
    pagination,
    onPaginationChange: setPagination,
    manualPagination: false,
    withFilteredRowModel: false,
    withSortedRowModel: false,
    withFacetedRowModel: false,
  })

  const handleRowClick = useCallback(
    (model: PricingModel) => {
      onModelClick?.(model.model_name)
    },
    [onModelClick]
  )

  return (
    <div className='space-y-4'>
      {visibleGroups.map((group) => (
        <section
          key={group.groupId}
          className='overflow-hidden rounded-lg border'
        >
          <div className='bg-muted/40 flex flex-wrap items-center justify-between gap-2 px-4 py-3'>
            <h2 className='font-semibold'>{group.displayName}</h2>
            <span className='text-muted-foreground text-sm'>
              {t('Official price: 6%')}
            </span>
          </div>
          <div className='overflow-x-auto'>
            <table className='w-full min-w-[720px] text-sm'>
              <thead className='text-muted-foreground border-y text-left'>
                <tr>
                  {PUBLIC_COMPARISON_COLUMN_KEYS.map((key) => (
                    <th key={key} className='p-3'>
                      {t(key)}
                    </th>
                  ))}
                </tr>
              </thead>
              <tbody>
                {comparisonResults
                  .filter((result) => result.group.groupId === group.groupId)
                  .map((result) => (
                    <tr
                      key={result.model.model_name}
                      className='hover:bg-muted/30 cursor-pointer border-b'
                      onClick={() => handleRowClick(result.model)}
                    >
                      <td className='p-3'>
                        <PublicComparisonModelCell result={result} />
                      </td>
                      <td className='p-3 font-mono'>
                        {formatUsdPerMillion(result.salePrices.input)}
                      </td>
                      <td className='p-3 font-mono'>
                        {formatUsdPerMillion(result.salePrices.output)}
                      </td>
                      <td className='p-3 font-mono'>
                        {result.salePrices.cacheCreate === null
                          ? t('Not applicable')
                          : formatUsdPerMillion(result.salePrices.cacheCreate)}
                      </td>
                      <td className='p-3 font-mono'>
                        {result.salePrices.cacheRead === null
                          ? t('Not applicable')
                          : formatUsdPerMillion(result.salePrices.cacheRead)}
                      </td>
                      <td className='p-3'>
                        {t('Save about {{percent}}%', {
                          percent: result.savingsPercent,
                        })}
                      </td>
                    </tr>
                  ))}
              </tbody>
            </table>
          </div>
        </section>
      ))}
      {normalModels.length > 0 && (
        <DataTableView
          table={table}
          isLoading={isLoading}
          emptyTitle={t('No Models Found')}
          emptyDescription={t('No models match your current filters.')}
          skeletonKeyPrefix='pricing-skeleton'
          applyHeaderSize
          getColumnClassName={(_columnId, kind) =>
            kind === 'header' ? 'text-muted-foreground font-medium' : undefined
          }
          renderRow={(row: Row<PricingModel>) => (
            <DataTableRow
              key={row.id}
              row={row}
              className='hover:bg-muted/30 cursor-pointer transition-colors'
              onClick={() => handleRowClick(row.original)}
            />
          )}
        />
      )}

      {!isLoading && normalModels.length > 0 && (
        <DataTablePagination table={table} />
      )}
    </div>
  )
}
