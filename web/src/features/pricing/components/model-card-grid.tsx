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
import { useQuery } from '@tanstack/react-query'
import { ChevronLeft, ChevronRight } from 'lucide-react'
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { getPerfMetricsSummary } from '@/features/performance-metrics/api'

import { DEFAULT_PRICING_PAGE_SIZE, DEFAULT_TOKEN_UNIT } from '../constants'
import { getPublicComparisonDisplayPlan } from '../lib/public-comparison'
import { PUBLIC_COMPARISON_CATALOG } from '../public-comparison-catalog'
import type { PricingModel, TokenUnit } from '../types'
import { ModelCard } from './model-card'
import type { ModelPerfBadgeData } from './model-perf-badge'

export interface ModelCardGridProps {
  models: PricingModel[]
  onModelClick: (modelName: string) => void
  priceRate?: number
  usdExchangeRate?: number
  tokenUnit?: TokenUnit
  showRechargePrice?: boolean
  selectedGroup?: string
}

export function ModelCardGrid(props: ModelCardGridProps) {
  const { t } = useTranslation()
  const [page, setPage] = useState(1)
  const pageSize = DEFAULT_PRICING_PAGE_SIZE
  const tokenUnit = props.tokenUnit ?? DEFAULT_TOKEN_UNIT

  const perfQuery = useQuery({
    queryKey: ['perf-metrics-summary', 24],
    queryFn: () => getPerfMetricsSummary(24),
    staleTime: 60 * 1000,
    retry: false,
  })

  const perfMap = useMemo(() => {
    const map = new Map<string, ModelPerfBadgeData>()
    for (const model of perfQuery.data?.data?.models ?? []) {
      map.set(model.model_name, model)
    }
    return map
  }, [perfQuery.data])
  const displayPlan = useMemo(
    () =>
      getPublicComparisonDisplayPlan(
        props.models,
        PUBLIC_COMPARISON_CATALOG,
        page,
        pageSize
      ),
    [page, pageSize, props.models]
  )
  const {
    totalPages,
    currentPage,
    comparisonResults,
    visibleGroups,
    normalModels,
  } = displayPlan

  if (props.models.length === 0) {
    return null
  }

  return (
    <div className='space-y-4 sm:space-y-5'>
      {visibleGroups.map((group) => (
        <section key={group.groupId} className='space-y-3'>
          <div className='flex flex-wrap items-baseline gap-x-2 gap-y-1'>
            <h2 className='text-base font-semibold'>{group.displayName}</h2>
            <span className='text-muted-foreground text-sm'>
              {t('Official price: 6%')}
            </span>
          </div>
          <div className='grid grid-cols-1 gap-3 sm:gap-4 md:grid-cols-2 lg:grid-cols-3'>
            {comparisonResults
              .filter((result) => result.group.groupId === group.groupId)
              .map((result) => (
                <ModelCard
                  key={result.model.id ?? result.model.model_name}
                  model={result.model}
                  comparison={result}
                  tokenUnit={tokenUnit}
                  priceRate={props.priceRate}
                  usdExchangeRate={props.usdExchangeRate}
                  showRechargePrice={props.showRechargePrice}
                  selectedGroup={props.selectedGroup}
                  perf={perfMap.get(result.model.model_name || '')}
                  onClick={() =>
                    props.onModelClick(result.model.model_name || '')
                  }
                />
              ))}
          </div>
        </section>
      ))}
      <div className='grid grid-cols-1 gap-3 sm:gap-4 md:grid-cols-2 lg:grid-cols-3'>
        {normalModels.map((model) => (
          <ModelCard
            key={model.id ?? model.model_name}
            model={model}
            tokenUnit={tokenUnit}
            priceRate={props.priceRate}
            usdExchangeRate={props.usdExchangeRate}
            showRechargePrice={props.showRechargePrice}
            selectedGroup={props.selectedGroup}
            perf={perfMap.get(model.model_name || '')}
            onClick={() => props.onModelClick(model.model_name || '')}
          />
        ))}
      </div>

      {totalPages > 1 && (
        <div className='text-muted-foreground flex flex-col items-center justify-between gap-3 border-t px-4 py-3 text-sm sm:flex-row'>
          <p className='text-muted-foreground'>
            {t('Page {{current}} of {{total}}', {
              current: currentPage,
              total: totalPages,
            })}
          </p>
          <div className='flex items-center gap-2'>
            <Button
              type='button'
              variant='outline'
              size='sm'
              onClick={() => setPage(Math.max(1, currentPage - 1))}
              disabled={currentPage <= 1}
              className='gap-1.5'
            >
              <ChevronLeft className='size-4' />
              {t('Previous page')}
            </Button>
            <Button
              type='button'
              variant='outline'
              size='sm'
              onClick={() => setPage(Math.min(totalPages, currentPage + 1))}
              disabled={currentPage >= totalPages}
              className='gap-1.5'
            >
              {t('Next page')}
              <ChevronRight className='size-4' />
            </Button>
          </div>
        </div>
      )}
    </div>
  )
}
