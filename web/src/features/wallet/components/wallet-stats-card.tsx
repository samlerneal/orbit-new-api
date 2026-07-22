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
import { Receipt, WalletCards } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { IconBadge } from '@/components/ui/icon-badge'
import { Skeleton } from '@/components/ui/skeleton'
import { formatQuota } from '@/lib/format'

import type { UserWalletData } from '../types'

interface WalletStatsCardProps {
  user: UserWalletData | null
  loading?: boolean
  onOpenBilling: () => void
}

export function WalletStatsCard(props: WalletStatsCardProps) {
  const { t } = useTranslation()
  if (props.loading) {
    return (
      <Card data-card-hover='false'>
        <CardContent className='flex items-center justify-between gap-4 p-4 sm:p-5'>
          <div className='flex items-center gap-3'>
            <Skeleton className='size-10 rounded-lg' />
            <div>
              <Skeleton className='h-4 w-24' />
              <Skeleton className='mt-2 h-7 w-36' />
            </div>
          </div>
          <Skeleton className='h-9 w-24' />
        </CardContent>
      </Card>
    )
  }

  return (
    <Card data-card-hover='false'>
      <CardContent className='flex items-center justify-between gap-4 p-4 sm:p-5'>
        <div className='flex min-w-0 items-center gap-3'>
          <IconBadge tone='success' size='stat'>
            <WalletCards />
          </IconBadge>
          <div className='min-w-0'>
            <div className='text-muted-foreground text-xs font-medium tracking-wider uppercase'>
              {t('Current Balance')}
            </div>
            <div className='mt-1 truncate font-mono text-2xl font-bold tracking-tight tabular-nums'>
              {formatQuota(props.user?.total_quota ?? props.user?.quota ?? 0)}
            </div>
            {(props.user?.bonus_quota ?? 0) > 0 && (
              <div className='text-muted-foreground mt-1 text-xs'>
                {t('Includes {{amount}} promotional balance', {
                  amount: formatQuota(props.user?.bonus_quota ?? 0),
                })}
                {props.user?.bonus_nearest_expires_at
                  ? ` · ${t('Nearest expiry: {{date}}', {
                      date: new Date(
                        props.user.bonus_nearest_expires_at * 1000
                      ).toLocaleDateString(),
                    })}`
                  : ''}
              </div>
            )}
          </div>
        </div>
        <Button variant='outline' size='sm' onClick={props.onOpenBilling}>
          <Receipt className='size-4' />
          <span className='hidden sm:inline'>{t('Order History')}</span>
        </Button>
      </CardContent>
    </Card>
  )
}
