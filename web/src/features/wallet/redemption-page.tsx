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
import { CircleHelp, Gift, Loader2, WalletCards } from 'lucide-react'
import { useCallback, useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { SectionPageLayout } from '@/components/layout'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { IconBadge } from '@/components/ui/icon-badge'
import { Input } from '@/components/ui/input'
import { Skeleton } from '@/components/ui/skeleton'
import { getSelf } from '@/lib/api'
import { formatQuota } from '@/lib/format'

import { useRedemption } from './hooks'
import type { UserWalletData } from './types'

export function RedemptionPage() {
  const { t } = useTranslation()
  const [user, setUser] = useState<UserWalletData | null>(null)
  const [loading, setLoading] = useState(true)
  const [redemptionCode, setRedemptionCode] = useState('')
  const { redeeming, redeemCode } = useRedemption()

  const fetchUser = useCallback(async () => {
    try {
      setLoading(true)
      const response = await getSelf()
      if (response.success && response.data) {
        setUser(response.data as UserWalletData)
      }
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    void fetchUser()
  }, [fetchUser])

  const handleRedeem = async () => {
    const normalizedCode = redemptionCode.trim()
    if (!normalizedCode) return

    const success = await redeemCode(normalizedCode)
    if (success) {
      setRedemptionCode('')
      await fetchUser()
    }
  }

  return (
    <SectionPageLayout>
      <SectionPageLayout.Title>{t('Redeem')}</SectionPageLayout.Title>
      <SectionPageLayout.Content>
        <div className='mx-auto flex w-full max-w-4xl flex-col gap-5'>
          <Card data-card-hover='false' className='overflow-hidden'>
            <CardContent className='via-background flex min-h-48 flex-col items-center justify-center bg-gradient-to-br from-sky-50 to-orange-50 p-8 dark:from-sky-950/20 dark:to-orange-950/20'>
              <IconBadge tone='success' size='lg'>
                <WalletCards />
              </IconBadge>
              <div className='text-muted-foreground mt-4 text-sm'>
                {t('Current Balance')}
              </div>
              {loading ? (
                <Skeleton className='mt-2 h-10 w-40' />
              ) : (
                <div className='mt-2 font-mono text-4xl font-bold tracking-tight tabular-nums'>
                  {formatQuota(user?.quota ?? 0)}
                </div>
              )}
            </CardContent>
          </Card>

          <Card data-card-hover='false'>
            <CardHeader>
              <CardTitle>{t('Redemption Code')}</CardTitle>
            </CardHeader>
            <CardContent className='space-y-3'>
              <div className='relative'>
                <Gift className='text-muted-foreground absolute top-1/2 left-3 size-4 -translate-y-1/2' />
                <Input
                  value={redemptionCode}
                  onChange={(event) => setRedemptionCode(event.target.value)}
                  onKeyDown={(event) => {
                    if (event.key === 'Enter') void handleRedeem()
                  }}
                  placeholder={t('Enter your redemption code')}
                  aria-label={t('Redemption Code')}
                  className='h-11 pl-10'
                />
              </div>
              <p className='text-muted-foreground text-xs'>
                {t('Redemption codes are case-sensitive.')}
              </p>
              <Button
                className='w-full'
                onClick={handleRedeem}
                disabled={!redemptionCode.trim() || redeeming}
              >
                {redeeming && <Loader2 className='size-4 animate-spin' />}
                {t('Redeem')}
              </Button>
            </CardContent>
          </Card>

          <Card data-card-hover='false'>
            <CardHeader className='flex-row items-center gap-3'>
              <IconBadge tone='info'>
                <CircleHelp />
              </IconBadge>
              <CardTitle>{t('About redemption codes')}</CardTitle>
            </CardHeader>
            <CardContent>
              <ul className='text-muted-foreground list-disc space-y-2 pl-5 text-sm'>
                <li>{t('Each redemption code can only be used once.')}</li>
                <li>
                  {t('A valid redemption code adds balance immediately.')}
                </li>
                <li>
                  {t('Contact support if a valid code cannot be redeemed.')}
                </li>
              </ul>
            </CardContent>
          </Card>
        </div>
      </SectionPageLayout.Content>
    </SectionPageLayout>
  )
}
