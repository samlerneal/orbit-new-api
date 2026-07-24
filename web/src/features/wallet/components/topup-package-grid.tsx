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
import { Check, Sparkles } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardFooter, CardHeader } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import { formatLocalCurrencyAmount } from '@/lib/currency'

import {
  getTopupPackageVisualStyleClasses,
  type TopupCampaign,
  type TopupPackage,
} from '../types'

interface TopupPackageGridProps {
  packages: TopupPackage[]
  campaigns: TopupCampaign[]
  loading: boolean
  paymentAvailable: boolean
  onSelectPackage: (packageOption: TopupPackage) => void
}

export function TopupPackageGrid(props: TopupPackageGridProps) {
  const { t } = useTranslation()

  if (props.loading) {
    return (
      <div className='grid gap-4 sm:grid-cols-2 xl:grid-cols-4'>
        {['one', 'two', 'three', 'four'].map((key) => (
          <Card key={key} data-card-hover='false'>
            <CardHeader>
              <Skeleton className='h-5 w-20' />
              <Skeleton className='h-4 w-32' />
            </CardHeader>
            <CardContent>
              <Skeleton className='h-10 w-24' />
              <Skeleton className='mt-5 h-4 w-28' />
            </CardContent>
            <CardFooter>
              <Skeleton className='h-10 w-full' />
            </CardFooter>
          </Card>
        ))}
      </div>
    )
  }

  if (!props.paymentAvailable || props.packages.length === 0) {
    return (
      <Alert>
        <AlertTitle>{t('Topup is temporarily unavailable')}</AlertTitle>
        <AlertDescription>
          {t('Please contact the administrator for assistance.')}
        </AlertDescription>
      </Alert>
    )
  }

  return (
    <div className='space-y-4'>
      {props.campaigns.map((campaign) => (
        <Alert
          key={campaign.id}
          className='border-amber-200 bg-amber-50/70 text-amber-950 dark:border-amber-900 dark:bg-amber-950/20 dark:text-amber-100'
        >
          <Sparkles className='size-4' />
          <AlertTitle className='flex items-center justify-between gap-3'>
            <span>{t(campaign.title)}</span>
            <Badge variant='outline' className='shrink-0 border-amber-300'>
              {t('Maximum bonus {{amount}}', {
                amount: formatLocalCurrencyAmount(
                  campaign.cumulative_max_bonus || campaign.max_bonus
                ),
              })}
            </Badge>
          </AlertTitle>
          {campaign.participant_limited && (
            <AlertDescription className='mt-1 font-medium'>
              {t('Limited {{total}} accounts, {{remaining}} spots remaining', {
                total: campaign.participant_total,
                remaining: campaign.participant_remaining,
              })}
            </AlertDescription>
          )}
          <AlertDescription>{t(campaign.description)}</AlertDescription>
        </Alert>
      ))}

      <div className='grid gap-4 sm:grid-cols-2 xl:grid-cols-4'>
        {props.packages.map((packageOption) => {
          const hasBonus = packageOption.bonus_amount > 0
          const badgeValidDays = packageOption.campaign_badges
            .filter((b) => b.valid_days > 0)
            .map((b) => b.valid_days)
          const sellingPoints =
            packageOption.selling_points?.filter(Boolean) ?? []
          const footerNote = packageOption.footer_note?.trim()
          const visualStyleClasses = getTopupPackageVisualStyleClasses(
            packageOption.visual_style
          )

          return (
            <Card
              key={packageOption.id}
              data-card-hover='false'
              className={`border-muted hover:border-primary/60 flex min-h-80 flex-col transition-colors ${visualStyleClasses}`}
            >
              <CardHeader className='space-y-1.5'>
                <div className='flex items-center justify-between gap-2'>
                  <h3 className='text-lg font-semibold'>
                    {t(packageOption.name)}
                  </h3>
                  {packageOption.tag && (
                    <Badge variant='secondary'>{t(packageOption.tag)}</Badge>
                  )}
                </div>
                <p className='text-muted-foreground text-sm'>
                  {t(packageOption.description)}
                </p>
              </CardHeader>

              <CardContent className='flex flex-1 flex-col'>
                <div className='flex items-baseline gap-1'>
                  <span className='text-muted-foreground text-lg'>¥</span>
                  <span className='text-4xl font-bold tracking-tight tabular-nums'>
                    {packageOption.pay_amount.toLocaleString(undefined, {
                      maximumFractionDigits: 2,
                    })}
                  </span>
                </div>
                <p className='text-muted-foreground mt-4 text-sm'>
                  {t('Balance received: {{amount}}', {
                    amount: formatLocalCurrencyAmount(
                      packageOption.display_credit_amount
                    ),
                  })}
                </p>
                {hasBonus && (
                  <div className='mt-2 flex flex-wrap gap-1.5'>
                    {packageOption.campaign_badges.map((badge) => (
                      <Badge
                        key={badge.campaign_id}
                        className='border-emerald-200 bg-emerald-50 text-emerald-700 dark:border-emerald-900 dark:bg-emerald-950/30 dark:text-emerald-300'
                        variant='outline'
                      >
                        {badge.valid_days > 0
                          ? `${t(badge.text)} · ${badge.valid_days}${t('d')}`
                          : t(badge.text)}
                      </Badge>
                    ))}
                  </div>
                )}

                {sellingPoints.length > 0 && (
                  <div className='mt-3 space-y-1'>
                    {sellingPoints.slice(0, 3).map((point) => (
                      <div
                        key={point}
                        className='text-muted-foreground flex items-start gap-1.5 text-sm'
                      >
                        <Check className='mt-0.5 size-3.5 shrink-0 text-emerald-500' />
                        <span>{point}</span>
                      </div>
                    ))}
                  </div>
                )}

                <div className='mt-auto flex items-center gap-2 border-t pt-4 text-sm'>
                  <Check className='size-4' />
                  <span>
                    {hasBonus
                      ? t(
                          'Regular balance never expires; bonus has its own expiry'
                        )
                      : t('Balance never expires')}
                  </span>
                </div>

                {badgeValidDays.length > 0 &&
                  !(badgeValidDays.length === 1 && badgeValidDays[0] === 0) && (
                    <div className='text-muted-foreground flex flex-col gap-0.5 text-sm'>
                      {packageOption.campaign_badges
                        .filter((b) => b.valid_days > 0)
                        .map((badge) => (
                          <div
                            key={badge.campaign_id}
                            className='flex items-center gap-1.5'
                          >
                            <Check className='size-3.5 text-emerald-500' />
                            <span>
                              {badgeValidDays.length === 1
                                ? t('Bonus balance valid {{days}} days', {
                                    days: badge.valid_days,
                                  })
                                : `${t(badge.text)}: ${t('Bonus balance valid {{days}} days', { days: badge.valid_days })}`}
                            </span>
                          </div>
                        ))}
                    </div>
                  )}

                {footerNote && (
                  <p className='text-muted-foreground mt-2 text-xs'>
                    {footerNote}
                  </p>
                )}
              </CardContent>

              <CardFooter>
                <Button
                  className='w-full'
                  onClick={() => props.onSelectPackage(packageOption)}
                >
                  {t('Top up now')}
                </Button>
              </CardFooter>
            </Card>
          )
        })}
      </div>
    </div>
  )
}
