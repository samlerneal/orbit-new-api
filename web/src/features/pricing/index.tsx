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
import { useCallback, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { PublicLayout } from '@/components/layout'
import { PageTransition } from '@/components/page-transition'

import {
  LoadingSkeleton,
  EmptyState,
  SearchBar,
  PricingTable,
  PricingToolbar,
  ModelDetailsDrawer,
} from './components'
import type { PricingToolbarProps } from './components/pricing-toolbar'
import { useFilters } from './hooks/use-filters'
import { usePricingData } from './hooks/use-pricing-data'
import { extractAllTags } from './lib/filters'
import {
  canShowGroupComparison,
  getPublicComparisonResults,
} from './lib/public-comparison'
import {
  PUBLIC_COMPARISON_CATALOG,
  type PublicComparisonGroup,
} from './public-comparison-catalog'
import type { PricingModel, PricingVendor } from './types'

type CatalogProjection = {
  vendor: PricingVendor
  group: PublicComparisonGroup
}

const EMPTY_MODELS: PricingModel[] = []
const EMPTY_VENDORS: PricingVendor[] = []

function getCatalogProjection(
  models: PricingModel[],
  vendors: PricingVendor[]
): CatalogProjection[] {
  const results = getPublicComparisonResults(models, PUBLIC_COMPARISON_CATALOG)
  const vendorById = new Map(vendors.map((vendor) => [vendor.id, vendor]))

  return PUBLIC_COMPARISON_CATALOG.flatMap((group) => {
    if (!canShowGroupComparison(results, group)) return []

    const groupResults = results.filter(
      (result) => result.group.groupId === group.groupId
    )
    const vendorIds = new Set(
      groupResults.map((result) => result.model.vendor_id)
    )
    if (vendorIds.size !== 1) return []

    const vendorId = [...vendorIds][0]
    const vendor = vendorId == null ? undefined : vendorById.get(vendorId)
    return vendor ? [{ vendor, group }] : []
  })
}

export interface PricingCatalogLayoutProps {
  models: PricingModel[]
  filteredModels: PricingModel[]
  vendors: PricingVendor[]
  searchInput: string
  onSearchChange: (value: string) => void
  onClearSearch: () => void
  hasActiveFilters: boolean
  onClearAll: () => void
  toolbarProps: Omit<
    PricingToolbarProps,
    'filteredCount' | 'totalCount' | 'vendors' | 'groups' | 'tags' | 'models'
  >
  onModelClick: (modelName: string) => void
}

function SupplierNavigation({ suppliers }: { suppliers: PricingVendor[] }) {
  const { t } = useTranslation()
  if (suppliers.length === 0) return null

  return (
    <nav
      aria-label={t('All Vendors')}
      className='flex flex-wrap gap-2'
      data-pricing-block='supplier-navigation'
    >
      {suppliers.map((vendor) => (
        <span
          key={vendor.id}
          aria-current='page'
          className='bg-foreground text-background rounded-full px-4 py-2 text-sm font-semibold'
        >
          {vendor.name}
        </span>
      ))}
    </nav>
  )
}

function ProductGroupNavigation({
  groups,
}: {
  groups: PublicComparisonGroup[]
}) {
  const { t } = useTranslation()
  if (groups.length === 0) return null

  return (
    <nav
      aria-label={t('Groups')}
      className='flex flex-wrap gap-2 border-b pb-3'
      data-pricing-block='product-group-navigation'
    >
      {groups.map((group) => (
        <span
          key={group.groupId}
          aria-current='page'
          className='border-primary text-foreground border-b-2 px-3 py-2 text-sm font-semibold'
        >
          {group.displayName.split(' · ')[0]}
        </span>
      ))}
    </nav>
  )
}

export function PricingCatalogLayout(props: PricingCatalogLayoutProps) {
  const { t } = useTranslation()
  const projection = useMemo(
    () => getCatalogProjection(props.models, props.vendors),
    [props.models, props.vendors]
  )
  const suppliers = [
    ...new Map(
      projection.map((item) => [item.vendor.id, item.vendor])
    ).values(),
  ]
  const groups = projection.map((item) => item.group)
  const catalogModelIds = new Set(
    groups.flatMap((group) =>
      group.models.map((catalogModel) => catalogModel.publicModelId)
    )
  )
  const catalogModels = props.models.filter((model) =>
    catalogModelIds.has(model.model_name)
  )
  const tableModels = props.filteredModels.filter((model) =>
    catalogModelIds.has(model.model_name)
  )
  const catalogSalesGroups = [
    ...new Set(catalogModels.flatMap((model) => model.enable_groups)),
  ]

  return (
    <div className='space-y-5'>
      <header className='space-y-2' data-pricing-block='title'>
        <h1 className='text-3xl font-bold tracking-tight sm:text-4xl'>
          {t('Model Square')}
        </h1>
        <p className='text-muted-foreground max-w-3xl text-sm sm:text-base'>
          {t('This site currently has {{count}} models enabled', {
            count: props.models.length,
          })}
        </p>
      </header>

      <SupplierNavigation suppliers={suppliers} />
      <ProductGroupNavigation groups={groups} />

      <div
        className='grid gap-3 lg:grid-cols-[minmax(260px,1fr)_auto]'
        data-pricing-block='catalog-controls'
      >
        <SearchBar
          value={props.searchInput}
          onChange={props.onSearchChange}
          onClear={props.onClearSearch}
          placeholder={t('Search model name, provider, endpoint, or tag...')}
        />
        <PricingToolbar
          {...props.toolbarProps}
          filteredCount={tableModels.length}
          totalCount={catalogModels.length}
          vendors={suppliers}
          groups={catalogSalesGroups}
          tags={extractAllTags(catalogModels)}
          models={catalogModels}
        />
      </div>

      <main
        data-pricing-block='catalog-table'
        data-public-comparison={groups.length > 0 ? 'ready' : 'hidden'}
      >
        {groups.length === 0 || tableModels.length === 0 ? (
          <EmptyState
            searchQuery={props.searchInput}
            hasActiveFilters={props.hasActiveFilters}
            onClearFilters={props.onClearAll}
          />
        ) : (
          <PricingTable
            models={tableModels}
            groups={groups}
            vendorName={projection[0]?.vendor.name}
            onModelClick={props.onModelClick}
          />
        )}
      </main>
    </div>
  )
}

export function Pricing() {
  const [selectedModelName, setSelectedModelName] = useState<string | null>(
    null
  )
  const pricingData = usePricingData()
  const models = pricingData.models || EMPTY_MODELS
  const vendors = pricingData.vendors || EMPTY_VENDORS
  const filters = useFilters(models)
  const { clearFilters, clearSearch } = filters

  const handleModelClick = useCallback((modelName: string) => {
    setSelectedModelName(modelName)
  }, [])
  const selectedModel = useMemo(
    () =>
      selectedModelName
        ? models.find((model) => model.model_name === selectedModelName) || null
        : null,
    [models, selectedModelName]
  )
  const handleClearAll = useCallback(() => {
    clearFilters()
    clearSearch()
  }, [clearFilters, clearSearch])

  if (pricingData.isLoading) {
    return (
      <PublicLayout showMainContainer={false}>
        <div className='mx-auto w-full max-w-[1600px] px-3 pt-16 pb-8 sm:px-6 sm:pt-20'>
          <LoadingSkeleton />
        </div>
      </PublicLayout>
    )
  }

  return (
    <PublicLayout showMainContainer={false}>
      <PageTransition className='mx-auto w-full max-w-[1600px] px-3 pt-16 pb-8 sm:px-6 sm:pt-20 xl:px-8'>
        <PricingCatalogLayout
          models={models}
          filteredModels={filters.filteredModels}
          vendors={vendors}
          searchInput={filters.searchInput}
          onSearchChange={filters.setSearchInput}
          onClearSearch={filters.clearSearch}
          hasActiveFilters={filters.hasActiveFilters}
          onClearAll={handleClearAll}
          onModelClick={handleModelClick}
          toolbarProps={{
            sortBy: filters.sortBy,
            onSortChange: filters.setSortBy,
            tokenUnit: filters.tokenUnit,
            onTokenUnitChange: filters.setTokenUnit,
            showRechargePrice: filters.showRechargePrice,
            onRechargePriceChange: filters.setShowRechargePrice,
            quotaTypeFilter: filters.quotaTypeFilter,
            endpointTypeFilter: filters.endpointTypeFilter,
            vendorFilter: filters.vendorFilter,
            groupFilter: filters.groupFilter,
            tagFilter: filters.tagFilter,
            onQuotaTypeChange: filters.setQuotaTypeFilter,
            onEndpointTypeChange: filters.setEndpointTypeFilter,
            onVendorChange: filters.setVendorFilter,
            onGroupChange: filters.setGroupFilter,
            onTagChange: filters.setTagFilter,
            groupRatios: pricingData.groupRatio,
            hasActiveFilters: filters.hasActiveFilters,
            activeFilterCount: filters.activeFilterCount,
            onClearFilters: filters.clearFilters,
          }}
        />

        {selectedModel && (
          <ModelDetailsDrawer
            open
            onOpenChange={(open) => {
              if (!open) setSelectedModelName(null)
            }}
            model={selectedModel}
            groupRatio={pricingData.groupRatio || {}}
            usableGroup={pricingData.usableGroup || {}}
            endpointMap={
              (pricingData.endpointMap as Record<
                string,
                { path?: string; method?: string }
              >) || {}
            }
            autoGroups={pricingData.autoGroups || []}
            priceRate={pricingData.priceRate ?? 1}
            usdExchangeRate={pricingData.usdExchangeRate ?? 1}
            tokenUnit={filters.tokenUnit}
            showRechargePrice={filters.showRechargePrice}
          />
        )}
      </PageTransition>
    </PublicLayout>
  )
}
