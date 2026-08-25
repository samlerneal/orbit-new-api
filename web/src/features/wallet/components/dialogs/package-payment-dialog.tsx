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
import { ChevronDown, Loader2 } from 'lucide-react'
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from '@/components/ui/collapsible'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Label } from '@/components/ui/label'
import { normalizeInterfaceLanguage } from '@/i18n/languages'
import { formatLocalCurrencyAmount } from '@/lib/currency'

import { getPaymentIcon } from '../../lib'
import { getPaymentMethodName } from '../../lib/billing'
import {
  canSubmitPackagePayment,
  REFUND_NOTICE_VERSION,
  type EpayPaymentMethod,
  type RefundNoticeAcceptance,
  type SupportContact,
  type TopupPackage,
} from '../../types'

interface PackagePaymentDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  packageOption: TopupPackage | null
  paymentMethods: EpayPaymentMethod[]
  supportContacts: SupportContact[]
  processing: boolean
  onPay: (
    paymentMethod: EpayPaymentMethod,
    refundNotice: RefundNoticeAcceptance
  ) => void | Promise<void>
}

function getSupportContactLabel(type: SupportContact['type']) {
  switch (type) {
    case 'qq':
      return 'Customer service QQ'
    case 'wechat':
      return 'Customer service WeChat'
    case 'phone':
      return 'Customer service phone'
    case 'qrcode':
      return 'Customer service QR code'
  }
}

export function PackagePaymentDialog(props: PackagePaymentDialogProps) {
  const { i18n, t } = useTranslation()
  const packageOption = props.packageOption
  const [refundNoticeAccepted, setRefundNoticeAccepted] = useState(false)
  const [refundNoticeExpanded, setRefundNoticeExpanded] = useState(false)

  useEffect(() => {
    setRefundNoticeAccepted(false)
    setRefundNoticeExpanded(false)
  }, [packageOption?.id, props.open])

  const handlePay = (paymentMethod: EpayPaymentMethod) => {
    if (!canSubmitPackagePayment(refundNoticeAccepted, props.processing)) {
      return
    }

    props.onPay(paymentMethod, {
      refund_notice_accepted: true,
      refund_notice_version: REFUND_NOTICE_VERSION,
      refund_notice_language: normalizeInterfaceLanguage(
        i18n.resolvedLanguage || i18n.language
      ),
    })
  }

  return (
    <Dialog open={props.open} onOpenChange={props.onOpenChange}>
      <DialogContent className='sm:max-w-lg'>
        <DialogHeader>
          <DialogTitle className='text-xl'>
            {t('Select payment method')}
          </DialogTitle>
          <DialogDescription>{t('Payment details')}</DialogDescription>
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
                    packageOption.display_credit_amount
                  ),
                })}
              </div>
            </div>

            <div className='rounded-xl border px-4 py-3 text-center font-medium'>
              {t('RMB payment')}
            </div>

            <div className='space-y-3 rounded-xl border border-blue-200 bg-blue-50/60 p-4 dark:border-blue-900 dark:bg-blue-950/20'>
              <div className='text-sm font-semibold'>{t('Refund notice')}</div>
              <p className='text-muted-foreground text-sm leading-6'>
                {t(
                  'Refund summary: Request a full refund within 7 days if no billable API use occurred.'
                )}
              </p>
              <Collapsible
                open={refundNoticeExpanded}
                onOpenChange={setRefundNoticeExpanded}
              >
                <CollapsibleTrigger className='hover:bg-background/70 flex w-full items-center justify-between rounded-lg border px-3 py-2 text-left text-sm font-medium'>
                  <span>
                    {refundNoticeExpanded
                      ? t('Hide full refund notice')
                      : t('View full refund notice')}
                  </span>
                  <ChevronDown
                    className={`size-4 transition-transform ${refundNoticeExpanded ? 'rotate-180' : ''}`}
                  />
                </CollapsibleTrigger>
                <CollapsibleContent className='text-muted-foreground space-y-3 px-1 pt-3 text-sm leading-6'>
                  <p>{t('Refund notice paragraph 1')}</p>
                  <p>{t('Refund notice paragraph 2')}</p>
                  <p>{t('Refund notice paragraph 3')}</p>
                  {props.supportContacts.length > 0 && (
                    <div className='bg-background/70 space-y-2 rounded-lg border p-3'>
                      <div className='text-foreground font-medium'>
                        {t('Customer service')}
                      </div>
                      {props.supportContacts.map((contact) =>
                        contact.type === 'qrcode' ? (
                          <div key={contact.id} className='space-y-2'>
                            <div>{t('Customer service QR code')}</div>
                            <img
                              src={contact.value}
                              alt={t('Customer service QR code')}
                              className='size-36 rounded-lg border object-contain'
                              loading='lazy'
                              referrerPolicy='no-referrer'
                            />
                          </div>
                        ) : (
                          <div key={contact.id} className='break-all'>
                            {t(getSupportContactLabel(contact.type))}：
                            {contact.value}
                          </div>
                        )
                      )}
                    </div>
                  )}
                </CollapsibleContent>
              </Collapsible>
              <div className='bg-background/60 flex items-start gap-3 rounded-lg border px-3 py-3'>
                <Checkbox
                  id='package-payment-refund-notice'
                  checked={refundNoticeAccepted}
                  disabled={props.processing}
                  onCheckedChange={(checked) =>
                    setRefundNoticeAccepted(checked === true)
                  }
                  className='mt-0.5'
                />
                <Label
                  htmlFor='package-payment-refund-notice'
                  className='cursor-pointer text-left text-sm leading-5 font-normal'
                >
                  {t('I have read and agree to the refund notice')}
                </Label>
              </div>
            </div>

            <div className='space-y-2'>
              <div className='text-sm font-medium'>{t('Payment Method')}</div>
              {props.paymentMethods.map((paymentMethod) => (
                <Button
                  key={paymentMethod}
                  variant='outline'
                  className='h-14 w-full justify-center gap-3 rounded-xl text-base'
                  onClick={() => handlePay(paymentMethod)}
                  disabled={
                    !canSubmitPackagePayment(
                      refundNoticeAccepted,
                      props.processing
                    )
                  }
                >
                  {props.processing ? (
                    <Loader2 className='size-5 animate-spin' />
                  ) : (
                    getPaymentIcon(paymentMethod, 'size-5')
                  )}
                  {getPaymentMethodName(paymentMethod, t)}
                </Button>
              ))}
            </div>
          </div>
        )}
      </DialogContent>
    </Dialog>
  )
}
