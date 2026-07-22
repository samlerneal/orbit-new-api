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
import { Loader2 } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { formatLocalCurrencyAmount } from '@/lib/currency'

import { getPaymentIcon } from '../../lib'
import type { TopupPackage } from '../../types'

interface PackagePaymentDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  packageOption: TopupPackage | null
  processing: boolean
  onPay: () => void
}

export function PackagePaymentDialog(props: PackagePaymentDialogProps) {
  const { t } = useTranslation()
  const packageOption = props.packageOption

  return (
    <Dialog open={props.open} onOpenChange={props.onOpenChange}>
      <DialogContent className='sm:max-w-lg'>
        <DialogHeader>
          <DialogTitle className='text-xl'>
            {t('Select payment method')}
          </DialogTitle>
          <DialogDescription>
            {t('Confirm the amount, then continue with WeChat Pay.')}
          </DialogDescription>
        </DialogHeader>

        {packageOption && (
          <div className='space-y-5 pt-2'>
            <div className='rounded-2xl border border-orange-200 bg-orange-50/60 p-5 dark:border-orange-900 dark:bg-orange-950/20'>
              <div className='text-muted-foreground text-sm'>
                {t('Payment details')}
              </div>
              <div className='mt-2 text-4xl font-bold tracking-tight tabular-nums'>
                {formatLocalCurrencyAmount(packageOption.pay_amount)}
              </div>
              <div className='text-muted-foreground mt-4 text-sm'>
                {t('Balance received: {{amount}}', {
                  amount: formatLocalCurrencyAmount(
                    packageOption.credit_amount
                  ),
                })}
              </div>
            </div>

            <div className='rounded-xl border px-4 py-3 text-center font-medium'>
              {t('RMB payment')}
            </div>

            <div className='space-y-2'>
              <div className='text-sm font-medium'>{t('Payment Method')}</div>
              <Button
                variant='outline'
                className='h-14 w-full justify-center gap-3 rounded-xl text-base'
                onClick={props.onPay}
                disabled={props.processing}
              >
                {props.processing ? (
                  <Loader2 className='size-5 animate-spin' />
                ) : (
                  getPaymentIcon('wxpay', 'size-5')
                )}
                {t('WeChat Pay')}
              </Button>
            </div>
          </div>
        )}
      </DialogContent>
    </Dialog>
  )
}
