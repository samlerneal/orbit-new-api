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
import { CalendarDays, Coins } from 'lucide-react'
import { useCallback, useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { SectionPageLayout } from '@/components/layout'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { getSelf } from '@/lib/api'

import { BillingHistoryDialog } from './components/dialogs/billing-history-dialog'
import { PackagePaymentDialog } from './components/dialogs/package-payment-dialog'
import { TopupPackageGrid } from './components/topup-package-grid'
import { WalletStatsCard } from './components/wallet-stats-card'
import { usePackagePayment, useTopupInfo } from './hooks'
import {
  isEpayPaymentMethod,
  REFUND_NOTICE_VERSION,
  type EpayPaymentMethod,
  type RefundNoticeAcceptance,
  type TopupPackage,
  type UserWalletData,
} from './types'

interface WalletProps {
  initialShowHistory?: boolean
}

export function Wallet(props: WalletProps) {
  const { t } = useTranslation()
  const [user, setUser] = useState<UserWalletData | null>(null)
  const [userLoading, setUserLoading] = useState(true)
  const [billingDialogOpen, setBillingDialogOpen] = useState(false)
  const [paymentDialogOpen, setPaymentDialogOpen] = useState(false)
  const [selectedPackage, setSelectedPackage] = useState<TopupPackage | null>(
    null
  )

  const { topupInfo, loading: topupLoading } = useTopupInfo()
  const { processing, processPackagePayment } = usePackagePayment()

  const fetchUser = useCallback(async () => {
    try {
      setUserLoading(true)
      const response = await getSelf()
      if (response.success && response.data) {
        setUser(response.data as UserWalletData)
      }
    } finally {
      setUserLoading(false)
    }
  }, [])

  useEffect(() => {
    void fetchUser()
  }, [fetchUser])

  useEffect(() => {
    if (!props.initialShowHistory) return

    setBillingDialogOpen(true)
    window.history.replaceState({}, '', window.location.pathname)
  }, [props.initialShowHistory])

  const handleSelectPackage = (packageOption: TopupPackage) => {
    setSelectedPackage(packageOption)
    setPaymentDialogOpen(true)
  }

  const handlePay = async (
    paymentMethod: EpayPaymentMethod,
    refundNotice: RefundNoticeAcceptance
  ) => {
    if (!selectedPackage) return

    const success = await processPackagePayment(
      selectedPackage.id,
      paymentMethod,
      refundNotice
    )
    if (success) {
      setPaymentDialogOpen(false)
    }
  }

  const epayPaymentMethods = [
    ...new Set(
      (topupInfo?.pay_methods ?? [])
        .map((method) => method.type)
        .filter(isEpayPaymentMethod)
    ),
  ]
  const paymentAvailable =
    topupInfo?.enable_online_topup === true &&
    topupInfo.refund_notice_version === REFUND_NOTICE_VERSION &&
    epayPaymentMethods.length > 0

  return (
    <>
      <SectionPageLayout>
        <SectionPageLayout.Title>{t('Wallet')}</SectionPageLayout.Title>
        <SectionPageLayout.Content>
          <div className='mx-auto flex w-full max-w-7xl flex-col gap-5'>
            <WalletStatsCard
              user={user}
              loading={userLoading}
              onOpenBilling={() => setBillingDialogOpen(true)}
            />

            <Tabs defaultValue='pay-as-you-go'>
              <TabsList className='bg-muted/60 grid h-auto w-full grid-cols-2 rounded-xl p-1 dark:bg-white/5'>
                <TabsTrigger
                  value='pay-as-you-go'
                  className='data-active:bg-background min-h-11 gap-2 rounded-lg data-active:shadow-sm dark:data-active:bg-white/10'
                >
                  <Coins className='size-4' />
                  {t('Pay as you go')}
                </TabsTrigger>
                <TabsTrigger
                  value='monthly-subscription'
                  className='data-active:bg-background bg-background/40 border-border/30 min-h-11 gap-2 rounded-lg border shadow-sm disabled:opacity-100 aria-disabled:opacity-100 dark:border-white/10 dark:bg-white/5'
                  disabled
                >
                  <span className='inline-flex min-w-0 flex-wrap items-center justify-center gap-2 whitespace-normal opacity-55'>
                    <CalendarDays className='size-4' />
                    {t('Monthly subscription')}
                    <span className='text-muted-foreground text-xs'>
                      {t('Coming soon')}
                    </span>
                  </span>
                </TabsTrigger>
              </TabsList>

              <TabsContent value='pay-as-you-go' className='mt-5'>
                <TopupPackageGrid
                  packages={topupInfo?.topup_packages ?? []}
                  campaigns={topupInfo?.campaigns ?? []}
                  loading={topupLoading}
                  paymentAvailable={paymentAvailable}
                  onSelectPackage={handleSelectPackage}
                />
              </TabsContent>
            </Tabs>
          </div>
        </SectionPageLayout.Content>
      </SectionPageLayout>

      <PackagePaymentDialog
        open={paymentDialogOpen}
        onOpenChange={setPaymentDialogOpen}
        packageOption={selectedPackage}
        paymentMethods={epayPaymentMethods}
        supportContacts={topupInfo?.support_contacts ?? []}
        processing={processing}
        onPay={handlePay}
      />

      <BillingHistoryDialog
        open={billingDialogOpen}
        onOpenChange={setBillingDialogOpen}
      />
    </>
  )
}
