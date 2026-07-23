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
import i18next from 'i18next'
import { useCallback, useState } from 'react'
import { toast } from 'sonner'

import { isApiSuccess, requestPayment } from '../api'
import { submitPaymentForm } from '../lib'
import {
  createPackagePaymentRequest,
  type RefundNoticeAcceptance,
} from '../types'

export function usePackagePayment() {
  const [processing, setProcessing] = useState(false)

  const processPackagePayment = useCallback(
    async (packageId: string, refundNotice: RefundNoticeAcceptance) => {
      try {
        setProcessing(true)
        const response = await requestPayment(
          createPackagePaymentRequest(packageId, refundNotice)
        )

        if (!isApiSuccess(response)) {
          toast.error(response.message || i18next.t('Payment request failed'))
          return false
        }

        if (!response.data || !response.url) {
          toast.error(i18next.t('Payment request failed'))
          return false
        }

        submitPaymentForm(response.url, response.data)
        toast.success(i18next.t('Redirecting to payment page...'))
        return true
      } catch {
        toast.error(i18next.t('Payment request failed'))
        return false
      } finally {
        setProcessing(false)
      }
    },
    []
  )

  return { processing, processPackagePayment }
}
