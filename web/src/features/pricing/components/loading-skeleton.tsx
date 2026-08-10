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
import { Skeleton } from '@/components/ui/skeleton'

const TABLE_COLUMNS = [
  { key: 'model', width: 200 },
  { key: 'input', width: 100 },
  { key: 'output', width: 100 },
  { key: 'cache-create', width: 100 },
  { key: 'cache-read', width: 80 },
  { key: 'savings', width: 100 },
]

const TABLE_ROWS = Array.from(
  { length: 5 },
  (_value, index) => `row-${index + 1}`
)

function NavigationSkeleton({ widths }: { widths: string[] }) {
  return (
    <div className='flex gap-2'>
      {widths.map((width) => (
        <Skeleton key={width} className={`h-9 ${width} rounded-full`} />
      ))}
    </div>
  )
}

function CatalogControlsSkeleton() {
  return (
    <div className='grid gap-3 lg:grid-cols-[minmax(260px,1fr)_auto]'>
      <Skeleton className='h-10 w-full rounded-lg' />
      <div className='flex gap-2 rounded-xl border p-3'>
        <Skeleton className='h-8 w-24 rounded-lg' />
        <Skeleton className='h-8 w-28 rounded-lg' />
        <Skeleton className='h-8 w-20 rounded-lg' />
      </div>
    </div>
  )
}

function TableContentSkeleton() {
  return (
    <div className='overflow-hidden rounded-xl border'>
      <div className='bg-muted/30 flex items-center gap-4 border-b px-4 py-3'>
        {TABLE_COLUMNS.map((column) => (
          <Skeleton
            key={column.key}
            className='h-4'
            style={{ width: `${column.width}px` }}
          />
        ))}
      </div>
      {TABLE_ROWS.map((row) => (
        <div
          key={row}
          className='flex items-center gap-4 border-b px-4 py-3 last:border-b-0'
        >
          {TABLE_COLUMNS.map((column) => (
            <Skeleton
              key={column.key}
              className='h-5'
              style={{ width: `${column.width}px` }}
            />
          ))}
        </div>
      ))}
    </div>
  )
}

export function LoadingSkeleton() {
  return (
    <div className='space-y-5'>
      <div className='space-y-2'>
        <Skeleton className='h-10 w-52' />
        <Skeleton className='h-5 w-80 max-w-full' />
      </div>
      <NavigationSkeleton widths={['w-24']} />
      <div className='border-b pb-3'>
        <NavigationSkeleton widths={['w-28']} />
      </div>
      <CatalogControlsSkeleton />
      <div className='space-y-3'>
        <div className='flex items-start justify-between gap-3'>
          <div className='space-y-2'>
            <Skeleton className='h-6 w-72 max-w-full' />
            <Skeleton className='h-4 w-96 max-w-full' />
          </div>
          <Skeleton className='h-8 w-36 rounded-full' />
        </div>
        <TableContentSkeleton />
      </div>
    </div>
  )
}
