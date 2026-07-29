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
import { useTranslation } from 'react-i18next'

import { formatQuota, formatTimestampToDate } from '@/lib/format'

type UserQuotaCellProps = {
  permanent: number
  bonus: number
  nearestBonusExpiresAt: number
  total: number
}

export function UserQuotaCell(props: UserQuotaCellProps) {
  const { t } = useTranslation()
  const nearestExpiresAt = props.nearestBonusExpiresAt
    ? formatTimestampToDate(props.nearestBonusExpiresAt).split(' ')[0]
    : t('None')

  return (
    <div className='min-w-[220px] space-y-1 text-xs'>
      <div className='flex min-w-0 justify-between gap-3'>
        <span className='truncate'>{t('Permanent quota')}:</span>
        <span className='shrink-0 font-medium tabular-nums'>
          {formatQuota(props.permanent)}
        </span>
      </div>
      <div className='flex min-w-0 justify-between gap-3'>
        <span className='truncate'>{t('Bonus balance')}:</span>
        <span className='shrink-0 font-medium tabular-nums'>
          {formatQuota(props.bonus)}
        </span>
      </div>
      <div className='flex min-w-0 justify-between gap-3'>
        <span className='truncate'>{t('Available total')}:</span>
        <span className='shrink-0 font-medium tabular-nums'>
          {formatQuota(props.total)}
        </span>
      </div>
      <div className='text-muted-foreground truncate'>
        {t('Nearest bonus expiry')}: {nearestExpiresAt}
      </div>
    </div>
  )
}
