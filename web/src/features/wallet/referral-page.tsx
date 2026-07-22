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
import { Check, Gift } from 'lucide-react'
import { useCallback, useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { SectionPageLayout } from '@/components/layout'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { IconBadge } from '@/components/ui/icon-badge'
import { getSelf } from '@/lib/api'

import { AffiliateRewardsCard } from './components/affiliate-rewards-card'
import { TransferDialog } from './components/dialogs/transfer-dialog'
import { useAffiliate } from './hooks'
import type { UserWalletData } from './types'

export function ReferralPage() {
  const { t } = useTranslation()
  const [user, setUser] = useState<UserWalletData | null>(null)
  const [transferDialogOpen, setTransferDialogOpen] = useState(false)
  const { affiliateLink, loading, transferQuota, transferring } = useAffiliate()

  const fetchUser = useCallback(async () => {
    const response = await getSelf()
    if (response.success && response.data) {
      setUser(response.data as UserWalletData)
    }
  }, [])

  useEffect(() => {
    void fetchUser()
  }, [fetchUser])

  const handleTransfer = async (amount: number) => {
    const success = await transferQuota(amount)
    if (success) {
      await fetchUser()
    }
    return success
  }

  return (
    <>
      <SectionPageLayout>
        <SectionPageLayout.Title>
          {t('Referral Rewards')}
        </SectionPageLayout.Title>
        <SectionPageLayout.Content>
          <div className='mx-auto flex w-full max-w-5xl flex-col gap-5'>
            <Card data-card-hover='false'>
              <CardHeader className='flex-row items-center gap-3'>
                <IconBadge tone='success'>
                  <Gift />
                </IconBadge>
                <div>
                  <CardTitle>{t('Invite friends and earn rewards')}</CardTitle>
                  <p className='text-muted-foreground mt-1 text-sm'>
                    {t('Share your personal link with friends.')}
                  </p>
                </div>
              </CardHeader>
              <CardContent>
                <AffiliateRewardsCard
                  user={user}
                  affiliateLink={affiliateLink}
                  onTransfer={() => setTransferDialogOpen(true)}
                  loading={loading}
                />
              </CardContent>
            </Card>

            <Card data-card-hover='false'>
              <CardHeader>
                <CardTitle>{t('Reward details')}</CardTitle>
              </CardHeader>
              <CardContent>
                <ul className='space-y-3 text-sm'>
                  {[
                    t('Invite friends through your personal referral link.'),
                    t('Rewards are credited after eligible friend activity.'),
                    t('Transfer available rewards to your account balance.'),
                  ].map((item) => (
                    <li key={item} className='flex items-start gap-2'>
                      <Check className='mt-0.5 size-4 text-green-600' />
                      <span>{item}</span>
                    </li>
                  ))}
                </ul>
              </CardContent>
            </Card>
          </div>
        </SectionPageLayout.Content>
      </SectionPageLayout>

      <TransferDialog
        open={transferDialogOpen}
        onOpenChange={setTransferDialogOpen}
        onConfirm={handleTransfer}
        availableQuota={user?.aff_quota ?? 0}
        transferring={transferring}
      />
    </>
  )
}
